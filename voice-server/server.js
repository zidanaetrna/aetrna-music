const express = require('express');
const {
    joinVoiceChannel, createAudioPlayer, createAudioResource,
    AudioPlayerStatus, VoiceConnectionStatus, entersState, StreamType,
    generateDependencyReport
} = require('@discordjs/voice');
const { spawn, execFile } = require('child_process');
const fs = require('fs');
const path = require('path');
const http = require('http');

console.log('📋 [VoiceServer] Voice dependency report:');
console.log(generateDependencyReport());

const app = express();
app.use(express.json());

const PORT = process.env.PORT || 3005;
const BOT_WEBHOOK = process.env.BOT_WEBHOOK || 'http://127.0.0.1:8080/internal/track-end';
const GATEWAY_SEND_WEBHOOK = process.env.GATEWAY_SEND_WEBHOOK || 'http://127.0.0.1:8080/internal/gateway-send';

const adapters = new Map();
const voiceStatesMap = new Map();
const connections = new Map();
const players = new Map();
const activeStreams = new Map();

function createCustomAdapter(guildId) {
    return (methods) => {
        adapters.set(guildId, methods);

        // Apply cached voice credentials ONLY when complete (token, endpoint, sessionId, userId)
        const cachedState = voiceStatesMap.get(guildId);
        if (cachedState && cachedState.token && cachedState.endpoint && cachedState.sessionId && cachedState.userId) {
            const cleanEndpoint = cachedState.endpoint.split(':')[0];
            console.log(`🔑 [VoiceServer] Applying FULL cached voice state on adapter creation for guild ${guildId} (Endpoint: ${cleanEndpoint})`);
            methods.onVoiceServerUpdate({ token: cachedState.token, endpoint: cleanEndpoint, guild_id: guildId });
            methods.onVoiceStateUpdate({ session_id: cachedState.sessionId, guild_id: guildId, user_id: cachedState.userId });
        }

        return {
            sendPayload(data) {
                // Forward OP4 Voice State payload from @discordjs/voice to Go Bot Gateway
                const body = JSON.stringify({ guildId, payload: data });
                const req = http.request(GATEWAY_SEND_WEBHOOK, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Content-Length': Buffer.byteLength(body)
                    }
                });
                req.on('error', (err) => console.error(`❌ [VoiceServer] CustomAdapter payload send error: ${err.message}`));
                req.write(body);
                req.end();
                return true;
            },
            destroy() {
                adapters.delete(guildId);
                voiceStatesMap.delete(guildId);
            }
        };
    };
}

function cleanupStreams(guildId) {
    if (activeStreams.has(guildId)) {
        const stream = activeStreams.get(guildId);
        try { if (stream.ffmpeg) stream.ffmpeg.kill('SIGKILL'); } catch (e) {}
        activeStreams.delete(guildId);
    }
}

function notifyBotTrackEnd(guildId, reason) {
    const data = JSON.stringify({ guildId, reason });
    const req = http.request(BOT_WEBHOOK, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) }
    });
    req.on('error', () => {});
    req.write(data);
    req.end();
}

// Async (non-blocking) yt-dlp URL resolver
function resolveStreamUrl(args, env) {
    return new Promise((resolve, reject) => {
        execFile('yt-dlp', args, { env, timeout: 30000, maxBuffer: 2 * 1024 * 1024 }, (err, stdout, stderr) => {
            if (err) return reject(new Error(stderr.trim() || err.message));
            const url = stdout.trim().split('\n')[0];
            if (!url) return reject(new Error('yt-dlp returned empty URL'));
            resolve(url);
        });
    });
}

// Receive Gateway Voice Updates from Go Bot
app.post('/voice-state', (req, res) => {
    const { guildId, token, endpoint, sessionId, userId } = req.body;
    if (!guildId) return res.status(400).json({ error: 'Missing guildId' });

    const cleanEndpoint = endpoint ? endpoint.split(':')[0] : undefined;

    // Store in voiceStatesMap for cache
    const current = voiceStatesMap.get(guildId) || {};
    if (token) current.token = token;
    if (cleanEndpoint) current.endpoint = cleanEndpoint;
    if (sessionId) current.sessionId = sessionId;
    if (userId) current.userId = userId;
    voiceStatesMap.set(guildId, current);

    const adapter = adapters.get(guildId);
    if (adapter) {
        // ONLY trigger voice updates when all 4 parameters exist together to prevent @discordjs/voice abort
        if (current.token && current.endpoint && current.sessionId && current.userId) {
            adapter.onVoiceServerUpdate({ token: current.token, endpoint: current.endpoint, guild_id: guildId });
            adapter.onVoiceStateUpdate({ session_id: current.sessionId, guild_id: guildId, user_id: current.userId });
            console.log(`✅ [VoiceServer] FULL Voice state applied to active adapter for guild ${guildId} (Endpoint: ${current.endpoint})`);
            return res.json({ status: 'ok', updated: true });
        }
        console.log(`⏳ [VoiceServer] Partial voice state received for guild ${guildId} (waiting for complete credentials)`);
        return res.json({ status: 'ok', updated: false, pending: true });
    }

    console.log(`🔑 [VoiceServer] Voice state cached for guild ${guildId} (awaiting adapter creation)`);
    res.json({ status: 'ok', updated: false, cached: true });
});

app.post('/play', async (req, res) => {
    const { guildId, channelId, url, volume = 1.0 } = req.body;
    if (!guildId || !channelId || !url) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or url' });
    }

    try {
        cleanupStreams(guildId);

        // Join voice channel using Custom Adapter (delegates Gateway payloads to Go Bot)
        let connection = connections.get(guildId);
        if (!connection || connection.joinConfig.channelId !== channelId || connection.state.status === VoiceConnectionStatus.Destroyed) {
            if (connection) {
                try { connection.destroy(); } catch (e) {}
            }

            connection = joinVoiceChannel({
                channelId: channelId,
                guildId: guildId,
                adapterCreator: createCustomAdapter(guildId),
                selfDeaf: true,
            });

            connections.set(guildId, connection);

            connection.on(VoiceConnectionStatus.Disconnected, async () => {
                try {
                    await Promise.race([
                        entersState(connection, VoiceConnectionStatus.Signalling, 3_000),
                        entersState(connection, VoiceConnectionStatus.Connecting, 3_000),
                    ]);
                } catch {
                    try { connection.destroy(); } catch (e) {}
                    connections.delete(guildId);
                }
            });
        }

        // Get or create audio player
        let player = players.get(guildId);
        if (!player) {
            player = createAudioPlayer();
            players.set(guildId, player);

            player.on(AudioPlayerStatus.Idle, () => {
                cleanupStreams(guildId);
                notifyBotTrackEnd(guildId, 'finished');
            });

            player.on('error', (err) => {
                console.error(`❌ [VoiceServer] Player error in guild ${guildId}:`, err.message);
                cleanupStreams(guildId);
                notifyBotTrackEnd(guildId, 'error');
            });
        }

        // Cookie file check
        let cookieFile = '/opt/aetrna-music/prod/cookies.txt';
        if (!fs.existsSync(cookieFile)) cookieFile = path.resolve(__dirname, '../cookies.txt');
        if (!fs.existsSync(cookieFile)) cookieFile = path.resolve(__dirname, './cookies.txt');
        const useCookies = fs.existsSync(cookieFile);
        if (useCookies) console.log(`🔑 [VoiceServer] Found cookies file at: ${cookieFile}`);

        const spawnEnv = {
            ...process.env,
            HOME: '/root',
            PATH: (process.env.PATH || '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin') + ':/root/.deno/bin:/root/.local/bin',
        };

        const resolveArgs = [
            ...(useCookies ? ['--cookies', cookieFile] : []),
            '-f', 'bestaudio/best',
            '--no-playlist', '--geo-bypass', '--no-check-certificates', '--no-warnings',
            '-g', url
        ];

        // Step 1: Resolve stream URL async (non-blocking)
        let streamUrl;
        try {
            streamUrl = await resolveStreamUrl(resolveArgs, spawnEnv);
            console.log(`🔗 [VoiceServer] Resolved stream URL for guild ${guildId}`);
        } catch (e) {
            console.error(`❌ [VoiceServer] yt-dlp resolve error: ${e.message}`);
            return res.status(500).json({ error: `yt-dlp resolve failed: ${e.message}` });
        }

        // Step 2: Wait for Voice Connection Ready
        try {
            console.log(`⏳ [VoiceServer] Waiting for voice connection Ready...`);
            await entersState(connection, VoiceConnectionStatus.Ready, 20_000);
            console.log(`✅ [VoiceServer] Voice connection Ready!`);
        } catch (stateErr) {
            console.error(`❌ [VoiceServer] Voice connection failed: ${stateErr.message}`);
            return res.status(500).json({ error: `Voice connection failed: ${stateErr.message}` });
        }

        // Step 3: Stream directly via ffmpeg
        const ffmpegArgs = [
            '-reconnect', '1', '-reconnect_streamed', '1', '-reconnect_delay_max', '5',
            '-i', streamUrl,
            '-analyzeduration', '0', '-loglevel', 'error',
            '-f', 's16le', '-ar', '48000', '-ac', '2', 'pipe:1'
        ];

        const ffmpeg = spawn('ffmpeg', ffmpegArgs, { stdio: ['ignore', 'pipe', 'pipe'] });
        activeStreams.set(guildId, { ffmpeg });

        ffmpeg.stderr.on('data', (d) => {
            const msg = d.toString().trim();
            if (msg) console.log(`[ffmpeg] ${msg}`);
        });

        const resource = createAudioResource(ffmpeg.stdout, {
            inputType: StreamType.Raw,
            inlineVolume: true,
        });
        resource.volume.setVolume(volume);

        player.play(resource);
        connection.subscribe(player);

        res.json({ status: 'ok', message: 'Playback started' });

    } catch (err) {
        console.error('❌ [VoiceServer] Play error:', err);
        res.status(500).json({ error: err.message });
    }
});

app.post('/stop', (req, res) => {
    const { guildId } = req.body;
    cleanupStreams(guildId);

    const player = players.get(guildId);
    if (player) { player.stop(); players.delete(guildId); }

    const connection = connections.get(guildId);
    if (connection) {
        try { connection.destroy(); } catch (e) {}
        connections.delete(guildId);
    }
    adapters.delete(guildId);
    voiceStatesMap.delete(guildId);

    res.json({ status: 'ok' });
});

app.post('/pause', (req, res) => {
    const player = players.get(req.body.guildId);
    if (player) player.pause();
    res.json({ status: 'ok' });
});

app.post('/resume', (req, res) => {
    const player = players.get(req.body.guildId);
    if (player) player.unpause();
    res.json({ status: 'ok' });
});

app.get('/health', (req, res) => {
    res.json({ status: 'ok', connections: connections.size, adapters: adapters.size, cachedStates: voiceStatesMap.size });
});

app.listen(PORT, '127.0.0.1', () => {
    console.log(`✅ Voice Server listening on http://127.0.0.1:${PORT}`);
});
