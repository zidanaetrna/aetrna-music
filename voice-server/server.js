const dns = require('dns');
const fs = require('fs');
const path = require('path');
const { PassThrough } = require('stream');
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

function getCacheDir() {
    try {
        const dir = path.join(__dirname, '../.cache/yt-dlp');
        if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
        return dir;
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

const GITHUB_REPO = 'zidanaetrna/aetrna-music';
const GITHUB_RELEASES_URL = `https://api.github.com/repos/${GITHUB_REPO}/releases/latest`;
const RELEASES_PAGE_URL = `https://github.com/${GITHUB_REPO}/releases`;
const DEFAULT_CHECK_INTERVAL_MS = 20 * 60 * 1000;
const REQUEST_TIMEOUT_MS = 8 * 1000;
let VOICE_SERVER_VERSION = 'v2.1.4';
try {
    const pkg = require('./package.json');
    if (pkg && pkg.version) VOICE_SERVER_VERSION = `v${pkg.version}`;
} catch (_) {}

function parseSemver(raw) {
    let s = String(raw || '').trim();
    s = s.replace(/^[vV]/, '');
    let pre = '';
    const idx = s.search(/[-+]/);
    if (idx !== -1) {
        if (s[idx] === '-') pre = s.substring(idx + 1);
        s = s.substring(0, idx);
    }
    const parts = s.split('.');
    if (parts.length !== 3) return null;
    const maj = parseInt(parts[0], 10);
    const min = parseInt(parts[1], 10);
    const pat = parseInt(parts[2], 10);
    if (isNaN(maj) || isNaN(min) || isNaN(pat)) return null;
    return { major: maj, minor: min, patch: pat, pre };
}

function compareSemver(a, b) {
    const sa = parseSemver(a);
    const sb = parseSemver(b);
    if (!sa || !sb) return 0;
    if (sa.major < sb.major) return -1;
    if (sa.major > sb.major) return 1;
    if (sa.minor < sb.minor) return -1;
    if (sa.minor > sb.minor) return 1;
    if (sa.patch < sb.patch) return -1;
    if (sa.patch > sb.patch) return 1;
    if (sa.pre === '' && sb.pre !== '') return 1;
    if (sa.pre !== '' && sb.pre === '') return -1;
    return 0;
}

function getenvBool(key, def) {
    const v = String(process.env[key] || '').trim();
    if (v === '') return def;
    try { return JSON.parse(v.toLowerCase()); } catch (_) { return def; }
}

async function getRemoteLatest() {
    if (getenvBool('DISABLE_VERSION_CHECK', false)) {
        return { tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: null };
    }
    return new Promise((resolve) => {
        const timeout = setTimeout(() => {
            resolve({ tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: new Error('request timeout') });
        }, REQUEST_TIMEOUT_MS);
        const req = http.request(GITHUB_RELEASES_URL, {
            method: 'GET',
            headers: {
                'Accept': 'application/vnd.github+json',
                'X-GitHub-Api-Version': '2022-11-28',
                'User-Agent': `aetrna-music/${VOICE_SERVER_VERSION}`,
                ...(process.env.GITHUB_TOKEN ? { 'Authorization': `Bearer ${process.env.GITHUB_TOKEN.trim()}` } : {})
            }
        }, (res) => {
            let data = '';
            res.on('data', (chunk) => { data += chunk; });
            res.on('end', () => {
                clearTimeout(timeout);
                if (res.statusCode === 404) {
                    resolve({ tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: null });
                    return;
                }
                if (res.statusCode >= 400) {
                    resolve({ tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: new Error(`github http ${res.statusCode}`) });
                    return;
                }
                try {
                    const rel = JSON.parse(data);
                    if (rel.draft) {
                        resolve({ tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: null });
                        return;
                    }
                    if (rel.prerelease && !getenvBool('VERSION_CHECK_PRERELEASE', false)) {
                        resolve({ tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: null });
                        return;
                    }
                    const tag = String(rel.tag_name || '').trim();
                    if (!tag) {
                        resolve({ tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: null });
                        return;
                    }
                    resolve({
                        tag,
                        changelogURL: rel.html_url || `${RELEASES_PAGE_URL}/tag/${tag}`,
                        err: null
                    });
                } catch (e) {
                    resolve({ tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: e });
                }
            });
        });
        req.on('error', (e) => {
            clearTimeout(timeout);
            resolve({ tag: VOICE_SERVER_VERSION, changelogURL: RELEASES_PAGE_URL, err: e });
        });
        req.end();
    });
}

let voiceVersionCache = { tag: null, changelogURL: RELEASES_PAGE_URL, checkedAt: 0 };
async function runVoiceVersionCheck() {
    if (getenvBool('DISABLE_VERSION_CHECK', false)) return;
    const now = Date.now();
    let latest, changelogURL, err;
    if (voiceVersionCache.tag && (now - voiceVersionCache.checkedAt) < DEFAULT_CHECK_INTERVAL_MS) {
        latest = voiceVersionCache.tag;
        changelogURL = voiceVersionCache.changelogURL;
        err = null;
    } else {
        const res = await getRemoteLatest();
        latest = res.tag;
        changelogURL = res.changelogURL;
        err = res.err;
        voiceVersionCache = { tag: latest, changelogURL, checkedAt: now };
    }
    if (err) {
        console.warn(`[WARN] [VoiceServer] [VersionCheck] Remote latest lookup failed: ${err.message}`);
        return;
    }
    const cmp = compareSemver(VOICE_SERVER_VERSION, latest);
    if (cmp < 0) {
        console.warn(`[WARN] [VoiceServer] [VersionCheck] This instance: ${VOICE_SERVER_VERSION} — latest published release: ${latest}. Upgrade recommended: ${changelogURL}`);
    } else {
        console.log(`[INFO] [VoiceServer] [VersionCheck] Running ${VOICE_SERVER_VERSION}. Latest published release: ${latest}.`);
    }
}

if (!getenvBool('DISABLE_VERSION_CHECK', false)) {
    setTimeout(() => { runVoiceVersionCheck().catch(() => {}); }, 2000);
    setInterval(() => { runVoiceVersionCheck().catch(() => {}); }, DEFAULT_CHECK_INTERVAL_MS);
} else {
    console.log('[INFO] [VoiceServer] [VersionCheck] Version checking is disabled via DISABLE_VERSION_CHECK.');
}

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
    new SlashCommandBuilder().setName('previous').setDescription('Play previous song from history'),
    new SlashCommandBuilder().setName('stop').setDescription('Stop playback and clear queue'),
    new SlashCommandBuilder().setName('pause').setDescription('Pause playback'),
    new SlashCommandBuilder().setName('resume').setDescription('Resume playback'),
    new SlashCommandBuilder().setName('queue').setDescription('Show upcoming queue'),
    new SlashCommandBuilder().setName('nowplaying').setDescription('Show interactive Now Playing card'),
    new SlashCommandBuilder().setName('lyrics').setDescription('Show synced or plain text lyrics for current song'),
    new SlashCommandBuilder().setName('shuffle').setDescription('Shuffle the current song queue'),
    new SlashCommandBuilder().setName('loop').setDescription('Toggle loop mode (Off / Song / Queue)'),
    new SlashCommandBuilder()
        .setName('volume')
        .setDescription('Set playback volume level (0 - 200%)')
        .addIntegerOption(opt => opt.setName('level').setDescription('Volume level percentage (0 - 200)').setMinValue(0).setMaxValue(200).setRequired(true)),
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
const audioResources = new Map(); // guildId -> AudioResource
const playSessions = new Map(); // guildId -> sessionId (integer)

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
const INTERNAL_IPC_TOKEN = process.env.INTERNAL_IPC_TOKEN || 'aetrna-internal-ipc-token-dev-2026';
const GO_BACKEND = 'http://127.0.0.1:47392/internal/interaction';
const GO_TRACK_END = 'http://127.0.0.1:47392/internal/track-end';
const GO_IPC_HEADERS = {
    'Content-Type': 'application/json',
    'X-Internal-IPC-Token': INTERNAL_IPC_TOKEN
};

if (!BOT_TOKEN) {
    console.error('[ERROR] [VoiceServer] DISCORD_TOKEN is missing!');
    process.exit(1);
}

discordClient.on('raw', (packet) => {
    if (packet.t === 'VOICE_SERVER_UPDATE' || packet.t === 'VOICE_STATE_UPDATE') {
        console.log(`[DEBUG] [VoiceServer] Gateway event ${packet.t} for guild=${packet.d?.guild_id || packet.d?.guildId}`);
    }
});

function commandsEqual(a, b) {
    try {
        return JSON.stringify(a) === JSON.stringify(b);
    } catch (_) { return false; }
}

async function syncSlashCommands(client) {
    const rest = new REST({ version: '10' }).setToken(BOT_TOKEN);
    const appId = client.user.id;
    try {
        // 1) Fetch existing global commands and compare — only PUT if changed
        let globalChanged = false;
        try {
            const existingGlobal = await rest.get(Routes.applicationCommands(appId));
            if (!Array.isArray(existingGlobal) || existingGlobal.length !== commands.length) {
                globalChanged = true;
            } else {
                const normalizedExisting = existingGlobal.map(c => ({
                    name: c.name,
                    type: c.type,
                    description: c.description,
                    options: c.options || [],
                    default_member_permissions: c.default_member_permissions ?? null,
                    nsfw: !!c.nsfw,
                    dm_permission: c.dm_permission ?? true
                })).sort((x, y) => x.name.localeCompare(y.name));
                const normalizedDesired = commands.map(c => ({
                    name: c.name,
                    type: c.type,
                    description: c.description,
                    options: c.options || [],
                    default_member_permissions: c.default_member_permissions ?? null,
                    nsfw: !!c.nsfw,
                    dm_permission: c.dm_permission ?? true
                })).sort((x, y) => x.name.localeCompare(y.name));
                globalChanged = !commandsEqual(normalizedExisting, normalizedDesired);
            }
        } catch (_) {
            globalChanged = true;
        }
        if (globalChanged) {
            await rest.put(Routes.applicationCommands(appId), { body: commands });
            console.log('[INFO] [VoiceServer] Global slash commands UPDATED successfully');
        } else {
            console.log('[INFO] [VoiceServer] Global slash commands already up-to-date — skipped registration');
        }

        // 2) Cleanup ALL guild-level commands (duplicates) — we rely ONLY on global commands
        const cleanupGuild = async (guildId, guildName) => {
            try {
                const existingGuild = await rest.get(Routes.applicationGuildCommands(appId, guildId));
                if (Array.isArray(existingGuild) && existingGuild.length > 0) {
                    console.log(`[INFO] [VoiceServer] Cleaning up ${existingGuild.length} stale/duplicate guild-level commands in ${guildName || guildId}`);
                    for (const cmd of existingGuild) {
                        try {
                            await rest.delete(Routes.applicationGuildCommand(appId, guildId, cmd.id));
                        } catch (_) {}
                    }
                }
            } catch (e) {
                console.error(`[WARN] [VoiceServer] Guild command cleanup failed for ${guildId}:`, e.message);
            }
        };
        const cleanups = [];
        for (const [gId, guild] of client.guilds.cache) {
            cleanups.push(cleanupGuild(gId, guild.name));
        }
        if (cleanups.length) await Promise.all(cleanups);
    } catch (e) {
        console.error('[WARN] [VoiceServer] Failed to sync slash commands:', e.message);
    }
}

discordClient.login(BOT_TOKEN).then(async () => {
    console.log(`[INFO] [VoiceServer] discord.js Client logged in as ${discordClient.user.tag}`);
    await syncSlashCommands(discordClient);
}).catch(err => {
    console.error('[ERROR] [VoiceServer] Login failed:', err.message);
    process.exit(1);
});

discordClient.on('guildCreate', async (guild) => {
    const rest = new REST({ version: '10' }).setToken(BOT_TOKEN);
    // Cleanup any orphan guild commands immediately for new guild
    try {
        const existingGuild = await rest.get(Routes.applicationGuildCommands(discordClient.user.id, guild.id));
        if (Array.isArray(existingGuild) && existingGuild.length > 0) {
            for (const cmd of existingGuild) {
                try { await rest.delete(Routes.applicationGuildCommand(discordClient.user.id, guild.id, cmd.id)); } catch (_) {}
            }
            console.log(`[INFO] [VoiceServer] Cleaned up stale guild commands for new guild ${guild.name} (${guild.id})`);
        } else {
            console.log(`[INFO] [VoiceServer] New guild ${guild.name} (${guild.id}) joined — global commands sync automatically (no guild-level override needed)`);
        }
    } catch (e) {
        console.error(`[WARN] [VoiceServer] Guild ${guild.id} initial cleanup failed:`, e.message);
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
            headers: Object.assign({}, GO_IPC_HEADERS, { 'Content-Length': Buffer.byteLength(body) }),
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

function resolveIsAdmin(interaction) {
    try {
        if (interaction.memberPermissions && typeof interaction.memberPermissions.has === 'function') {
            if (interaction.memberPermissions.has(PermissionFlagsBits.Administrator)) return true;
        }
        if (interaction.member && interaction.member.permissions) {
            const p = interaction.member.permissions;
            if (typeof p.has === 'function' && p.has(PermissionFlagsBits.Administrator)) return true;
            if (typeof p === 'string') {
                const ADMIN_BIT = 1n << 3n;
                try { if ((BigInt(p) & ADMIN_BIT) !== 0n) return true; } catch (_) {}
            } else if (typeof p === 'bigint') {
                const ADMIN_BIT = 1n << 3n;
                if ((p & ADMIN_BIT) !== 0n) return true;
            }
        }
    } catch (_) {}
    return false;
}

const LOCALE_LANG_NAMES = {
    en: 'English 🇬🇧',
    id: 'Bahasa Indonesia 🇮🇩',
    jp: 'Japanese 🇯🇵'
};

// Receive ALL interactions from Discord Gateway.
// Node.js ALWAYS defers first (guarantees Discord ack within 3s).
// Go Bot then edits the deferred message via REST (has 15 minutes).
discordClient.on('interactionCreate', async (interaction) => {
    if (!interaction.guildId) return;

    // ── Pre-flight permission enforcement for admin-only commands ────────
    if (interaction.isChatInputCommand()) {
        const cmd = interaction.commandName;
        if (cmd === 'language') {
            const isAdmin = resolveIsAdmin(interaction);
            if (!isAdmin) {
                try {
                    await interaction.reply({
                        content: '❌ Hanya Administrator Server yang dapat mengubah bahasa bot! (Only Server Administrators can change the bot language!)',
                        flags: 64 // Ephemeral
                    });
                } catch (_) {}
                console.warn(`[WARN] [VoiceServer] Non-admin user ${interaction.user.tag} (${interaction.user.id}) blocked from /language in guild ${interaction.guildId}`);
                return;
            }
        }
    }

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

    const isAdmin = resolveIsAdmin(interaction);

    // Extra: sanitize /language option value — only allow known codes
    let options = interaction.isChatInputCommand() ? serializeOptions(interaction.options.data) : [];
    if (interaction.isChatInputCommand() && interaction.commandName === 'language') {
        options = options.map(opt => {
            if (opt && opt.name === 'lang' && typeof opt.value === 'string') {
                const val = String(opt.value).trim().toLowerCase();
                if (LOCALE_LANG_NAMES[val]) return { ...opt, value: val };
                return { ...opt, value: 'en' }; // fallback safe
            }
            return opt;
        });
    }

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
        options,
        custom_id: (interaction.isButton() || interaction.isStringSelectMenu()) ? interaction.customId : null,
        message_id: (interaction.isButton() || interaction.isStringSelectMenu()) ? interaction.message?.id : null,
        values: interaction.isStringSelectMenu() ? interaction.values : [],
        is_admin: isAdmin,
    };

    // Fire and forget — Go Bot has 15 minutes to edit the deferred message
    sendToGoBot(payload);
});

// Clean up streams for a guild
function cleanupStreams(guildId) {
    audioResources.delete(guildId);
    if (activeStreams.has(guildId)) {
        const stream = activeStreams.get(guildId);
        try { if (stream.ytdlp) stream.ytdlp.kill('SIGKILL'); } catch (e) {}
        try { if (stream.ffmpeg) stream.ffmpeg.kill('SIGKILL'); } catch (e) {}
        activeStreams.delete(guildId);
    }
}

// Notify Go Bot of track end
function notifyBotTrackEnd(guildId, reason) {
    const data = JSON.stringify({ guildId, reason });
    const req = http.request(GO_TRACK_END, {
        method: 'POST',
        headers: Object.assign({}, GO_IPC_HEADERS, { 'Content-Length': Buffer.byteLength(data) }),
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

    console.log(`[INFO] [VoiceServer] Creating fresh voice connection for channel ${channelId} in guild ${guild.name}...`);
    connection = joinVoiceChannel({
        channelId: String(channelId),
        guildId: String(guildId),
        adapterCreator: guild.voiceAdapterCreator,
        selfDeaf: true,
        selfMute: false,
    });

    connections.set(guildId, connection);

    connection.on('stateChange', (oldState, newState) => {
        console.log(`[INFO] [VoiceServer] VoiceConnection ${guildId}: ${oldState.status} -> ${newState.status}`);

        const net = newState.networking || (oldState && oldState.networking);
        if (net && !net._listenersAttached) {
            net._listenersAttached = true;

            net.on('stateChange', (oldNetState, newNetState) => {
                console.log(`[DEBUG] [VoiceServer NetState ${guildId}] ${oldNetState.code} (${oldNetState.status}) -> ${newNetState.code} (${newNetState.status})`);
                if (newNetState.ws) {
                    newNetState.ws.on('close', (eventOrCode, reason) => {
                        const code = (typeof eventOrCode === 'object' && eventOrCode !== null) ? eventOrCode.code : eventOrCode;
                        const rsn = (typeof eventOrCode === 'object' && eventOrCode !== null) ? eventOrCode.reason : reason;
                        console.log(`[DEBUG] [VoiceServer WS Closed ${guildId}] Code: ${code}, Reason: "${rsn}"`, JSON.stringify(eventOrCode));
                    });
                    newNetState.ws.on('error', (err) => {
                        console.error(`[ERROR] [VoiceServer WS Error ${guildId}]`, err.message);
                    });
                }
            });

            net.on('debug', (msg) => console.log(`[DEBUG] [VoiceServer Network ${guildId}]`, msg));
            net.on('error', (err) => console.error(`[ERROR] [VoiceServer Network ${guildId}]`, err));
        }
    });

    connection.on(VoiceConnectionStatus.Disconnected, async () => {
        try {
            await Promise.race([
                entersState(connection, VoiceConnectionStatus.Signalling, 3_000),
                entersState(connection, VoiceConnectionStatus.Connecting, 3_000),
            ]);
        } catch {
            cleanupStreams(guildId);
            try { connection.destroy(); } catch (e) {}
            connections.delete(guildId);
            notifyBotTrackEnd(guildId, 'disconnected');
        }
    });

    return connection;
}

// Express API for Go Bot to trigger audio playback
const app = express();
app.use(express.json());
const PORT = process.env.VOICE_PORT || 3005;

const requireIPCToken = (req, res, next) => {
    const reqToken = req.headers['x-internal-ipc-token'];
    if (INTERNAL_IPC_TOKEN && reqToken !== INTERNAL_IPC_TOKEN) {
        console.warn(`[WARN] [VoiceServer] Rejected IPC request from ${req.socket.remoteAddress}: invalid token`);
        return res.status(401).json({ error: 'Unauthorized' });
    }
    next();
};

app.use(requireIPCToken);

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

        // Track whether bot was ALREADY connected in voice chat before this request
        const wasAlreadyInVoice = connections.has(guildId) && connections.get(guildId).state.status === VoiceConnectionStatus.Ready;

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
                const userAgent = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36';
                let headerStr = `Referer: https://www.youtube.com/\r\n`;
                const cookieHeader = getCookieHeaderString();
                if (cookieHeader) {
                    headerStr += `Cookie: ${cookieHeader}\r\n`;
                }

                const videoInputUrl = (songUrl && (songUrl.includes('youtube.com') || songUrl.includes('youtu.be'))) ? songUrl
                    : (streamUrl.includes('youtube.com') || streamUrl.includes('youtu.be')) ? streamUrl
                    : null;

                console.log(`[INFO] [VoiceServer] Starting direct FFmpeg stream for guild ${guildId}...`);

                let ytdlp = null;
                let isFallingBack = false;

                let ffmpeg = spawn('ffmpeg', [
                    '-reconnect', '1', '-reconnect_streamed', '1', '-reconnect_delay_max', '5',
                    '-user_agent', userAgent,
                    '-headers', headerStr,
                    '-i', streamUrl,
                    '-loglevel', 'warning',
                    '-af', audioFilter,
                    '-f', 's16le', '-ar', '48000', '-ac', '2', 'pipe:1'
                ], { stdio: ['ignore', 'pipe', 'pipe'] });

                if (ffmpeg.stdin) ffmpeg.stdin.on('error', () => {});
                activeStreams.set(guildId, { ffmpeg });

                let hasAudioData = false;
                ffmpeg.stdout.on('data', () => { hasAudioData = true; });

                ffmpeg.stderr.on('data', (d) => {
                    const msg = d.toString().trim();
                    if (msg) console.error(`[ffmpeg ${guildId}] ${msg}`);

                    // Automatic 403 Forbidden Fallback: If direct stream gets 403 and has no audio yet, fallback to yt-dlp pipe
                    if (msg.includes('403 Forbidden') && !hasAudioData && !isFallingBack && videoInputUrl) {
                        isFallingBack = true;
                        console.warn(`[WARN] [VoiceServer] Direct FFmpeg got 403 Forbidden for guild ${guildId}. Falling back to yt-dlp pipe...`);
                        try { ffmpeg.kill('SIGKILL'); } catch (e) {}

                        const ytdlpClients = process.env.YTDLP_CLIENTS || 'tv';
                        const cookiesPath = getAbsoluteCookiesPath();
                        const cacheDir = getCacheDir();
                        let cookiesValid = false;
                        try {
                            if (cookiesPath) {
                                const st = fs.statSync(cookiesPath);
                                cookiesValid = st.size > 100;
                            }
                        } catch (_) {}
                        const ytdlpArgs = [
                            '-4',
                            '--js-runtimes', 'node',
                            '--extractor-args', `youtube:player_client=${ytdlpClients}`,
                            '-f', 'ba/ba*/bestaudio/b',
                            '--concurrent-fragments', '5',
                            '--no-playlist',
                            '--geo-bypass',
                            '--no-check-certificates',
                            '--no-warnings',
                            '--user-agent', userAgent,
                            '-o', '-',
                            videoInputUrl
                        ];
                        if (cacheDir) ytdlpArgs.unshift('--cache-dir', cacheDir);
                        if (cookiesValid) ytdlpArgs.unshift('--cookies', cookiesPath);

                        ytdlp = spawn('yt-dlp', ytdlpArgs, { stdio: ['ignore', 'pipe', 'pipe'] });
                        ffmpeg = spawn('ffmpeg', [
                            '-i', 'pipe:0',
                            '-loglevel', 'warning',
                            '-af', audioFilter,
                            '-f', 's16le', '-ar', '48000', '-ac', '2', 'pipe:1'
                        ], { stdio: ['pipe', 'pipe', 'pipe'] });

                        if (ffmpeg.stdin) ffmpeg.stdin.on('error', () => {});
                        if (ytdlp.stdout) {
                            ytdlp.stdout.on('error', () => {});
                            ytdlp.stdout.pipe(ffmpeg.stdin);
                            ytdlp.stderr.on('data', (errData) => { const m = errData.toString().trim(); if (m) console.error(`[ytdlp ${guildId}] ${m}`); });
                        }

                        activeStreams.set(guildId, { ffmpeg, ytdlp });
                        const fbResource = createAudioResource(ffmpeg.stdout, { inputType: StreamType.Raw, inlineVolume: true });
                        fbResource.volume.setVolume(volume);
                        audioResources.set(guildId, fbResource);
                        player.play(fbResource);
                    }
                });

                ffmpeg.on('exit', (code, signal) => {
                    if (code !== 0 && code !== null && signal !== 'SIGKILL' && !isFallingBack) {
                        console.error(`[WARN] [VoiceServer] FFmpeg process exited with code ${code} (signal: ${signal}) for guild ${guildId}`);
                    }
                });

                const resource = createAudioResource(ffmpeg.stdout, { inputType: StreamType.Raw, inlineVolume: true });
                resource.volume.setVolume(volume);
                audioResources.set(guildId, resource);

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
    
    // Force disconnect fallback via Discord.js guild voice state (guarantees leaving voice channel)
    try {
        const guild = discordClient.guilds.cache.get(String(guildId));
        if (guild && guild.members.me && guild.members.me.voice) {
            guild.members.me.voice.disconnect().catch(() => {});
        }
    } catch (_) {}

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

app.post('/volume', (req, res) => {
    const { guildId, volume } = req.body;
    const resource = audioResources.get(guildId);
    if (resource && resource.volume) {
        const volVal = parseFloat(volume);
        resource.volume.setVolume(isNaN(volVal) ? 1.0 : volVal);
        console.log(`[INFO] [VoiceServer] Audio volume dynamically adjusted to ${(volVal * 100).toFixed(0)}% for guild ${guildId}`);
    }
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

    console.log(`[INFO] [/test-voice] Starting diagnostic for guild ${guild.name} (${guild.id}) channel ${channel.name}...`);

    try {
        const connection = joinVoiceChannel({
            channelId: channel.id,
            guildId: guild.id,
            adapterCreator: guild.voiceAdapterCreator,
            selfDeaf: true,
        });

        connection.on('stateChange', (oldState, newState) => {
            console.log(`[INFO] [/test-voice State] ${oldState.status} -> ${newState.status}`);

            const net = newState.networking || (oldState && oldState.networking);
            if (net && !net._listenersAttached) {
                net._listenersAttached = true;

                net.on('stateChange', (oldNetState, newNetState) => {
                    console.log(`[DEBUG] [/test-voice NetState] ${oldNetState.code} (${oldNetState.status}) -> ${newNetState.code} (${newNetState.status})`);
                    if (newNetState.ws) {
                        newNetState.ws.on('close', (eventOrCode, reason) => {
                            const code = (typeof eventOrCode === 'object' && eventOrCode !== null) ? eventOrCode.code : eventOrCode;
                            const rsn = (typeof eventOrCode === 'object' && eventOrCode !== null) ? eventOrCode.reason : reason;
                            console.log(`[DEBUG] [/test-voice Voice WS Closed] Code: ${code}, Reason: "${rsn}"`, JSON.stringify(eventOrCode));
                        });
                        newNetState.ws.on('error', (err) => {
                            console.error(`[ERROR] [/test-voice Voice WS Error]`, err.message);
                        });
                    }
                });

                net.on('debug', (msg) => console.log('[DEBUG] [Networking Debug]', msg));
                net.on('error', (err) => console.error('[ERROR] [Networking Error]', err));
            }

            if (oldState.status === 'connecting' && newState.status === 'signalling') {
                console.warn(`[WARN] [/test-voice] Connecting -> Signalling reset detected`);
            }
        });

        connection.on('error', (err) => {
            console.error(`[ERROR] [/test-voice Error]`, err);
        });

        connection.on('debug', (msg) => {
            console.log(`[DEBUG] [/test-voice Debug]`, typeof msg === 'object' ? JSON.stringify(msg) : msg);
        });

        res.json({ status: 'initiated', guild: guild.name, channel: channel.name });
    } catch (e) {
        res.status(500).json({ error: e.message });
    }
});

app.listen(PORT, '127.0.0.1', () => {
    console.log(`[INFO] [VoiceServer] Voice Server listening on http://127.0.0.1:${PORT}`);
});
