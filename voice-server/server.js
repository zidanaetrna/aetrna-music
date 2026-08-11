const express = require('express');
const { joinVoiceChannel, createAudioPlayer, createAudioResource, AudioPlayerStatus, VoiceConnectionStatus, entersState } = require('@discordjs/voice');
const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const http = require('http');

const app = express();
app.use(express.json());

const PORT = process.env.PORT || 3000;
const BOT_WEBHOOK = process.env.BOT_WEBHOOK || 'http://127.0.0.1:8080/internal/track-end';

// Store connections and players per guild
const connections = new Map();
const players = new Map();
const activeStreams = new Map();

function cleanupStreams(guildId) {
    if (activeStreams.has(guildId)) {
        const { ytdlp, ffmpeg } = activeStreams.get(guildId);
        try { ytdlp.kill('SIGKILL'); } catch (e) {}
        try { ffmpeg.kill('SIGKILL'); } catch (e) {}
        activeStreams.delete(guildId);
    }
}

app.post('/play', async (req, res) => {
    const { guildId, channelId, url, volume = 1.0 } = req.body;
    if (!guildId || !channelId || !url) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or url' });
    }

    try {
        // Cleanup old streams for this guild
        cleanupStreams(guildId);

        // Get or create player
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

        // Get adapter from discord.js client instance
        let connection = connections.get(guildId);
        if (!connection || connection.joinConfig.channelId !== channelId) {
            if (connection) {
                try { connection.destroy(); } catch (e) {}
            }

            // We need voice adapter creator from Discord Gateway
            // Go bot sends voice state via manual join, or Node.js joins via client
            // Node.js joins directly using Discord Client instance
            return res.status(400).json({ error: 'Use /connect first or pass gateway payload' });
        }

        // Spawn yt-dlp stream
        const cookieFile = path.resolve(__dirname, '../cookies.txt');
        const useCookies = fs.existsSync(cookieFile);

        const ytdlpArgs = [
            ...(useCookies ? ['--cookies', cookieFile] : []),
            '--source-address', '2a02:c202:2234:4630::1',
            '--extractor-args', 'youtube:player_client=android,web',
            '-f', 'ba[ext=m4a]/ba[ext=webm]/bestaudio',
            '--no-playlist',
            '--geo-bypass',
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

        ytdlp.stdout.pipe(ffmpeg.stdin);
        activeStreams.set(guildId, { ytdlp, ffmpeg });

        const resource = createAudioResource(ffmpeg.stdout, {
            inputType: 'raw',
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
    res.json({ status: 'ok', connections: connections.size });
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
