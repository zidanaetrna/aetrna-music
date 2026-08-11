package audio

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"

	"layeh.com/gopus"
)

type Streamer struct {
	cacheMgr *CacheManager
}

func NewStreamer(cacheMgr *CacheManager) *Streamer {
	return &Streamer{cacheMgr: cacheMgr}
}

// CreateOpusStream spawns yt-dlp + ffmpeg (or streams cached file + ffmpeg) and encodes raw PCM into Opus frames for Discord
func (s *Streamer) StreamAudio(ctx context.Context, targetURL, videoID, filterName string, cookiesPath, ytdlpClients string, volume float64) (<-chan []byte, chan struct{}, error) {
	var ffmpegInput string
	var ytdlpCmd *exec.Cmd

	// Check disk cache first
	if cachedPath, ok := s.cacheMgr.GetCachedPath(videoID); ok {
		ffmpegInput = cachedPath
	} else {
		// Spawn yt-dlp process
		ytdlpArgs := []string{
			"--extractor-args", fmt.Sprintf("youtube:player_client=%s", ytdlpClients),
			"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"-f", "ba[ext=m4a][abr<=128]/ba[ext=webm][abr<=160]/ba/bestaudio",
			"--no-playlist",
			"--geo-bypass",
			"--no-check-certificates",
			"-o", "-",
			targetURL,
		}

		if _, err := os.Stat(cookiesPath); err == nil {
			ytdlpArgs = append([]string{"--cookies", cookiesPath}, ytdlpArgs...)
		}

		ytdlpCmd = exec.CommandContext(ctx, "yt-dlp", ytdlpArgs...)
	}

	// Prepare FFmpeg args
	ffmpegArgs := []string{
		"-re",
		"-i", "pipe:0",
		"-analyzeduration", "0",
		"-loglevel", "quiet",
		"-f", "s16le",
		"-ar", "48000",
		"-ac", "2",
	}

	// Add dynamic volume adjustment filter
	volFilter := fmt.Sprintf("volume=%.2f", volume)
	dspFilters := GetFFmpegFilterArgs(filterName)
	if len(dspFilters) > 0 {
		ffmpegArgs = append(ffmpegArgs, "-af", dspFilters[1]+","+volFilter)
	} else {
		ffmpegArgs = append(ffmpegArgs, "-af", volFilter)
	}

	ffmpegArgs = append(ffmpegArgs, "pipe:1")

	var ffmpegCmd *exec.Cmd
	if ffmpegInput != "" {
		// From cached file
		args := append([]string{"-re", "-i", ffmpegInput}, ffmpegArgs[3:]...)
		ffmpegCmd = exec.CommandContext(ctx, "ffmpeg", args...)
	} else {
		ffmpegCmd = exec.CommandContext(ctx, "ffmpeg", ffmpegArgs...)
	}

	ffmpegStdout, err := ffmpegCmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get ffmpeg stdout: %w", err)
	}

	if ffmpegInput == "" {
		ytdlpStdout, err := ytdlpCmd.StdoutPipe()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get yt-dlp stdout: %w", err)
		}
		ffmpegCmd.Stdin = ytdlpStdout

		if err := ytdlpCmd.Start(); err != nil {
			return nil, nil, fmt.Errorf("failed to start yt-dlp: %w", err)
		}
	}

	if err := ffmpegCmd.Start(); err != nil {
		if ytdlpCmd != nil && ytdlpCmd.Process != nil {
			_ = ytdlpCmd.Process.Kill()
		}
		return nil, nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Setup Opus encoder (48000Hz, 2 channels, VoIP mode)
	encoder, err := gopus.NewEncoder(48000, 2, gopus.Audio)
	if err != nil {
		_ = ffmpegCmd.Process.Kill()
		if ytdlpCmd != nil && ytdlpCmd.Process != nil {
			_ = ytdlpCmd.Process.Kill()
		}
		return nil, nil, fmt.Errorf("failed to create opus encoder: %w", err)
	}

	opusChan := make(chan []byte, 100)
	stopChan := make(chan struct{})

	go func() {
		defer close(opusChan)
		defer func() {
			if ffmpegCmd.Process != nil {
				_ = ffmpegCmd.Process.Kill()
			}
			if ytdlpCmd != nil && ytdlpCmd.Process != nil {
				_ = ytdlpCmd.Process.Kill()
			}
		}()

		const frameSize = 960 // 20ms at 48kHz
		const pcmBufferSize = frameSize * 2 * 2 // 960 samples * 2 channels * 2 bytes per sample (s16le) = 3840 bytes

		pcmBuf := make([]int16, frameSize*2)
		rawBuf := make([]byte, pcmBufferSize)
		reader := bufio.NewReaderSize(ffmpegStdout, pcmBufferSize*4)

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			default:
				_, err := io.ReadFull(reader, rawBuf)
				if err != nil {
					return
				}

				// Convert s16le bytes to int16 PCM samples
				for i := 0; i < len(pcmBuf); i++ {
					pcmBuf[i] = int16(binary.LittleEndian.Uint16(rawBuf[i*2 : (i+1)*2]))
				}

				// Encode PCM to Opus frame
				opusFrame, err := encoder.Encode(pcmBuf, frameSize, maxOpusBytes)
				if err != nil || len(opusFrame) == 0 {
					continue
				}

				opusChan <- opusFrame
			}
		}
	}()

	return opusChan, stopChan, nil
}

const maxOpusBytes = 4000
