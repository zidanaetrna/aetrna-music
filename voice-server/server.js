const express = require('express');
const { joinVoiceChannel, createAudioPlayer, createAudioResource, AudioPlayerStatus, VoiceConnectionStatus, entersState, StreamType } = require('@discordjs/voice');
const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const http = require('http');

const app = express();
app.use(express.json());

const PORT = process.env.PORT || 3005;
const BOT_WEBHOOK = process.env.BOT_WEBHOOK || 'http://127.0.0.1:8080/internal/track-end';

// Custom Voice Adapters map
const adapters = new Map();
const connections = new Map();
const players = new Map();
const activeStreams = new Map();

function createCustomAdapter(guildId) {
    return (methods) => {
        adapters.set(guildId, methods);
        return {
            sendPayload(data) {
                // Gateway payload sent by voice connection if needed
                return true;
            },
            destroy() {
                adapters.delete(guildId);
            }
        };
    };
}

function cleanupStreams(guildId) {
    if (activeStreams.has(guildId)) {
        const { ytdlp, ffmpeg } = activeStreams.get(guildId);
        try { ytdlp.kill('SIGKILL'); } catch (e) {}
        try { ffmpeg.kill('SIGKILL'); } catch (e) {}
        activeStreams.delete(guildId);
    }
}

// Receive Gateway Voice Updates from Go Bot
app.post('/voice-state', (req, res) => {
    const { guildId, token, endpoint, sessionId, userId } = req.body;
    const adapter = adapters.get(guildId);

    if (adapter) {
        if (token && endpoint) {
            adapter.onVoiceServerUpdate({ token, endpoint, guild_id: guildId });
        }
        if (sessionId && userId) {
            adapter.onVoiceStateUpdate({ session_id: sessionId, guild_id: guildId, user_id: userId });
        }
        return res.json({ status: 'ok', updated: true });
    }
    res.json({ status: 'ok', updated: false, message: 'Adapter not registered yet' });
});

app.post('/play', async (req, res) => {
    const { guildId, channelId, url, volume = 1.0, token, endpoint, sessionId, userId } = req.body;
    if (!guildId || !channelId || !url) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or url' });
    }

    try {
        cleanupStreams(guildId);

        // Join voice channel with custom adapter
        let connection = connections.get(guildId);
        if (!connection || connection.joinConfig.channelId !== channelId) {
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
                } catch (error) {
                    try { connection.destroy(); } catch (e) {}
                    connections.delete(guildId);
                }
            });
        }

        // Feed voice state if provided in request
        const adapter = adapters.get(guildId);
        if (adapter && token && endpoint && sessionId && userId) {
            adapter.onVoiceServerUpdate({ token, endpoint, guild_id: guildId });
            adapter.onVoiceStateUpdate({ session_id: sessionId, guild_id: guildId, user_id: userId });
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

        // Cookie file check across standard paths
        let cookieFile = path.resolve(__dirname, '../cookies.txt');
        if (!fs.existsSync(cookieFile)) {
            cookieFile = '/opt/aetrna-music/prod/cookies.txt';
        }
        if (!fs.existsSync(cookieFile)) {
            cookieFile = path.resolve(__dirname, './cookies.txt');
        }
        const useCookies = fs.existsSync(cookieFile);
        if (useCookies) {
            console.log(`🔑 [VoiceServer] Found cookies file at: ${cookieFile}`);
        }

        const ytdlpArgs = [
            '--source-address', '2a02:c202:2234:4630::1',
            '--user-agent', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
            '--add-header', 'Accept-Language:en-US,en;q=0.9',
            '-f', 'bestaudio/best',
            '--no-playlist',
            '--geo-bypass',
            '--no-check-certificates',
            '--no-warnings',
            '-o', '-',
            url
        ];

        const ffmpegArgs = [
            '-i', 'pipe:0',
            '-analyzeduration', '0',
            '-loglevel', '0',
            '-f', 's16le',
            '-ar', '48000',
            '-ac', '2',
            'pipe:1'
        ];

        const ytdlp = spawn('yt-dlp', ytdlpArgs, { stdio: ['ignore', 'pipe', 'pipe'] });
        const ffmpeg = spawn('ffmpeg', ffmpegArgs, { stdio: ['pipe', 'pipe', 'pipe'] });

        ytdlp.stderr.on('data', (d) => {
            const msg = d.toString().trim();
            if (msg) console.log(`[yt-dlp] ${msg}`);
        });

        ytdlp.stdout.pipe(ffmpeg.stdin);
        activeStreams.set(guildId, { ytdlp, ffmpeg });

        const resource = createAudioResource(ffmpeg.stdout, {
            inputType: StreamType.Raw,
            inlineVolume: true
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
    if (player) {
        player.stop();
        players.delete(guildId);
    }

    const connection = connections.get(guildId);
    if (connection) {
        try { connection.destroy(); } catch (e) {}
        connections.delete(guildId);
    }
    adapters.delete(guildId);

    res.json({ status: 'ok' });
});

app.post('/pause', (req, res) => {
    const { guildId } = req.body;
    const player = players.get(guildId);
    if (player) player.pause();
    res.json({ status: 'ok' });
});

app.post('/resume', (req, res) => {
    const { guildId } = req.body;
    const player = players.get(guildId);
    if (player) player.unpause();
    res.json({ status: 'ok' });
});

app.get('/health', (req, res) => {
    res.json({ status: 'ok', connections: connections.size, adapters: adapters.size });
});

function notifyBotTrackEnd(guildId, reason) {
    const data = JSON.stringify({ guildId, reason });
    const req = http.request(BOT_WEBHOOK, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
    });
    req.on('error', () => {});
    req.write(data);
    req.end();
}

app.listen(PORT, '127.0.0.1', () => {
    console.log(`✅ Voice Server listening on http://127.0.0.1:${PORT}`);
});
