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

module.exports = {
    getFFmpegAudioFilter
};
