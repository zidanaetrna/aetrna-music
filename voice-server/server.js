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
const { Client, GatewayIntentBits, REST, Routes, SlashCommandBuilder } = require('discord.js');
const {
    joinVoiceChannel, createAudioPlayer, createAudioResource,
    AudioPlayerStatus, VoiceConnectionStatus, entersState, StreamType,
    generateDependencyReport
} = require('@discordjs/voice');
const { spawn } = require('child_process');
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

const commands = [
    new SlashCommandBuilder()
        .setName('play')
        .setDescription('Play a song from YouTube or Spotify')
        .addStringOption(option => option.setName('query').setDescription('Song name, YouTube URL, or Spotify link').setRequired(true)),
    new SlashCommandBuilder().setName('search').setDescription('Search for a song on YouTube')
        .addStringOption(option => option.setName('query').setDescription('Song name').setRequired(true)),
    new SlashCommandBuilder().setName('skip').setDescription('Skip the current song'),
    new SlashCommandBuilder().setName('stop').setDescription('Stop playback and clear queue'),
    new SlashCommandBuilder().setName('pause').setDescription('Pause playback'),
    new SlashCommandBuilder().setName('resume').setDescription('Resume playback'),
    new SlashCommandBuilder().setName('queue').setDescription('Show upcoming queue'),
    new SlashCommandBuilder().setName('nowplaying').setDescription('Show interactive Now Playing card'),
    new SlashCommandBuilder().setName('favorite').setDescription('Add current song to favorites'),
    new SlashCommandBuilder().setName('favorites').setDescription('List your favorite songs'),
    new SlashCommandBuilder().setName('filter').setDescription('Apply an audio filter')
        .addStringOption(option => option.setName('name').setDescription('Filter name').setRequired(true)),
    new SlashCommandBuilder().setName('help').setDescription('Show help and command guide'),
    new SlashCommandBuilder().setName('stats').setDescription('Show bot system statistics'),
    new SlashCommandBuilder().setName('ping').setDescription('Check bot latency')
].map(cmd => cmd.toJSON());

const connections = new Map();
const players = new Map();
const activeStreams = new Map();

const discordClient = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates,
        GatewayIntentBits.GuildMessages,
        GatewayIntentBits.MessageContent
    ]
});

const BOT_TOKEN = process.env.DISCORD_TOKEN;
const GO_BACKEND_URL = process.env.GO_BACKEND_URL || 'http://127.0.0.1:8080/internal/interaction';

if (BOT_TOKEN) {
    discordClient.login(BOT_TOKEN).then(async () => {
        console.log(`✅ [VoiceServer] discord.js Client logged in as ${discordClient.user.tag} (Single Gateway Client)`);
        try {
            const rest = new REST({ version: '10' }).setToken(BOT_TOKEN);
            console.log('⏳ Registering slash commands...');
            await rest.put(Routes.applicationCommands(discordClient.user.id), { body: commands });
            for (const [gId] of discordClient.guilds.cache) {
                try {
                    await rest.put(Routes.applicationGuildCommands(discordClient.user.id, gId), { body: commands });
                    console.log(`⚡ Instant guild commands registered for guild ${gId}`);
                } catch (e) {}
            }
            console.log('✅ Slash commands registered successfully!');
        } catch (e) {
            console.error('⚠️ Failed to register slash commands:', e.message);
        }
    }).catch(err => {
        console.error('❌ [VoiceServer] discord.js Client login failed:', err.message);
    });
} else {
    console.error('⚠️ [VoiceServer] DISCORD_TOKEN is missing in environment!');
}

function proxyInteractionToGoBackend(interactionPayload) {
    return new Promise((resolve, reject) => {
        const body = JSON.stringify(interactionPayload);
        const req = http.request(GO_BACKEND_URL, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(body)
            },
            timeout: 10000
        }, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => {
                try {
                    resolve(JSON.parse(data));
                } catch (e) {
                    resolve({ error: 'Failed to parse Go response' });
                }
            });
        });
        req.on('error', (err) => resolve({ error: err.message }));
        req.write(body);
        req.end();
    });
}

// Proxy Interaction Events (Slash commands & Button Clicks) to Golang Backend
discordClient.on('interactionCreate', async (interaction) => {
    try {
        const payload = {
            id: interaction.id,
            token: interaction.token,
            type: interaction.type,
            guild_id: interaction.guildId,
            channel_id: interaction.channelId,
            user_id: interaction.user.id,
            member_voice_channel_id: interaction.member?.voice?.channelId || null,
            data: interaction.data || null,
            custom_id: interaction.customId || null,
        };

        if (interaction.isChatInputCommand()) {
            payload.command_name = interaction.commandName;
            payload.options = interaction.options.data || [];
        }

        console.log(`📩 [VoiceServer] Proxying interaction '${payload.command_name || payload.custom_id}' to Golang Backend...`);

        if (interaction.isChatInputCommand()) {
            await interaction.deferReply().catch(() => {});
        } else if (interaction.isButton()) {
            await interaction.deferUpdate().catch(() => {});
        }

        const goResponse = await proxyInteractionToGoBackend(payload);

        if (goResponse) {
            if (goResponse.content || goResponse.embeds || goResponse.components) {
                const responseData = {};
                if (goResponse.content) responseData.content = goResponse.content;
                if (goResponse.embeds) responseData.embeds = goResponse.embeds;
                if (goResponse.components) responseData.components = goResponse.components;

                if (interaction.deferred || interaction.replied) {
                    await interaction.editReply(responseData).catch(() => {});
                } else {
                    await interaction.reply(responseData).catch(() => {});
                }
            }
        }
    } catch (err) {
        console.error('❌ Interaction handling error:', err.message);
    }
});

function cleanupStreams(guildId) {
    if (activeStreams.has(guildId)) {
        const stream = activeStreams.get(guildId);
        try { if (stream.ffmpeg) stream.ffmpeg.kill('SIGKILL'); } catch (e) {}
        activeStreams.delete(guildId);
    }
}

function notifyBotTrackEnd(guildId, reason) {
    const data = JSON.stringify({ guildId, reason });
    const req = http.request('http://127.0.0.1:8080/internal/track-end', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) }
    });
    req.on('error', () => {});
    req.write(data);
    req.end();
}

// Express API Server for Golang Play Requests
const app = express();
app.use(express.json());
const PORT = process.env.PORT || 3005;

app.post('/join-and-play', async (req, res) => {
    const { guildId, channelId, streamUrl, volume = 1.0 } = req.body;
    if (!guildId || !channelId || !streamUrl) {
        return res.status(400).json({ error: 'Missing guildId, channelId, or streamUrl' });
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

        console.log(`⏳ [VoiceServer] Waiting for voice connection Ready...`);
        await entersState(connection, VoiceConnectionStatus.Ready, 20_000);
        console.log(`✅ [VoiceServer] Native voice connection Ready!`);

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
