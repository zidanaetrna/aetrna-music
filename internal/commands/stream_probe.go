package commands

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

// streamUserAgent matches the User-Agent the voice server's ffmpeg sends.
const streamUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

// maxStreamResolveAttempts bounds how many fresh URLs GetPlayableStreamURL asks
// yt-dlp for before giving up, so the caller can fall back to letting yt-dlp
// download the song itself (which gets through where direct URLs don't).
const maxStreamResolveAttempts = 2

// ErrStreamRejected means yt-dlp returned stream URLs but YouTube refused to
// serve them.
var ErrStreamRejected = errors.New("YouTube rejected the stream URL")

// probeClient dials over IPv4 only: yt-dlp runs with -4, and googlevideo URLs
// are bound to the IP that requested them.
var probeClient = &http.Client{
	Timeout: 8 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp4", addr)
		},
		TLSHandshakeTimeout: 5 * time.Second,
	},
}

// ProbeStreamURL checks that YouTube actually serves a stream URL by fetching
// its first byte. YouTube sometimes hands out URLs that answer 403 right away.
func ProbeStreamURL(streamURL string) error {
	req, err := http.NewRequest(http.MethodGet, streamURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", streamUserAgent)
	req.Header.Set("Range", "bytes=0-0")
	resp, err := probeClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("stream URL returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// GetPlayableStreamURL resolves a stream URL and probes it, asking yt-dlp for a
// fresh one when YouTube rejects it. Probing here takes a fraction of a second,
// while letting ffmpeg fail costs a full round trip through the voice server.
func GetPlayableStreamURL(query, cookiesPath, ytdlpClients string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= maxStreamResolveAttempts; attempt++ {
		streamURL, err := GetStreamURL(query, cookiesPath, ytdlpClients)
		if err != nil {
			return "", err // yt-dlp itself failed; it already tried its fallbacks
		}
		if lastErr = ProbeStreamURL(streamURL); lastErr == nil {
			if attempt > 1 {
				log.Printf("[INFO] [GetPlayableStreamURL] Got a working stream URL on attempt %d for '%s'", attempt, query)
			}
			return streamURL, nil
		}
		log.Printf("[WARN] [GetPlayableStreamURL] Attempt %d/%d for '%s': %v", attempt, maxStreamResolveAttempts, query, lastErr)
	}
	return "", fmt.Errorf("%w after %d attempts: %v", ErrStreamRejected, maxStreamResolveAttempts, lastErr)
}
