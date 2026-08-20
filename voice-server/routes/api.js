const express = require('express');
const {
    joinVoiceChannel, createAudioPlayer, createAudioResource,
    AudioPlayerStatus, VoiceConnectionStatus, entersState, StreamType
} = require('@discordjs/voice');
const { spawn } = require('child_process');
const http = require('http');

const { getFFmpegAudioFilter } = require('../audio/filters');

function createApiRouter(context) {
    const router = express.Router();
    const {
        connections, players, activeStreams, audioResources, playSessions,
        client, version, GO_IPC_PORT, INTERNAL_IPC_TOKEN, cleanupStreams
    } = context;

    function notifyBotTrackEnd(guildId, reason) {
        const data = JSON.stringify({ guildId: guildId, guild_id: guildId, reason: reason || 'finished' });
        const req = http.request(`http://127.0.0.1:${GO_IPC_PORT}/internal/track-end`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(data),
                'X-Internal-IPC-Token': INTERNAL_IPC_TOKEN,
            },
            timeout: 3000,
        }, (res) => {
            res.resume();
        });
        req.on('error', (err) => {
            console.error(`[ERROR] [VoiceServer] Failed to notify Go bot of track end for guild ${guildId}:`, err.message);
        });
        req.write(data);
        req.end();
    }

    router.get('/health', (req, res) => {
        res.json({ status: 'ok', version, activeConnections: connections.size });
    });

    router.post('/join-and-play', (req, res) => {
        const { guildId, channelId, streamUrl, filter, volume = 1.0 } = req.body;
        if (!guildId || !channelId || !streamUrl) {
            return res.status(400).json({ error: 'Missing required parameters: guildId, channelId, streamUrl' });
        }

        const currentSessionId = (playSessions.get(guildId) || 0) + 1;
        playSessions.set(guildId, currentSessionId);
        cleanupStreams(guildId);
        res.json({ status: 'initiating', guildId, channelId, sessionId: currentSessionId });

        (async () => {
            try {
                let connection = connections.get(guildId);
                if (!connection || connection.state.status === VoiceConnectionStatus.Destroyed) {
                    console.log(`[INFO] [VoiceServer] Connecting to voice channel ${channelId} in guild ${guildId}...`);
                    connection = joinVoiceChannel({
                        channelId,
                        guildId,
                        adapterCreator: client.guilds.cache.get(guildId).voiceAdapterCreator,
                        selfDeaf: true,
                        selfMute: false,
                    });
                    connections.set(guildId, connection);

                    connection.on(VoiceConnectionStatus.Disconnected, async () => {
                        try {
                            await Promise.race([
                                entersState(connection, VoiceConnectionStatus.Signalling, 5000),
                                entersState(connection, VoiceConnectionStatus.Connecting, 5000),
                            ]);
                        } catch (e) {
                            console.warn(`[WARN] [VoiceServer] Connection lost in guild ${guildId}, cleaning up`);
                            cleanupStreams(guildId);
                            try { connection.destroy(); } catch (_) {}
                            connections.delete(guildId);
                        }
                    });

                    await entersState(connection, VoiceConnectionStatus.Ready, 20_000);
                }

                if (playSessions.get(guildId) !== currentSessionId) return;

                let player = players.get(guildId);
                if (player) {
                    player.removeAllListeners();
                    try { player.stop(); } catch (e) {}
                }

                player = createAudioPlayer();
                players.set(guildId, player);

                let streamPlaybackStartedAt = 0;
                let playRequestedAt = Date.now();
                let hasEnteredPlayingState = false;

                player.on(AudioPlayerStatus.Playing, () => {
                    hasEnteredPlayingState = true;
                    streamPlaybackStartedAt = Date.now();
                    console.log(`[INFO] [VoiceServer] AudioPlayer entered Playing state for guild ${guildId}`);
                });

                player.on(AudioPlayerStatus.Idle, () => {
                    if (playSessions.get(guildId) !== currentSessionId) return;

                    // Ignore initial Idle state during network/FFmpeg buffering phase (0-15s window before Playing)
                    if (!hasEnteredPlayingState && (Date.now() - playRequestedAt) < 15000) {
                        console.log(`[DEBUG] [VoiceServer] Ignoring premature Idle state during buffering for guild ${guildId}`);
                        return;
                    }

                    const durationMs = streamPlaybackStartedAt > 0 ? (Date.now() - streamPlaybackStartedAt) : 0;
                    console.log(`[INFO] [VoiceServer] Track ended for guild ${guildId} (duration: ${durationMs}ms)`);
                    cleanupStreams(guildId);
                    if (!hasEnteredPlayingState || durationMs < 1500) {
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
                const headerStr = `Referer: https://www.youtube.com/\r\n`;

                console.log(`[INFO] [VoiceServer] Starting direct FFmpeg stream for guild ${guildId}...`);

                const ffmpeg = spawn('ffmpeg', [
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

                ffmpeg.stderr.on('data', (d) => {
                    const msg = d.toString().trim();
                    if (msg) console.error(`[ffmpeg ${guildId}] ${msg}`);
                });

                ffmpeg.on('exit', (code, signal) => {
                    if (code !== 0 && code !== null && signal !== 'SIGKILL') {
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
    });

    router.post('/stop', (req, res) => {
        const { guildId } = req.body;
        cleanupStreams(guildId);
        const player = players.get(guildId);
        if (player) { player.stop(); players.delete(guildId); }
        const connection = connections.get(guildId);
        if (connection) { try { connection.destroy(); } catch (e) {} connections.delete(guildId); }
        res.json({ status: 'stopped', guildId });
    });

    router.post('/pause', (req, res) => {
        const { guildId } = req.body;
        const player = players.get(guildId);
        if (player) { player.pause(); }
        res.json({ status: 'paused', guildId });
    });

    router.post('/resume', (req, res) => {
        const { guildId } = req.body;
        const player = players.get(guildId);
        if (player) { player.unpause(); }
        res.json({ status: 'resumed', guildId });
    });

    router.post('/volume', (req, res) => {
        const { guildId, volume } = req.body;
        const resource = audioResources.get(guildId);
        if (resource && resource.volume) {
            resource.volume.setVolume(volume);
        }
        res.json({ status: 'volume_set', guildId, volume });
    });

    return router;
}

module.exports = createApiRouter;
