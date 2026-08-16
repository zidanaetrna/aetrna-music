const dns = require('dns');
const fs = require('fs');
const path = require('path');
dns.setDefaultResultOrder('ipv4first');

function getAbsoluteCookiesPath() {
    try {
        const possiblePaths = [
            process.env.COOKIES_PATH,
            process.env.COOKIES_PATH ? path.resolve(process.cwd(), process.env.COOKIES_PATH) : null,
            path.join(__dirname, '../cookies.txt'),
            path.join(__dirname, 'cookies.txt'),
            '/opt/aetrna-music/prod/cookies.txt',
            '/opt/aetrna-music/cookies.txt',
            './cookies.txt',
            '/app/cookies.txt'
        ].filter(Boolean);

        for (const p of possiblePaths) {
            if (fs.existsSync(p)) {
                return p;
            }
        }
    } catch (_) {}
    return null;
}

function getCookieHeaderString() {
    try {
        const cookiesFile = getAbsoluteCookiesPath();
        if (!cookiesFile) return '';
        const content = fs.readFileSync(cookiesFile, 'utf8');
        if (!content) return '';

        const lines = content.split('\n');
        const cookiePairs = [];

        for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed) continue;
            if (trimmed.startsWith('#') && !trimmed.startsWith('#HttpOnly_')) continue;
            const cleanLine = trimmed.startsWith('#HttpOnly_') ? trimmed.substring(10) : trimmed;
            const parts = cleanLine.split(/\s+/);
            if (parts.length >= 7) {
                const domain = parts[0].trim();
                const name = parts[5].trim();
                const value = parts[6].trim();
                if ((domain.includes('youtube') || domain.includes('googlevideo') || domain.includes('.com')) && name && value) {
                    cookiePairs.push(`${name}=${value}`);
                }
            }
        }
        return cookiePairs.join('; ');
    } catch (e) {
        return '';
    }
}

// Load DAVE E2EE protocol module before @discordjs/voice initializes
try {
    require('@snazzah/davey');
    console.log('[INFO] [VoiceServer] DAVE E2EE protocol (@snazzah/davey) loaded successfully');
} catch (e) {
    console.error('[WARN] [VoiceServer] DAVE E2EE protocol module failed to load:', e.message);
}

const express = require('express');
const { Client, GatewayIntentBits, REST, Routes, SlashCommandBuilder, PermissionFlagsBits } = require('discord.js');
const {
    joinVoiceChannel, createAudioPlayer, createAudioResource,
    AudioPlayerStatus, VoiceConnectionStatus, entersState, StreamType,
    generateDependencyReport
} = require('@discordjs/voice');
const { spawn } = require('child_process');
const http = require('http');
const sodium = require('libsodium-wrappers');
require('dotenv').config();

console.log('[INFO] [VoiceServer] Voice dependency report:');
console.log(generateDependencyReport());

(async () => {
    try {
        await sodium.ready;
        console.log('[INFO] [VoiceServer] libsodium-wrappers initialized');
    } catch (e) {
        console.error('[WARN] [VoiceServer] libsodium-wrappers warning:', e.message);
    }
})();

const commands = [
    new SlashCommandBuilder()
        .setName('play')
        .setDescription('Play a song from YouTube or Spotify')
        .addStringOption(opt => opt.setName('query').setDescription('Song name, YouTube URL, or Spotify link').setRequired(true)),
    new SlashCommandBuilder()
        .setName('search')
        .setDescription('Search for a song on YouTube')
        .addStringOption(opt => opt.setName('query').setDescription('Song name').setRequired(true)),
    new SlashCommandBuilder().setName('skip').setDescription('Skip the current song'),
    new SlashCommandBuilder().setName('stop').setDescription('Stop playback and clear queue'),
    new SlashCommandBuilder().setName('pause').setDescription('Pause playback'),
    new SlashCommandBuilder().setName('resume').setDescription('Resume playback'),
    new SlashCommandBuilder().setName('queue').setDescription('Show upcoming queue'),
    new SlashCommandBuilder().setName('nowplaying').setDescription('Show interactive Now Playing card'),
    new SlashCommandBuilder().setName('favorite').setDescription('Add current song to favorites'),
    new SlashCommandBuilder().setName('favorites').setDescription('List your favorite songs'),
    new SlashCommandBuilder()
        .setName('filter')
        .setDescription('Apply an audio filter')
        .addStringOption(opt => opt.setName('name').setDescription('Filter name (bassboost, nightcore, etc.)').setRequired(true)),
    new SlashCommandBuilder().setName('help').setDescription('Show help and command guide'),
    new SlashCommandBuilder().setName('stats').setDescription('Show bot system statistics'),
    new SlashCommandBuilder().setName('ping').setDescription('Check bot latency'),
    new SlashCommandBuilder()
        .setName('language')
        .setDescription('Set bot language for this server (Admin only)')
        .setDefaultMemberPermissions(PermissionFlagsBits.Administrator)
        .addStringOption(opt =>
            opt.setName('lang')
                .setDescription('Select server language')
                .setRequired(true)
                .addChoices(
                    { name: 'English 🇬🇧', value: 'en' },
                    { name: 'Bahasa Indonesia 🇮🇩', value: 'id' },
                    { name: 'Japanese 🇯🇵', value: 'jp' }
                )
        ),
].map(cmd => cmd.toJSON());

const connections = new Map();
const players = new Map();
const activeStreams = new Map();
const playSessions = new Map(); // guildId -> sessionId (integer)
const prefetchStreams = new Map(); // guildId -> { ytdlp, chunks: Buffer[] } pre-spawned yt-dlp for next track

function getFFmpegAudioFilter(filterName) {
    const baseLoudnorm = 'loudnorm=I=-16:TP=-1.5:LRA=11';
    switch ((filterName || 'none').toLowerCase()) {
        case 'bassboost':
            return `equalizer=f=60:width_type=h:width=50:g=10,${baseLoudnorm}`;
        case 'nightcore':
            return `asetrate=48000*1.25,aresample=48000,${baseLoudnorm}`;
        case 'vaporwave':
            return `asetrate=48000*0.8,aresample=48000,${baseLoudnorm}`;
        case '8d':
            return `apulsator=hz=0.125,${baseLoudnorm}`;
        case 'pop':
            return `equalizer=f=1000:width_type=h:width=200:g=4,${baseLoudnorm}`;
        case 'none':
        default:
            return baseLoudnorm;
    }
}

const discordClient = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates,
        GatewayIntentBits.GuildMessages,
        GatewayIntentBits.MessageContent,
    ]
});

const BOT_TOKEN = process.env.DISCORD_TOKEN;
const GO_BACKEND = 'http://127.0.0.1:47392/internal/interaction';

if (!BOT_TOKEN) {
    console.error('[ERROR] [VoiceServer] DISCORD_TOKEN is missing!');
    process.exit(1);
}

discordClient.on('raw', (packet) => {
    if (packet.t === 'VOICE_SERVER_UPDATE' || packet.t === 'VOICE_STATE_UPDATE') {
        console.log(`[DEBUG] [VoiceServer] Gateway event ${packet.t} for guild=${packet.d?.guild_id || packet.d?.guildId}`);
    }
});

discordClient.login(BOT_TOKEN).then(async () => {
    console.log(`[INFO] [VoiceServer] discord.js Client logged in as ${discordClient.user.tag}`);
    try {
        const rest = new REST({ version: '10' }).setToken(BOT_TOKEN);
        
        await rest.put(Routes.applicationCommands(discordClient.user.id), { body: commands });
        console.log('[INFO] [VoiceServer] Registered global slash commands successfully');

        for (const [gId, guild] of discordClient.guilds.cache) {
            try {
                await rest.put(Routes.applicationGuildCommands(discordClient.user.id, gId), { body: commands });
                console.log(`[INFO] [VoiceServer] Instant-registered guild slash commands for ${guild.name} (${gId})`);
            } catch (e) {
                console.error(`[WARN] [VoiceServer] Failed to register guild commands for ${gId}:`, e.message);
            }
        }
    } catch (e) {
        console.error('[WARN] [VoiceServer] Failed to register slash commands:', e.message);
    }
}).catch(err => {
    console.error('[ERROR] [VoiceServer] Login failed:', err.message);
    process.exit(1);
});

discordClient.on('guildCreate', async (guild) => {
    try {
        const rest = new REST({ version: '10' }).setToken(BOT_TOKEN);
        await rest.put(Routes.applicationGuildCommands(discordClient.user.id, guild.id), { body: commands });
        console.log(`[INFO] [VoiceServer] Instant-registered guild slash commands for new guild ${guild.name} (${guild.id})`);
    } catch (e) {
        console.error(`[WARN] [VoiceServer] Failed to register commands for new guild ${guild.id}:`, e.message);
    }
});

// Serialize interaction options recursively
function serializeOptions(options) {
    if (!options) return [];
    return options.map(opt => ({
        name: opt.name,
        type: opt.type,
        value: opt.value !== undefined ? opt.value : null,
        options: opt.options ? serializeOptions(opt.options) : [],
    }));
}

// Forward raw interaction data to Go Bot
function sendToGoBot(payload) {
    return new Promise((resolve) => {
        const body = JSON.stringify(payload);
        console.log(`[INFO] [VoiceServer] Forwarding ${payload.command_name || payload.custom_id || 'interaction'} to Go Bot`);
        const req = http.request(GO_BACKEND, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) },
            timeout: 12000,
        }, (res) => {
            console.log(`[INFO] [VoiceServer] Go Bot responded HTTP ${res.statusCode} for ${payload.command_name || payload.custom_id}`);
            res.resume();
            resolve();
        });
        req.on('timeout', () => {
            console.error(`[ERROR] [VoiceServer] Go Bot request timeout for ${payload.command_name || payload.custom_id}`);
            req.destroy();
            resolve();
        });
        req.on('error', (err) => {
            console.error(`[ERROR] [VoiceServer] Go Bot request error: ${err.message}`);
            resolve();
        });
        req.write(body);
        req.end();
    });
}

// Receive ALL interactions from Discord Gateway.
// Node.js ALWAYS defers first (guarantees Discord ack within 3s).
// Go Bot then edits the deferred message via REST (has 15 minutes).
discordClient.on('interactionCreate', async (interaction) => {
    if (!interaction.guildId) return;

    try {
        // Immediately acknowledge to Discord (prevents "application did not respond")
        if (interaction.isChatInputCommand()) {
            await interaction.deferReply();
        } else if (interaction.isButton() || interaction.isStringSelectMenu()) {
            // deferUpdate silently acknowledges the button click, no "thinking..." shown
            await interaction.deferUpdate();
        } else {
            return;
        }
    } catch (e) {
        console.error('[VoiceServer] Failed to defer interaction:', e.message);
        return;
    }

    const voiceChannel = interaction.member?.voice?.channel;
    const voiceChannelId = voiceChannel?.id || null;
    const voiceChannelMembers = voiceChannel ? voiceChannel.members.filter(m => !m.user.bot).size : 0;

    const payload = {
        id: interaction.id,
        token: interaction.token,
        application_id: interaction.applicationId,
        type: interaction.type,
        guild_id: interaction.guildId,
        channel_id: interaction.channelId,
        user_id: interaction.user.id,
        username: interaction.user.username,
        member_voice_channel_id: voiceChannelId,
        voice_channel_members: voiceChannelMembers,
        command_name: interaction.isChatInputCommand() ? interaction.commandName : null,
        options: interaction.isChatInputCommand() ? serializeOptions(interaction.options.data) : [],
        custom_id: (interaction.isButton() || interaction.isStringSelectMenu()) ? interaction.customId : null,
        message_id: (interaction.isButton() || interaction.isStringSelectMenu()) ? interaction.message?.id : null,
        values: interaction.isStringSelectMenu() ? interaction.values : [],
        is_admin: interaction.memberPermissions?.has(PermissionFlagsBits.Administrator) || false,
    };

    // Fire and forget — Go Bot has 15 minutes to edit the deferred message
    sendToGoBot(payload);
});

// Clean up streams for a guild
function cleanupStreams(guildId) {
    if (activeStreams.has(guildId)) {
        const stream = activeStreams.get(guildId);
        try { if (stream.ytdlp) stream.ytdlp.kill('SIGKILL'); } catch (e) {}
        try { if (stream.ffmpeg) stream.ffmpeg.kill('SIGKILL'); } catch (e) {}
        activeStreams.delete(guildId);
    }
}

// Kill any pre-fetched yt-dlp process for a guild
function cleanupPrefetch(guildId) {
    if (prefetchStreams.has(guildId)) {
        const pf = prefetchStreams.get(guildId);
        try { if (pf.ytdlp) pf.ytdlp.kill('SIGKILL'); } catch (e) {}
        prefetchStreams.delete(guildId);
    }
}

// Spawn yt-dlp in background for a YouTube URL to warm up player JS cache
function startPrefetch(guildId, youtubeUrl) {
    cleanupPrefetch(guildId);
    const ytdlpClients = process.env.YTDLP_CLIENTS || 'mweb';
    const cookiesPath = getAbsoluteCookiesPath();
    const userAgent = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36';
    const ytdlpArgs = [
        '-4',
        '--js-runtimes', 'node',
        '--extractor-args', `youtube:player_client=${ytdlpClients}`,
        '-f', '251/140/249/250/139/ba[ext=m4a]/ba[ext=webm]/ba/bestaudio',
        '--no-playlist',
        '--geo-bypass',
        '--no-check-certificates',
        '--no-warnings',
        '--user-agent', userAgent,
        '-o', '-',
        youtubeUrl
    ];
    if (cookiesPath) ytdlpArgs.unshift('--cookies', cookiesPath);

    console.log(`[INFO] [VoiceServer] Prefetching yt-dlp for '${youtubeUrl}' in guild ${guildId}`);
    const ytdlp = spawn('yt-dlp', ytdlpArgs, { stdio: ['ignore', 'pipe', 'pipe'] });
    const chunks = [];
    ytdlp.stdout.on('data', (chunk) => chunks.push(chunk));
    ytdlp.stderr.on('data', (d) => {
        const msg = d.toString().trim();
        if (msg) console.log(`[prefetch ${guildId}] ${msg}`);
    });
    ytdlp.on('error', () => {});
    prefetchStreams.set(guildId, { ytdlp, chunks, url: youtubeUrl });
}

// Notify Go Bot of track end
function notifyBotTrackEnd(guildId, reason) {
    const data = JSON.stringify({ guildId, reason });
    const req = http.request('http://127.0.0.1:47392/internal/track-end', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) },
    });
    req.on('error', () => {});
    req.write(data);
    req.end();
}

function createVoiceConnection(guildId, channelId) {
    let connection = connections.get(guildId);
    if (connection && connection.state.status === VoiceConnectionStatus.Ready) {
        if (connection.joinConfig.channelId === String(channelId)) {
            return connection;
        }
    }

    if (connection) {
        try { connection.destroy(); } catch (e) {}
        connections.delete(guildId);
    }

    let guild = discordClient.guilds.cache.get(guildId);
    if (!guild || !guild.voiceAdapterCreator) {
        throw new Error(`Guild ${guildId} missing or invalid voiceAdapterCreator`);
    }

    console.log(`🎙️ [VoiceServer] Creating fresh voice connection for channel ${channelId} in guild ${guild.name}...`);
    connection = joinVoiceChannel({
        channelId: String(channelId),
        guildId: String(guildId),
        adapterCreator: guild.voiceAdapterCreator,
        selfDeaf: true,
        selfMute: false,
    });

    connections.set(guildId, connection);

    connection.on('stateChange', (oldState, newState) => {
        console.log(`🔄 [VoiceServer] VoiceConnection ${guildId}: ${oldState.status} ➔ ${newState.status}`);

        const net = newState.networking || (oldState && oldState.networking);
        if (net && !net._listenersAttached) {
            net._listenersAttached = true;

            net.on('stateChange', (oldNetState, newNetState) => {
                console.log(`🌐 [VoiceServer NetState ${guildId}] ${oldNetState.code} (${oldNetState.status}) ➔ ${newNetState.code} (${newNetState.status})`);
                if (newNetState.ws) {
                    newNetState.ws.on('close', (eventOrCode, reason) => {
                        const code = (typeof eventOrCode === 'object' && eventOrCode !== null) ? eventOrCode.code : eventOrCode;
                        const rsn = (typeof eventOrCode === 'object' && eventOrCode !== null) ? eventOrCode.reason : reason;
                        console.log(`🔌 [VoiceServer WS Closed ${guildId}] Code: ${code}, Reason: "${rsn}"`, JSON.stringify(eventOrCode));
                    });
                    newNetState.ws.on('error', (err) => {
                        console.log(`❌ [VoiceServer WS Error ${guildId}]`, err.message);
                    });
                }
            });

            net.on('debug', (msg) => console.log(`🔍 [VoiceServer Debug ${guildId}]`, msg));
            net.on('error', (err) => console.log(`❌ [VoiceServer Error ${guildId}]`, err));
        }
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

    return connection;
}

// Express API for Go Bot to trigger audio playback
const app = express();
app.use(express.json());
const PORT = process.env.PORT || 3005;

app.post('/join-and-play', async (req, res) => {
    const { guildId, channelId, streamUrl, songUrl, nextSongUrl, volume = 1.0, filter = 'none' } = req.body;
    if (!guildId || !channelId || !streamUrl) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or streamUrl' });
    }

    try {
        const parsedUrl = new URL(streamUrl);
        if (parsedUrl.protocol !== 'http:' && parsedUrl.protocol !== 'https:') {
            return res.status(400).json({ error: 'Invalid streamUrl protocol' });
        }
    } catch (err) {
        return res.status(400).json({ error: 'Invalid streamUrl format' });
    }

    const currentSessionId = (playSessions.get(guildId) || 0) + 1;
    playSessions.set(guildId, currentSessionId);

    try {
        cleanupStreams(guildId);

        let guild = discordClient.guilds.cache.get(guildId);
        if (!guild) {
            try {
                await discordClient.guilds.fetch(guildId);
                guild = discordClient.guilds.cache.get(guildId);
            } catch (e) {}
        }
        if (!guild || !guild.voiceAdapterCreator) {
            return res.status(500).json({ error: `Missing voiceAdapterCreator for guild ${guildId}` });
        }

        // Always create a fresh voice connection negotiation so Request #2 executes the identical Discord Voice Gateway handshake as Request #1
        let connection = createVoiceConnection(guildId, channelId);

        // Return HTTP 200 immediately to Go Bot
        res.json({ status: 'ok', message: 'Voice connection initiated' });

        // Start playback asynchronously once connection reaches Ready with retry loop (Fixes Issue #11553)
        (async () => {
            try {
                let attempts = 0;
                while (connection.state.status !== VoiceConnectionStatus.Ready && attempts < 3) {
                    attempts++;
                    try {
                        console.log(`[INFO] [VoiceServer] Waiting for voice connection in guild ${guildId} (attempt ${attempts}/3)...`);
                        await entersState(connection, VoiceConnectionStatus.Ready, 8_000);
                    } catch (e) {
                        console.log(`[WARN] [VoiceServer] Voice connection attempt ${attempts} timed out in ${connection.state.status}. Retrying...`);
                        if (attempts < 3) {
                            await new Promise(r => setTimeout(r, 1000));
                            connection = createVoiceConnection(guildId, channelId);
                        } else {
                            throw e;
                        }
                    }
                }

                if (playSessions.get(guildId) !== currentSessionId) {
                    console.log(`[DEBUG] [VoiceServer] Session superceded during connection phase for guild ${guildId}`);
                    return;
                }

                console.log(`[INFO] [VoiceServer] Voice connection Ready for guild ${guildId}`);

                let player = players.get(guildId);
                if (player) {
                    player.removeAllListeners();
                    try { player.stop(); } catch (e) {}
                }

                player = createAudioPlayer();
                players.set(guildId, player);

                let streamPlaybackStartedAt = 0;

                player.on(AudioPlayerStatus.Playing, () => {
                    streamPlaybackStartedAt = Date.now();
                    console.log(`[INFO] [VoiceServer] AudioPlayer entered Playing state for guild ${guildId}`);
                });

                player.on(AudioPlayerStatus.Idle, () => {
                    if (playSessions.get(guildId) !== currentSessionId) {
                        console.log(`[DEBUG] [VoiceServer] Stale Idle ignored for guild ${guildId} (session ${currentSessionId})`);
                        return;
                    }
                    const durationMs = streamPlaybackStartedAt > 0 ? (Date.now() - streamPlaybackStartedAt) : 0;
                    console.log(`[INFO] [VoiceServer] Track ended for guild ${guildId} (session ${currentSessionId}, duration: ${durationMs}ms)`);
                    cleanupStreams(guildId);

                    if (streamPlaybackStartedAt === 0 || durationMs < 1500) {
                        console.warn(`[WARN] [VoiceServer] Track ended prematurely (${durationMs}ms). FFmpeg stream failed to play.`);
                        notifyBotTrackEnd(guildId, 'playback_failed');
                    } else {
                        notifyBotTrackEnd(guildId, 'finished');
                    }
                });

                player.on('error', (err) => {
                    if (playSessions.get(guildId) !== currentSessionId) return;
                    console.error(`[ERROR] [VoiceServer] Player error in guild ${guildId}:`, err.message);
                    cleanupStreams(guildId);
                    notifyBotTrackEnd(guildId, 'error');
                });

                const audioFilter = getFFmpegAudioFilter(filter);
                const userAgent = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36';
                let headerStr = `Referer: https://www.youtube.com/\r\n`;
                const cookieHeader = getCookieHeaderString();
                if (cookieHeader) {
                    headerStr += `Cookie: ${cookieHeader}\r\n`;
                }

                let ffmpeg;
                let ytdlp;

                // Always use yt-dlp pipe for YouTube streams — direct CDN URL gets IP-blocked 403 on track 2+
                const videoInputUrl = (songUrl && (songUrl.includes('youtube.com') || songUrl.includes('youtu.be'))) ? songUrl
                    : (streamUrl.includes('youtube.com') || streamUrl.includes('youtu.be')) ? streamUrl
                    : null;

                if (videoInputUrl) {
                    const ytdlpClients = process.env.YTDLP_CLIENTS || 'mweb';
                    const cookiesPath = getAbsoluteCookiesPath();

                    // Check if we have a prefetched yt-dlp process for this URL
                    const pf = prefetchStreams.get(guildId);
                    if (pf && pf.url === videoInputUrl && pf.ytdlp && pf.ytdlp.exitCode === null) {
                        console.log(`[INFO] [VoiceServer] Using prefetched yt-dlp for '${videoInputUrl}' in guild ${guildId}`);
                        ytdlp = pf.ytdlp;
                        prefetchStreams.delete(guildId);
                    } else {
                        cleanupPrefetch(guildId);
                        const ytdlpArgs = [
                            '-4',
                            '--js-runtimes', 'node',
                            '--extractor-args', `youtube:player_client=${ytdlpClients}`,
                            '-f', '251/140/249/250/139/ba[ext=m4a]/ba[ext=webm]/ba/bestaudio',
                            '--no-playlist',
                            '--geo-bypass',
                            '--no-check-certificates',
                            '--no-warnings',
                            '--user-agent', userAgent,
                            '-o', '-',
                            videoInputUrl
                        ];
                        if (cookiesPath) ytdlpArgs.unshift('--cookies', cookiesPath);

                        console.log(`[INFO] [VoiceServer] Spawning yt-dlp pipe for '${videoInputUrl}' in guild ${guildId}`);
                        ytdlp = spawn('yt-dlp', ytdlpArgs, { stdio: ['ignore', 'pipe', 'pipe'] });
                    }

                    ffmpeg = spawn('ffmpeg', [
                        '-i', 'pipe:0',
                        '-loglevel', 'warning',
                        '-af', audioFilter,
                        '-f', 's16le', '-ar', '48000', '-ac', '2', 'pipe:1'
                    ], { stdio: ['pipe', 'pipe', 'pipe'] });

                    ytdlp.stdout.pipe(ffmpeg.stdin);
                    ytdlp.stderr.on('data', (d) => { const msg = d.toString().trim(); if (msg) console.error(`[ytdlp ${guildId}] ${msg}`); });

                    // Start prefetching next song immediately so player JS is already cached when needed
                    if (nextSongUrl && (nextSongUrl.includes('youtube.com') || nextSongUrl.includes('youtu.be'))) {
                        setTimeout(() => startPrefetch(guildId, nextSongUrl), 3000);
                    }
                } else {
                    // Non-YouTube stream (e.g. SoundCloud, direct URL)
                    try {
                        const u = new URL(streamUrl);
                        console.log(`[DEBUG] [VoiceServer] Spawning direct FFmpeg | Guild: ${guildId} | Host: ${u.hostname}`);
                    } catch (_) {}

                    ffmpeg = spawn('ffmpeg', [
                        '-reconnect', '1', '-reconnect_streamed', '1', '-reconnect_delay_max', '5',
                        '-user_agent', userAgent,
                        '-headers', headerStr,
                        '-i', streamUrl,
                        '-loglevel', 'warning',
                        '-af', audioFilter,
                        '-f', 's16le', '-ar', '48000', '-ac', '2', 'pipe:1'
                    ], { stdio: ['ignore', 'pipe', 'pipe'] });
                }

                activeStreams.set(guildId, { ffmpeg, ytdlp });
                ffmpeg.stderr.on('data', (d) => { const msg = d.toString().trim(); if (msg) console.error(`[ffmpeg ${guildId}] ${msg}`); });
                ffmpeg.on('exit', (code, signal) => {
                    if (code !== 0 && code !== null && signal !== 'SIGKILL') {
                        console.error(`[WARN] [VoiceServer] FFmpeg process exited with code ${code} (signal: ${signal}) for guild ${guildId}`);
                    }
                });

                const resource = createAudioResource(ffmpeg.stdout, { inputType: StreamType.Raw, inlineVolume: true });
                resource.volume.setVolume(volume);

                connection.subscribe(player);
                player.play(resource);
            } catch (asyncErr) {
                if (playSessions.get(guildId) !== currentSessionId) return;
                console.error(`[ERROR] [VoiceServer] Async voice connection/playback failed for guild ${guildId}:`, asyncErr.message);
                cleanupStreams(guildId);
                notifyBotTrackEnd(guildId, 'connection_failed');
            }
        })();
    } catch (err) {
        console.error('[ERROR] [VoiceServer] Playback initiation error:', err.message);
        res.status(500).json({ error: err.message });
    }
});

app.post('/stop', (req, res) => {
    const { guildId } = req.body;
    cleanupStreams(guildId);
    const player = players.get(guildId);
    if (player) { player.stop(); players.delete(guildId); }
    const connection = connections.get(guildId);
    if (connection) { try { connection.destroy(); } catch (e) {} connections.delete(guildId); }
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

app.get('/test-voice', async (req, res) => {
    if (!discordClient.isReady()) {
        return res.status(503).json({ error: 'Discord client not ready yet' });
    }

    const { guildId } = req.query;
    let guild = guildId ? discordClient.guilds.cache.get(String(guildId)) : discordClient.guilds.cache.first();
    if (!guild) return res.json({ error: `Guild ${guildId || 'first'} not found` });

    const channel = guild.channels.cache.find(c => c.isVoiceBased());
    if (!channel) return res.json({ error: `No voice channels in guild ${guild.name}` });

    console.log(`🧪 [/test-voice] Starting diagnostic for guild ${guild.name} (${guild.id}) channel ${channel.name}...`);

    try {
        const connection = joinVoiceChannel({
            channelId: channel.id,
            guildId: guild.id,
            adapterCreator: guild.voiceAdapterCreator,
            selfDeaf: true,
        });

        connection.on('stateChange', (oldState, newState) => {
            console.log(`🔄 [/test-voice State] ${oldState.status} ➔ ${newState.status}`);

            const net = newState.networking || (oldState && oldState.networking);
            if (net && !net._listenersAttached) {
                net._listenersAttached = true;

                net.on('stateChange', (oldNetState, newNetState) => {
                    console.log(`🌐 [/test-voice NetState] ${oldNetState.code} (${oldNetState.status}) ➔ ${newNetState.code} (${newNetState.status})`);
                    if (newNetState.ws) {
                        newNetState.ws.on('close', (eventOrCode, reason) => {
                            const code = (typeof eventOrCode === 'object' && eventOrCode !== null) ? eventOrCode.code : eventOrCode;
                            const rsn = (typeof eventOrCode === 'object' && eventOrCode !== null) ? eventOrCode.reason : reason;
                            console.log(`🔌 [/test-voice Voice WS Closed!] Code: ${code}, Reason: "${rsn}"`, JSON.stringify(eventOrCode));
                        });
                        newNetState.ws.on('error', (err) => {
                            console.log(`❌ [/test-voice Voice WS Error!]`, err.message);
                        });
                    }
                });

                net.on('debug', (msg) => console.log('🔍 [Networking Debug]', msg));
                net.on('error', (err) => console.log('❌ [Networking Error]', err));
            }

            if (oldState.status === 'connecting' && newState.status === 'signalling') {
                console.log(`⚠️ [/test-voice Reset!] Connecting ➔ Signalling reset detected!`);
            }
        });

        connection.on('error', (err) => {
            console.error(`❌ [/test-voice Error]`, err);
        });

        connection.on('debug', (msg) => {
            console.log(`🔍 [/test-voice Debug]`, typeof msg === 'object' ? JSON.stringify(msg) : msg);
        });

        res.json({ status: 'initiated', guild: guild.name, channel: channel.name });
    } catch (e) {
        res.status(500).json({ error: e.message });
    }
});

app.listen(PORT, '127.0.0.1', () => {
    console.log(`✅ Voice Server listening on http://127.0.0.1:${PORT}`);
});
