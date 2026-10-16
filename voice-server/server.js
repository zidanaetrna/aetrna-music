const dns = require('dns');
const fs = require('fs');
const path = require('path');
const express = require('express');
const { Client, GatewayIntentBits, REST, Routes } = require('discord.js');
const { VoiceConnectionStatus } = require('@discordjs/voice');
const http = require('http');
const sodium = require('libsodium-wrappers');
require('dotenv').config();

const { commands } = require('./discord/commands');
const { initVersionCheck } = require('./services/updater');
const createApiRouter = require('./routes/api');

dns.setDefaultResultOrder('ipv4first');

// Load DAVE E2EE protocol module before @discordjs/voice initializes
try {
    require('@snazzah/davey');
    console.log('[INFO] [VoiceServer] DAVE E2EE protocol (@snazzah/davey) loaded successfully');
} catch (e) {
    console.error('[WARN] [VoiceServer] DAVE E2EE protocol module failed to load:', e.message);
}

(async () => {
    try {
        await sodium.ready;
        console.log('[INFO] [VoiceServer] libsodium-wrappers initialized');
    } catch (e) {
        console.error('[WARN] [VoiceServer] libsodium-wrappers warning:', e.message);
    }
})();

let VOICE_SERVER_VERSION = 'v2.1.9';
try {
    const pkg = require('./package.json');
    if (pkg && pkg.version) VOICE_SERVER_VERSION = `v${pkg.version}`;
} catch (_) {}

initVersionCheck(VOICE_SERVER_VERSION);

const connections = new Map();
const players = new Map();
const activeStreams = new Map();
const audioResources = new Map();
const playSessions = new Map();

const app = express();
app.use(express.json());

const VOICE_PORT = process.env.VOICE_PORT || 3005;
const GO_IPC_PORT = process.env.GO_IPC_PORT || 47392;
const INTERNAL_IPC_TOKEN = process.env.INTERNAL_IPC_TOKEN || 'aetrna-internal-ipc-token-dev-2026';

// Middleware for IPC Token authentication between Go core and Node voice worker
app.use((req, res, next) => {
    if (req.path === '/health') return next();
    const token = req.headers['x-internal-ipc-token'];
    if (INTERNAL_IPC_TOKEN && token !== INTERNAL_IPC_TOKEN) {
        console.warn(`[WARN] [VoiceServer] Unauthorized IPC request to ${req.path} from ${req.ip}`);
        return res.status(401).json({ error: 'Unauthorized IPC token' });
    }
    next();
});

const client = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates,
        GatewayIntentBits.GuildMessages,
        GatewayIntentBits.MessageContent,
    ]
});

function cleanupStreams(guildId) {
    const streams = activeStreams.get(guildId);
    if (streams) {
        if (streams.ffmpeg) { try { streams.ffmpeg.kill('SIGKILL'); } catch (e) {} }
        if (streams.ytdlp) { try { streams.ytdlp.kill('SIGKILL'); } catch (e) {} }
        activeStreams.delete(guildId);
    }
    audioResources.delete(guildId);
}

function notifyBotInteraction(interactionData) {
    const data = JSON.stringify(interactionData);
    const req = http.request(`http://127.0.0.1:${GO_IPC_PORT}/internal/interaction`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Content-Length': Buffer.byteLength(data),
            'X-Internal-IPC-Token': INTERNAL_IPC_TOKEN,
        },
        timeout: 5000,
    }, (res) => {
        res.resume();
    });
    req.on('error', (err) => {
        console.error(`[ERROR] [VoiceServer] Failed to forward interaction to Go bot:`, err.message);
    });
    req.write(data);
    req.end();
}

client.once('ready', async () => {
    console.log(`[INFO] [VoiceServer] Discord Gateway Connected as ${client.user.tag} (${client.user.id})`);
    try {
        const rest = new REST({ version: '10' }).setToken(process.env.DISCORD_TOKEN);
        console.log('[INFO] [VoiceServer] Registering global slash commands...');
        await rest.put(Routes.applicationCommands(client.user.id), { body: commands });
        console.log('[INFO] [VoiceServer] Global slash commands registered successfully');
    } catch (e) {
        console.error('[ERROR] [VoiceServer] Failed to register slash commands:', e.message);
    }
});

client.on('interactionCreate', async (interaction) => {
    try {
        if (!interaction.isChatInputCommand() && !interaction.isButton() && !interaction.isStringSelectMenu()) return;
        const member = interaction.member;
        const voiceChannel = member?.voice?.channel;
        const voiceChannelMembers = voiceChannel ? voiceChannel.members.filter(m => !m.user.bot).size : 0;
        const proxied = {
            id: interaction.id,
            token: interaction.token,
            application_id: interaction.applicationId,
            type: interaction.type,
            guild_id: interaction.guildId,
            channel_id: interaction.channelId,
            user_id: interaction.user.id,
            username: interaction.user.username,
            member_voice_channel_id: voiceChannel?.id || '',
            voice_channel_members: voiceChannelMembers,
            command_name: interaction.commandName || '',
            options: interaction.options?.data || [],
            custom_id: interaction.customId || '',
            message_id: interaction.message?.id || '',
            values: interaction.values || [],
            is_admin: member?.permissions?.has(require('discord.js').PermissionFlagsBits.Administrator) || false,
        };
        await interaction.deferReply();
        notifyBotInteraction(proxied);
    } catch (e) {
        console.error('[ERROR] [VoiceServer] Error handling interaction:', e.message);
    }
});

const apiRouter = createApiRouter({
    connections, players, activeStreams, audioResources, playSessions,
    client, version: VOICE_SERVER_VERSION, GO_IPC_PORT, INTERNAL_IPC_TOKEN, cleanupStreams
});

// Leave the voice channel once nobody but bots has been in it for a minute.
const ALONE_TIMEOUT_MS = 60_000;
const aloneTimers = new Map();

function humansInBotChannel(guild) {
    const channel = guild.members.me?.voice?.channel;
    if (!channel) return null;
    return channel.members.filter(m => !m.user.bot).size;
}

client.on('voiceStateUpdate', (oldState, newState) => {
    const guild = newState.guild || oldState.guild;
    if (!guild || !connections.has(guild.id)) return;

    const humans = humansInBotChannel(guild);
    if (humans === null || humans > 0) {
        clearTimeout(aloneTimers.get(guild.id));
        aloneTimers.delete(guild.id);
        return;
    }
    if (aloneTimers.has(guild.id)) return;

    aloneTimers.set(guild.id, setTimeout(() => {
        aloneTimers.delete(guild.id);
        if (connections.has(guild.id) && humansInBotChannel(guild) === 0) {
            console.log(`[INFO] [VoiceServer] Alone in voice channel for guild ${guild.id}, leaving`);
            apiRouter.leaveGuild(guild.id);
        }
    }, ALONE_TIMEOUT_MS));
});

app.use('/', apiRouter);

client.login(process.env.DISCORD_TOKEN).then(() => {
    // IPC is only for the local Go process; never expose it on the network
    app.listen(VOICE_PORT, '127.0.0.1', () => {
        console.log(`[INFO] [VoiceServer] HTTP Server listening on port ${VOICE_PORT}`);
    });
}).catch((err) => {
    console.error('[FATAL] [VoiceServer] Failed to login to Discord Gateway:', err.message);
    process.exit(1);
});
