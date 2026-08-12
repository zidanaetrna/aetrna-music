const dns = require('dns');
dns.setDefaultResultOrder('ipv4first');

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
].map(cmd => cmd.toJSON());

const connections = new Map();
const players = new Map();
const activeStreams = new Map();

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
    console.error('❌ [VoiceServer] DISCORD_TOKEN is missing!');
    process.exit(1);
}

discordClient.login(BOT_TOKEN).then(async () => {
    console.log(`✅ [VoiceServer] discord.js Client logged in as ${discordClient.user.tag} (Single Gateway Session)`);
    try {
        const rest = new REST({ version: '10' }).setToken(BOT_TOKEN);
        
        // Clear ALL global commands to prevent duplicates from old Go Bot registrations
        await rest.put(Routes.applicationCommands(discordClient.user.id), { body: [] });
        console.log('🗑️ Cleared global slash commands');
        
        // Register ONLY guild-specific commands (instant, no propagation delay, no duplicates)
        for (const [gId] of discordClient.guilds.cache) {
            try {
                await rest.put(Routes.applicationGuildCommands(discordClient.user.id, gId), { body: commands });
                console.log(`⚡ Guild commands registered for guild ${gId}`);
            } catch (e) {
                console.error(`⚠️ Failed for guild ${gId}:`, e.message);
            }
        }
        console.log('✅ Slash commands registered!');
    } catch (e) {
        console.error('⚠️ Failed to register slash commands:', e.message);
    }
}).catch(err => {
    console.error('❌ [VoiceServer] Login failed:', err.message);
    process.exit(1);
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
        console.log(`📬 [VoiceServer] Sending ${payload.command_name || payload.custom_id || 'interaction'} to Go Bot...`);
        const req = http.request(GO_BACKEND, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(body) },
            timeout: 12000,
        }, (res) => {
            console.log(`✅ [VoiceServer] Go Bot responded HTTP ${res.statusCode} for ${payload.command_name || payload.custom_id}`);
            res.resume();
            resolve();
        });
        req.on('timeout', () => {
            console.error(`⏰ [VoiceServer] Go Bot timeout for ${payload.command_name || payload.custom_id}`);
            req.destroy();
            resolve();
        });
        req.on('error', (err) => {
            console.error(`❌ [VoiceServer] Go Bot request error: ${err.message} (is bot.service running on :47392?)`);
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

    const voiceChannelId = interaction.member?.voice?.channelId || null;

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
        command_name: interaction.isChatInputCommand() ? interaction.commandName : null,
        options: interaction.isChatInputCommand() ? serializeOptions(interaction.options.data) : [],
        custom_id: (interaction.isButton() || interaction.isStringSelectMenu()) ? interaction.customId : null,
        values: interaction.isStringSelectMenu() ? interaction.values : [],
    };

    // Fire and forget — Go Bot has 15 minutes to edit the deferred message
    sendToGoBot(payload);
});

// Clean up streams for a guild
function cleanupStreams(guildId) {
    if (activeStreams.has(guildId)) {
        const stream = activeStreams.get(guildId);
        try { if (stream.ffmpeg) stream.ffmpeg.kill('SIGKILL'); } catch (e) {}
        activeStreams.delete(guildId);
    }
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

// Express API for Go Bot to trigger audio playback
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
        const isDestroyed = !connection || connection.state.status === VoiceConnectionStatus.Destroyed;
        const isWrongChannel = connection && connection.joinConfig.channelId !== channelId;

        if (isDestroyed || isWrongChannel) {
            if (connection) { try { connection.destroy(); } catch (e) {} }

            let guild = discordClient.guilds.cache.get(guildId);
            if (!guild) guild = await discordClient.guilds.fetch(guildId).catch(() => null);
            if (!guild) throw new Error(`Guild ${guildId} not found`);

            console.log(`🎙️ [VoiceServer] Joining voice channel ${channelId} in guild ${guildId}...`);
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
            });

            connection.on('error', (err) => {
                console.error(`❌ [VoiceServer] Connection error in ${guildId}:`, err.message);
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
                console.error(`❌ [VoiceServer] Player error in ${guildId}:`, err.message);
                cleanupStreams(guildId);
                notifyBotTrackEnd(guildId, 'error');
            });
        }

        const ffmpeg = spawn('ffmpeg', [
            '-reconnect', '1', '-reconnect_streamed', '1', '-reconnect_delay_max', '5',
            '-i', streamUrl,
            '-analyzeduration', '0', '-loglevel', 'error',
            '-f', 's16le', '-ar', '48000', '-ac', '2', 'pipe:1'
        ], { stdio: ['ignore', 'pipe', 'pipe'] });

        activeStreams.set(guildId, { ffmpeg });
        ffmpeg.stderr.on('data', (d) => { const msg = d.toString().trim(); if (msg) console.log(`[ffmpeg] ${msg}`); });

        const resource = createAudioResource(ffmpeg.stdout, { inputType: StreamType.Raw, inlineVolume: true });
        resource.volume.setVolume(volume);

        player.play(resource);
        connection.subscribe(player);

        res.json({ status: 'ok' });
    } catch (err) {
        console.error('❌ [VoiceServer] Playback error:', err.message);
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

app.listen(PORT, '127.0.0.1', () => {
    console.log(`✅ Voice Server listening on http://127.0.0.1:${PORT}`);
});
