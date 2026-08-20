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
