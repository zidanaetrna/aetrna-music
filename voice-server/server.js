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
const { Client, GatewayIntentBits, REST, Routes, SlashCommandBuilder, EmbedBuilder } = require('discord.js');
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

// Slash command definitions
const commands = [
    new SlashCommandBuilder()
        .setName('play')
        .setDescription('Play a song from YouTube')
        .addStringOption(option => option.setName('query').setDescription('Song name or YouTube URL').setRequired(true)),
    new SlashCommandBuilder()
        .setName('stop')
        .setDescription('Stop playback and leave the voice channel'),
    new SlashCommandBuilder()
        .setName('skip')
        .setDescription('Skip the current song'),
    new SlashCommandBuilder()
        .setName('pause')
        .setDescription('Pause playback'),
    new SlashCommandBuilder()
        .setName('resume')
        .setDescription('Resume playback'),
    new SlashCommandBuilder()
        .setName('queue')
        .setDescription('Show the current music queue')
].map(cmd => cmd.toJSON());

// State stores
const connections = new Map();
const players = new Map();
const activeStreams = new Map();
const guildQueues = new Map(); // guildId -> Array<{ title, url }>

// Initialize single discord.js Client
const discordClient = new Client({
    intents: [
        GatewayIntentBits.Guilds,
        GatewayIntentBits.GuildVoiceStates
    ]
});

const BOT_TOKEN = process.env.DISCORD_TOKEN;
if (BOT_TOKEN) {
    discordClient.login(BOT_TOKEN).then(async () => {
        console.log(`✅ [VoiceServer] discord.js Client logged in as ${discordClient.user.tag}`);
        try {
            const rest = new REST({ version: '10' }).setToken(BOT_TOKEN);
            console.log('⏳ Registering global slash commands...');
            await rest.put(Routes.applicationCommands(discordClient.user.id), { body: commands });
            console.log('✅ Global slash commands registered successfully!');
        } catch (e) {
            console.error('⚠️ Failed to register slash commands:', e.message);
        }
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

async function playNextInQueue(guildId, channelId, interactionChannel) {
    const queue = guildQueues.get(guildId) || [];
    if (!queue.length) {
        cleanupStreams(guildId);
        const connection = connections.get(guildId);
        if (connection) {
            try { connection.destroy(); } catch (e) {}
            connections.delete(guildId);
        }
        if (interactionChannel) {
            interactionChannel.send('🎶 Queue is empty. Leaving voice channel.').catch(() => {});
        }
        return;
    }

    const currentTrack = queue[0];

    try {
        cleanupStreams(guildId);

        let connection = connections.get(guildId);
        if (!connection || connection.joinConfig.channelId !== channelId || connection.state.status === VoiceConnectionStatus.Destroyed) {
            if (connection) { try { connection.destroy(); } catch (e) {} }

            const guild = discordClient.guilds.cache.get(guildId) || await discordClient.guilds.fetch(guildId).catch(() => null);
            if (!guild) throw new Error('Guild not found');

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
                console.log(`🔄 [VoiceServer] VoiceConnection ${guildId} state: ${oldState.status} ➔ ${newState.status}`);
            });

            connection.on('error', (error) => {
                console.error(`❌ [VoiceServer ConnectionError ${guildId}]`, error);
            });
        }

        // Check cookies file
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
            '--get-title', '--get-url',
            currentTrack.url
        ];

        const resolvePromise = resolveStreamUrl(resolveArgs, spawnEnv);

        console.log(`⏳ [VoiceServer] Waiting for voice connection Ready...`);
        await entersState(connection, VoiceConnectionStatus.Ready, 20_000);
        console.log(`✅ [VoiceServer] Native voice connection Ready!`);

        const [title, streamUrl] = await resolvePromise;
        currentTrack.title = title || currentTrack.url;
        console.log(`🔗 [VoiceServer] Resolved: ${currentTrack.title}`);

        let player = players.get(guildId);
        if (!player) {
            player = createAudioPlayer();
            players.set(guildId, player);

            player.on(AudioPlayerStatus.Idle, () => {
                console.log(`🎵 [VoiceServer] Track finished for guild ${guildId}`);
                const q = guildQueues.get(guildId) || [];
                q.shift(); // Remove played song
                guildQueues.set(guildId, q);
                playNextInQueue(guildId, channelId, interactionChannel);
            });

            player.on('error', (err) => {
                console.error(`❌ [VoiceServer] Player error in guild ${guildId}:`, err.message);
                const q = guildQueues.get(guildId) || [];
                q.shift();
                guildQueues.set(guildId, q);
                playNextInQueue(guildId, channelId, interactionChannel);
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

        player.play(resource);
        connection.subscribe(player);

        if (interactionChannel) {
            const embed = new EmbedBuilder()
                .setColor(0x00FF7F)
                .setTitle('🎵 Now Playing')
                .setDescription(`[${currentTrack.title}](${currentTrack.url})`)
                .setTimestamp();
            interactionChannel.send({ embeds: [embed] }).catch(() => {});
        }

    } catch (err) {
        console.error('❌ Playback error:', err.message);
        if (interactionChannel) {
            interactionChannel.send(`❌ Playback error: ${err.message}`).catch(() => {});
        }
        const q = guildQueues.get(guildId) || [];
        q.shift();
        guildQueues.set(guildId, q);
        playNextInQueue(guildId, channelId, interactionChannel);
    }
}

// Handle Slash Commands
discordClient.on('interactionCreate', async (interaction) => {
    if (!interaction.isChatInputCommand()) return;

    const { commandName, guildId, member, channel } = interaction;
    if (!guildId || !member) return;

    const voiceChannel = member.voice?.channel;

    if (commandName === 'play') {
        const query = interaction.options.getString('query');
        if (!voiceChannel) {
            return interaction.reply({ content: '❌ You must be in a voice channel to use `/play`!', ephemeral: true });
        }

        await interaction.deferReply();

        let trackUrl = query;
        if (!query.startsWith('http')) {
            trackUrl = `ytsearch1:${query}`;
        }

        const queue = guildQueues.get(guildId) || [];
        const isFirstSong = queue.length === 0;

        queue.push({ title: query, url: trackUrl });
        guildQueues.set(guildId, queue);

        const embed = new EmbedBuilder()
            .setColor(0x0099FF)
            .setTitle('🎶 Added to Queue')
            .setDescription(`\`${query}\``)
            .setFooter({ text: `Position in queue: ${queue.length}` });

        await interaction.editReply({ embeds: [embed] });

        if (isFirstSong) {
            playNextInQueue(guildId, voiceChannel.id, channel);
        }
    } else if (commandName === 'stop') {
        guildQueues.delete(guildId);
        cleanupStreams(guildId);

        const player = players.get(guildId);
        if (player) { player.stop(); players.delete(guildId); }

        const connection = connections.get(guildId);
        if (connection) {
            try { connection.destroy(); } catch (e) {}
            connections.delete(guildId);
        }

        await interaction.reply('⏹️ Stopped playback and cleared queue.');
    } else if (commandName === 'skip') {
        const player = players.get(guildId);
        if (player) {
            player.stop();
            await interaction.reply('⏭️ Skipped current track.');
        } else {
            await interaction.reply({ content: '❌ Nothing is playing right now.', ephemeral: true });
        }
    } else if (commandName === 'pause') {
        const player = players.get(guildId);
        if (player) {
            player.pause();
            await interaction.reply('⏸️ Paused playback.');
        } else {
            await interaction.reply({ content: '❌ Nothing is playing right now.', ephemeral: true });
        }
    } else if (commandName === 'resume') {
        const player = players.get(guildId);
        if (player) {
            player.unpause();
            await interaction.reply('▶️ Resumed playback.');
        } else {
            await interaction.reply({ content: '❌ Nothing is playing right now.', ephemeral: true });
        }
    } else if (commandName === 'queue') {
        const queue = guildQueues.get(guildId) || [];
        if (!queue.length) {
            return interaction.reply('🎶 Queue is currently empty.');
        }

        const description = queue.slice(0, 10).map((t, i) => `${i + 1}. \`${t.title}\``).join('\n');
        const embed = new EmbedBuilder()
            .setColor(0x9900FF)
            .setTitle('📜 Current Music Queue')
            .setDescription(description)
            .setFooter({ text: `Total tracks in queue: ${queue.length}` });

        await interaction.reply({ embeds: [embed] });
    }
});

// Express Server
const app = express();
app.use(express.json());
const PORT = process.env.PORT || 3005;

app.get('/health', (req, res) => {
    res.json({ status: 'ok', clientReady: discordClient.isReady(), connections: connections.size });
});

app.listen(PORT, '127.0.0.1', () => {
    console.log(`✅ Voice Server listening on http://127.0.0.1:${PORT}`);
});
