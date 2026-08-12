const dns = require('dns');
dns.setDefaultResultOrder('ipv4first');

const https = require('https');
const originalHttpsRequest = https.request;
https.request = function(url, options, callback) {
    const targetUrl = typeof url === 'string' ? url : (url && url.href ? url.href : '');
    const isDiscordMedia = targetUrl.includes('discord.media') ||
                           (options && options.host && options.host.includes('discord.media')) ||
                           (url && url.host && url.host.includes('discord.media'));
    if (isDiscordMedia) {
        if (typeof url === 'object' && url !== null) {
            url.headers = url.headers || {};
            url.headers['User-Agent'] = 'DiscordBot (https://github.com/discordjs/discord.js, 14.16.3)';
        }
        if (options && typeof options === 'object') {
            options.headers = options.headers || {};
            options.headers['User-Agent'] = 'DiscordBot (https://github.com/discordjs/discord.js, 14.16.3)';
        }
    }
    return originalHttpsRequest.call(this, url, options, callback);
};

const express = require('express');
const { Client, GatewayIntentBits } = require('discord.js');
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
require('dotenv').config();

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

// State stores
const connections = new Map();
const players = new Map();
const activeStreams = new Map();

// Dedicated discord.js Client for Gateway Voice management ONLY
const discordClient = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates
    ]
});

const BOT_TOKEN = process.env.DISCORD_TOKEN;
if (BOT_TOKEN) {
    discordClient.login(BOT_TOKEN).then(() => {
        console.log(`✅ [VoiceServer] discord.js Client logged in as ${discordClient.user.tag} (Dedicated Voice Gateway)`);
    }).catch(err => {
        console.error('❌ [VoiceServer] discord.js Client login failed:', err.message);
    });
} else {
    console.error('⚠️ [VoiceServer] DISCORD_TOKEN is missing in environment!');
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
            const lines = stdout.trim().split('\n').filter(Boolean);
            if (!lines.length) return reject(new Error('yt-dlp returned empty output'));
            resolve(lines);
        });
    });
}

// Express Server API for Go Bot
const app = express();
app.use(express.json());

const PORT = process.env.PORT || 3005;
const BOT_WEBHOOK = process.env.BOT_WEBHOOK || 'http://127.0.0.1:8080/internal/track-end';

app.post('/play', async (req, res) => {
    const { guildId, channelId, url, volume = 1.0 } = req.body;
    if (!guildId || !channelId || !url) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or url' });
    }

    try {
        cleanupStreams(guildId);

        let connection = connections.get(guildId);
        if (!connection || connection.joinConfig.channelId !== channelId || connection.state.status === VoiceConnectionStatus.Destroyed) {
            if (connection) { try { connection.destroy(); } catch (e) {} }

            console.log(`🎙️ [VoiceServer] Joining voice channel ${channelId} in guild ${guildId} via native discord.js adapter...`);

            let guild = discordClient.guilds.cache.get(guildId);
            if (!guild) {
                guild = await discordClient.guilds.fetch(guildId).catch(() => null);
            }
            if (!guild) throw new Error(`Guild ${guildId} not found in discord.js client cache`);

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

            connection.on('error', (error) => {
                console.error(`❌ [VoiceServer ConnectionError ${guildId}]`, error);
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

        const spawnEnv = {
            ...process.env,
            HOME: '/root',
            PATH: (process.env.PATH || '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin') + ':/root/.deno/bin:/root/.local/bin',
        };

        const resolveArgs = [
            ...(useCookies ? ['--cookies', cookieFile] : []),
            '-f', 'bestaudio/best',
            '--no-playlist', '--geo-bypass', '--no-check-certificates', '--no-warnings',
            '--get-url',
            url
        ];

        const resolvePromise = resolveStreamUrl(resolveArgs, spawnEnv);

        console.log(`⏳ [VoiceServer] Waiting for voice connection Ready...`);
        await entersState(connection, VoiceConnectionStatus.Ready, 20_000);
        console.log(`✅ [VoiceServer] Native voice connection Ready!`);

        const streamUrlLines = await resolvePromise;
        const streamUrl = streamUrlLines[0];
        console.log(`🔗 [VoiceServer] Resolved stream URL for guild ${guildId}`);

        let player = players.get(guildId);
        if (!player) {
            player = createAudioPlayer();
            players.set(guildId, player);

            player.on(AudioPlayerStatus.Idle, () => {
                console.log(`🎵 [VoiceServer] Track finished for guild ${guildId}`);
                cleanupStreams(guildId);
                notifyBotTrackEnd(guildId, 'finished');
            });

            player.on('error', (err) => {
                console.error(`❌ [VoiceServer] Player error in guild ${guildId}:`, err.message);
                cleanupStreams(guildId);
                notifyBotTrackEnd(guildId, 'error');
            });
        }

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
        console.error('❌ Playback error:', err.message);
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

app.get('/health', (req, res) => {
    res.json({ status: 'ok', clientReady: discordClient.isReady(), connections: connections.size });
});

app.listen(PORT, '127.0.0.1', () => {
    console.log(`✅ Voice Server listening on http://127.0.0.1:${PORT}`);
});
