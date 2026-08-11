package audio

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/jonas747/dca"
)

type Streamer struct {
	cacheMgr *CacheManager
}

func NewStreamer(cacheMgr *CacheManager) *Streamer {
	return &Streamer{cacheMgr: cacheMgr}
}

// StreamAudio pipes audio through yt-dlp + ffmpeg/dca into Discord Opus frames without CGO dependencies
func (s *Streamer) StreamAudio(ctx context.Context, targetURL, videoID, filterName string, cookiesPath, ytdlpClients string, volume float64) (<-chan []byte, chan struct{}, error) {
	var ffmpegInput string
	var ytdlpCmd *exec.Cmd

	// Check disk cache first
	if cachedPath, ok := s.cacheMgr.GetCachedPath(videoID); ok {
		ffmpegInput = cachedPath
	} else {
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

	opts := dca.StdEncodeOptions
	opts.RawOutput = true
	opts.Bitrate = 128
	opts.Application = dca.AudioApplicationAudio
	opts.Volume = int(volume * 256)

	dspFilters := GetFFmpegFilterArgs(filterName)
	if len(dspFilters) > 0 {
		opts.AudioFilter = dspFilters[1]
	}

	var encodeSession *dca.EncodeSession
	var err error

	if ffmpegInput != "" {
		encodeSession, err = dca.EncodeFile(ffmpegInput, opts)
	} else {
		ytdlpStdout, errPipe := ytdlpCmd.StdoutPipe()
		if errPipe != nil {
			return nil, nil, fmt.Errorf("failed to get yt-dlp stdout: %w", errPipe)
		}
		if errStart := ytdlpCmd.Start(); errStart != nil {
			return nil, nil, fmt.Errorf("failed to start yt-dlp: %w", errStart)
		}
		encodeSession, err = dca.EncodeMem(ytdlpStdout, opts)
	}

	if err != nil {
		if ytdlpCmd != nil && ytdlpCmd.Process != nil {
			_ = ytdlpCmd.Process.Kill()
		}
		return nil, nil, fmt.Errorf("failed to start dca encoding: %w", err)
	}

	opusChan := make(chan []byte, 100)
	stopChan := make(chan struct{})

	go func() {
		defer close(opusChan)
		defer encodeSession.Cleanup()
		defer func() {
			if ytdlpCmd != nil && ytdlpCmd.Process != nil {
				_ = ytdlpCmd.Process.Kill()
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopChan:
				return
			default:
				frame, err := encodeSession.ReadFrame()
				if err != nil {
					if err != io.EOF && err != io.ErrUnexpectedEOF {
						// Stream ended
					}
					return
				}
				opusChan <- frame
			}
		}
	}()

	return opusChan, stopChan, nil
}
