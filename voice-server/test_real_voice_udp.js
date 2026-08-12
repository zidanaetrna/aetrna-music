const { Client, GatewayIntentBits } = require('discord.js');
const { joinVoiceChannel, VoiceConnectionStatus } = require('@discordjs/voice');
const dgram = require('dgram');
require('dotenv').config();

const client = new Client({
    intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildVoiceStates]
});

client.on('ready', async () => {
    console.log(`✅ Logged in as ${client.user.tag}`);

    const guild = client.guilds.cache.first();
    if (!guild) {
        console.error('❌ No guild found');
        process.exit(1);
    }

    const channels = guild.channels.cache.filter(c => c.isVoiceBased());
    const channel = channels.first();
    if (!channel) {
        console.error('❌ No voice channel found in guild', guild.name);
        process.exit(1);
    }

    console.log(`🎙️ Test joining voice channel "${channel.name}" in guild "${guild.name}"...`);

    const connection = joinVoiceChannel({
        channelId: channel.id,
        guildId: guild.id,
        adapterCreator: guild.voiceAdapterCreator,
        selfDeaf: true,
    });

    connection.on('stateChange', (oldState, newState) => {
        console.log(`🔄 Connection state: ${oldState.status} ➔ ${newState.status}`);
    });

    connection.on('debug', (msg) => {
        console.log(`🔍 [Voice Debug]`, msg);
    });

    setTimeout(() => {
        console.log('⏰ Diagnostic finished after 15s. Exiting...');
        process.exit(0);
    }, 15000);
});

client.login(process.env.DISCORD_TOKEN);
