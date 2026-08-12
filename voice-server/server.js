const express = require('express');
const { Client, GatewayIntentBits } = require('discord.js');
const {
    joinVoiceChannel, createAudioPlayer, createAudioResource,
    AudioPlayerStatus, VoiceConnectionStatus, entersState, StreamType,
    generateDependencyReport, getVoiceConnection
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
const DISCORD_TOKEN = process.env.DISCORD_TOKEN;

const client = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates,
    ]
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

// Dummy handler for legacy compatibility
app.post('/voice-state', (req, res) => {
    res.json({ status: 'ok', note: 'discord.js client handles voice states natively' });
});

app.post('/play', async (req, res) => {
    const { guildId, channelId, url, volume = 1.0 } = req.body;
    if (!guildId || !channelId || !url) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or url' });
    }

    try {
        cleanupStreams(guildId);

        const guild = client.guilds.cache.get(guildId) || await client.guilds.fetch(guildId).catch(() => null);
        if (!guild) {
            console.error(`❌ [VoiceServer] Guild ${guildId} not found in discord.js client cache`);
            return res.status(404).json({ error: `Guild ${guildId} not found` });
        }

        // Join Voice Channel natively via guild.voiceAdapterCreator
        let connection = getVoiceConnection(guildId);
        if (!connection || connection.joinConfig.channelId !== channelId || connection.state.status === VoiceConnectionStatus.Destroyed) {
            console.log(`🎙️ [VoiceServer] Joining voice channel ${channelId} in guild ${guildId} via native adapter...`);
            connection = joinVoiceChannel({
                channelId: String(channelId),
                guildId: String(guildId),
                adapterCreator: guild.voiceAdapterCreator,
                selfDeaf: true,
                selfMute: false,
            });

            connections.set(guildId, connection);

            connection.on('stateChange', (oldState, newState) => {
                console.log(`🔄 [VoiceServer] VoiceConnection ${guildId} state: ${oldState.status} ➔ ${newState.status}`);
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

        // Wait for Native Voice Connection Ready
        try {
            console.log(`⏳ [VoiceServer] Waiting for native voice connection Ready...`);
            await entersState(connection, VoiceConnectionStatus.Ready, 15_000);
            console.log(`✅ [VoiceServer] Native Voice connection Ready!`);
        } catch (stateErr) {
            console.error(`❌ [VoiceServer] Native voice connection failed: ${stateErr.message}`);
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

    const connection = getVoiceConnection(guildId) || connections.get(guildId);
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

app.get('/health', (req, res) => {
    res.json({ status: 'ok', botReady: client.isReady(), connections: connections.size, players: players.size });
});

client.once('ready', () => {
    console.log(`✅ [VoiceServer] discord.js Client logged in as ${client.user.tag}`);
});

if (DISCORD_TOKEN) {
    client.login(DISCORD_TOKEN).catch((err) => {
        console.error(`❌ [VoiceServer] discord.js Client login failed: ${err.message}`);
    });
} else {
    console.warn(`⚠️ [VoiceServer] DISCORD_TOKEN is missing in environment variables.`);
}

app.listen(PORT, '127.0.0.1', () => {
    console.log(`✅ Voice Server listening on http://127.0.0.1:${PORT}`);
});
