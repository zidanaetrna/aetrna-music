const dns = require('dns');
dns.setDefaultResultOrder('ipv4first');

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
const sodium = require('libsodium-wrappers');

console.log('📋 [VoiceServer] Voice dependency report:');
console.log(generateDependencyReport());

(async () => {
    try {
        await sodium.ready;
        console.log('✅ [VoiceServer] libsodium-wrappers initialized!');
    } catch (e) {
        console.error('⚠️ [VoiceServer] libsodium-wrappers warning:', e.message);
    }
})();

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

function cleanEndpointString(endpoint) {
    if (!endpoint) return undefined;
    return endpoint.replace(/^wss?:\/\//, '').split(':')[0].trim();
}

function createCustomAdapter(guildId) {
    return (methods) => {
        adapters.set(guildId, methods);

        return {
            sendPayload(data) {
                if (data && data.d && data.d.channel_id) {
                    const current = voiceStatesMap.get(guildId) || {};
                    current.channelId = data.d.channel_id;
                    voiceStatesMap.set(guildId, current);
                }

                // Forward OP4 Voice State payload from @discordjs/voice to Go Bot Gateway session
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

// Receive Gateway Voice Credentials from Go Bot
app.post('/voice-state', (req, res) => {
    const { guildId, channelId, token, endpoint, sessionId, userId } = req.body;
    if (!guildId) return res.status(400).json({ error: 'Missing guildId' });

    const cleanEndpoint = cleanEndpointString(endpoint);

    const current = voiceStatesMap.get(guildId) || {};
    if (token) current.token = token;
    if (cleanEndpoint) current.endpoint = cleanEndpoint;
    if (sessionId) current.sessionId = sessionId;
    if (userId) current.userId = userId;
    if (channelId) current.channelId = channelId;
    voiceStatesMap.set(guildId, current);

    const adapter = adapters.get(guildId);
    if (adapter) {
        if (current.token && current.endpoint && current.sessionId && current.userId) {
            console.log(`🔑 [VoiceServer] Applying Voice State: User=${current.userId}, Session=${current.sessionId}, Endpoint=${current.endpoint}, Channel=${current.channelId}`);
            
            // Step A: Send Voice State Update FIRST so @discordjs/voice registers userId and sessionId
            adapter.onVoiceStateUpdate({
                session_id: current.sessionId,
                channel_id: current.channelId,
                user_id: current.userId,
                guild_id: guildId
            });

            // Step B: Wait 50ms for @discordjs/voice state machine tick, THEN send Voice Server Update
            setTimeout(() => {
                adapter.onVoiceServerUpdate({
                    token: current.token,
                    endpoint: current.endpoint,
                    guild_id: guildId
                });
                console.log(`✅ [VoiceServer] Applied Voice Server Update to adapter for guild ${guildId}`);
            }, 50);

            return res.json({ status: 'ok', updated: true });
        }
        console.log(`⏳ [VoiceServer] Partial voice state received for guild ${guildId}`);
        return res.json({ status: 'ok', updated: false, pending: true });
    }

    console.log(`🔑 [VoiceServer] Voice state cached for guild ${guildId}`);
    res.json({ status: 'ok', updated: false, cached: true });
});

app.post('/play', async (req, res) => {
    const { guildId, channelId, url, volume = 1.0 } = req.body;
    if (!guildId || !channelId || !url) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or url' });
    }

    try {
        cleanupStreams(guildId);

        // Reset voice state cache for fresh join request
        voiceStatesMap.delete(guildId);
        const current = { channelId: String(channelId) };
        voiceStatesMap.set(guildId, current);

        let connection = connections.get(guildId);
        if (!connection || connection.joinConfig.channelId !== channelId || connection.state.status === VoiceConnectionStatus.Destroyed) {
            if (connection) {
                try { connection.destroy(); } catch (e) {}
            }

            console.log(`🎙️ [VoiceServer] Joining voice channel ${channelId} in guild ${guildId}...`);
            connection = joinVoiceChannel({
                channelId: String(channelId),
                guildId: String(guildId),
                adapterCreator: createCustomAdapter(String(guildId)),
                selfDeaf: true,
                selfMute: false,
            });

            connections.set(guildId, connection);

            connection.on('stateChange', (oldState, newState) => {
                console.log(`🔄 [VoiceServer] VoiceConnection ${guildId} state: ${oldState.status} ➔ ${newState.status}`);
                if (newState.closeCode || newState.reason) {
                    console.log(`⚠️ [VoiceServer CloseDetails ${guildId}] CloseCode: ${newState.closeCode}, Reason: ${JSON.stringify(newState.reason || {})}`);
                }
            });

            connection.on('error', (error) => {
                console.error(`❌ [VoiceServer ConnectionError ${guildId}]`, error);
            });

            connection.on('debug', (msg) => {
                console.log(`🐛 [VoiceServer Debug ${guildId}] ${msg}`);
            });

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

        // Start yt-dlp URL resolution in parallel
        const resolvePromise = resolveStreamUrl(resolveArgs, spawnEnv);

        // Wait for Voice Connection Ready
        try {
            console.log(`⏳ [VoiceServer] Waiting for voice connection Ready...`);
            await entersState(connection, VoiceConnectionStatus.Ready, 20_000);
            console.log(`✅ [VoiceServer] Voice connection Ready!`);
        } catch (stateErr) {
            console.error(`❌ [VoiceServer] Voice connection failed: ${stateErr.message}`);
            return res.status(500).json({ error: `Voice connection failed: ${stateErr.message}` });
        }

        // Await stream URL from yt-dlp
        let streamUrl;
        try {
            streamUrl = await resolvePromise;
            console.log(`🔗 [VoiceServer] Resolved stream URL for guild ${guildId}`);
        } catch (e) {
            console.error(`❌ [VoiceServer] yt-dlp resolve error: ${e.message}`);
            return res.status(500).json({ error: `yt-dlp resolve failed: ${e.message}` });
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

        // Stream directly via ffmpeg
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
