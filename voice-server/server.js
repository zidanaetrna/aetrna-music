const express = require('express');
const { Client, GatewayIntentBits } = require('discord.js');
const {
    joinVoiceChannel, createAudioPlayer, createAudioResource,
    AudioPlayerStatus, VoiceConnectionStatus, entersState, StreamType
} = require('@discordjs/voice');
const { spawn, execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const http = require('http');

const app = express();
app.use(express.json());

const PORT = process.env.PORT || 3005;
const BOT_WEBHOOK = process.env.BOT_WEBHOOK || 'http://127.0.0.1:8080/internal/track-end';
const TOKEN = process.env.DISCORD_TOKEN;

if (!TOKEN) {
    console.error('❌ DISCORD_TOKEN not set. Exiting.');
    process.exit(1);
}

// Full discord.js client for DAVE-compatible voice — minimal intents, voice only
const client = new Client({
    intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildVoiceStates],
    presence: { status: 'invisible' },
});

let clientReady = false;
client.once('ready', () => {
    console.log(`✅ Voice Client logged in as ${client.user.tag}`);
    clientReady = true;
});

client.login(TOKEN).catch(err => {
    console.error('❌ Discord login failed:', err.message);
    process.exit(1);
});

const connections = new Map();
const players = new Map();
const activeStreams = new Map();

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

app.post('/play', async (req, res) => {
    const { guildId, channelId, url, volume = 1.0 } = req.body;
    if (!guildId || !channelId || !url) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or url' });
    }

    if (!clientReady) {
        return res.status(503).json({ error: 'Discord client not ready yet' });
    }

    try {
        cleanupStreams(guildId);

        // Lookup channel via full discord.js client (has guild cache from Guilds intent)
        const guild = await client.guilds.fetch(guildId);
        const channel = await guild.channels.fetch(channelId);
        if (!channel || !channel.isVoiceBased()) {
            return res.status(400).json({ error: 'Channel not found or not a voice channel' });
        }

        // Join or reuse existing connection — full client handles DAVE natively
        let connection = connections.get(guildId);
        if (!connection || connection.joinConfig.channelId !== channelId || connection.state.status === VoiceConnectionStatus.Destroyed) {
            if (connection) {
                try { connection.destroy(); } catch (e) {}
            }

            connection = joinVoiceChannel({
                channelId: channelId,
                guildId: guildId,
                adapterCreator: channel.guild.voiceAdapterCreator,
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

        // Cookie file check across standard paths
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

        // Step 1: Resolve actual stream URL using yt-dlp -g (fast, ~2s)
        const resolveArgs = [
            ...(useCookies ? ['--cookies', cookieFile] : []),
            '-f', 'bestaudio/best',
            '--no-playlist',
            '--geo-bypass',
            '--no-check-certificates',
            '--no-warnings',
            '-g',
            url
        ];

        let streamUrl;
        try {
            streamUrl = execFileSync('yt-dlp', resolveArgs, { env: spawnEnv, timeout: 30000 })
                .toString().trim().split('\n')[0];
        } catch (e) {
            console.error(`❌ [VoiceServer] yt-dlp resolve error: ${e.message}`);
            return res.status(500).json({ error: `yt-dlp resolve failed: ${e.message}` });
        }

        if (!streamUrl) return res.status(500).json({ error: 'yt-dlp returned empty stream URL' });
        console.log(`🔗 [VoiceServer] Resolved stream URL for guild ${guildId}`);

        // Step 2: Wait for voice connection Ready (discord.js client handles DAVE natively)
        try {
            console.log(`⏳ [VoiceServer] Waiting for voice connection Ready...`);
            await entersState(connection, VoiceConnectionStatus.Ready, 20_000);
            console.log(`✅ [VoiceServer] Voice connection Ready!`);
        } catch (stateErr) {
            console.error(`❌ [VoiceServer] Voice connection failed: ${stateErr.message}`);
            return res.status(500).json({ error: `Voice connection failed: ${stateErr.message}` });
        }

        // Step 3: Stream directly via ffmpeg (starts within <1 second)
        const ffmpegArgs = [
            '-reconnect', '1',
            '-reconnect_streamed', '1',
            '-reconnect_delay_max', '5',
            '-i', streamUrl,
            '-analyzeduration', '0',
            '-loglevel', 'error',
            '-f', 's16le',
            '-ar', '48000',
            '-ac', '2',
            'pipe:1'
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

// Legacy endpoint — no longer needed but kept for compatibility
app.post('/voice-state', (req, res) => {
    res.json({ status: 'ok', message: 'voice-state not needed with full client mode' });
});

app.get('/health', (req, res) => {
    res.json({ status: 'ok', ready: clientReady, connections: connections.size });
});

app.listen(PORT, '127.0.0.1', () => {
    console.log(`✅ Voice Server listening on http://127.0.0.1:${PORT}`);
});
