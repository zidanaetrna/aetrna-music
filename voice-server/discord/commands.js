const { SlashCommandBuilder, PermissionFlagsBits } = require('discord.js');

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
    new SlashCommandBuilder()
        .setName('loop')
        .setDescription('Set loop mode')
        .addSubcommand(sub => sub.setName('track').setDescription('Loop every track in the queue/playlist (repeats forever)'))
        .addSubcommand(sub => sub.setName('song').setDescription('Repeat the current song'))
        .addSubcommand(sub => sub.setName('off').setDescription('Turn looping off')),
    new SlashCommandBuilder()
        .setName('top')
        .setDescription('Show the 5 most played songs in this server')
        .addStringOption(opt =>
            opt.setName('period')
                .setDescription('Time range (default: this week)')
                .setRequired(false)
                .addChoices(
                    { name: 'This week (last 7 days)', value: 'week' },
                    { name: 'This month (last 30 days)', value: 'month' }
                )
        ),
    new SlashCommandBuilder()
        .setName('exclude')
        .setDescription('Skip tracks by queue number or song link (also on every loop cycle)')
        .addStringOption(opt => opt.setName('track').setDescription('Track number(s) like 7, 3,7,9 or 2-5, or a song link. Leave empty to list').setRequired(false)),
    new SlashCommandBuilder()
        .setName('include')
        .setDescription('Bring excluded tracks back, or re-add a song by link if it left the queue')
        .addStringOption(opt => opt.setName('track').setDescription('Song link (YouTube/Spotify), track number(s) like 7 or 3,7,9, or "all"').setRequired(true)),
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
        .setName('playlist')
        .setDescription('Manage your custom saved playlists')
        .addSubcommand(sub =>
            sub.setName('list')
               .setDescription('List all your saved playlists')
        )
        .addSubcommand(sub =>
            sub.setName('play')
               .setDescription('Play a saved playlist into your voice channel')
               .addStringOption(opt => opt.setName('name').setDescription('Playlist name').setRequired(true))
        )
        .addSubcommand(sub =>
            sub.setName('create')
               .setDescription('Create a new custom saved playlist')
               .addStringOption(opt => opt.setName('name').setDescription('Playlist name').setRequired(true))
        )
        .addSubcommand(sub =>
            sub.setName('add-track')
               .setDescription('Add a song, YouTube link, or Spotify link to a saved playlist')
               .addStringOption(opt => opt.setName('playlist').setDescription('Playlist name').setRequired(true))
               .addStringOption(opt => opt.setName('query').setDescription('Song name, YouTube link, or Spotify link').setRequired(true))
        )
        .addSubcommand(sub =>
            sub.setName('list-tracks')
               .setDescription('View tracks in a saved playlist')
               .addStringOption(opt => opt.setName('name').setDescription('Playlist name').setRequired(true))
        )
        .addSubcommand(sub =>
            sub.setName('delete')
               .setDescription('Delete a saved playlist')
               .addStringOption(opt => opt.setName('name').setDescription('Playlist name').setRequired(true))
        ),
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

module.exports = {
    commands
};
