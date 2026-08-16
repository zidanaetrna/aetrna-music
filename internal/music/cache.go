package music

import (
	"sync"
	"time"
)

type cacheItem struct {
	streamURL  string
	resolvedAt time.Time
}

type StreamCache struct {
	items map[string]cacheItem
	ttl   time.Duration
	mu    sync.RWMutex
}

var GlobalStreamCache = NewStreamCache(10 * time.Minute)

func NewStreamCache(ttl time.Duration) *StreamCache {
	c := &StreamCache{
		items: make(map[string]cacheItem),
		ttl:   ttl,
	}
	go c.startCleanupLoop()
	return c
}

func (c *StreamCache) Get(url string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[url]
	if !exists {
		return "", false
	}

	if time.Since(item.resolvedAt) > c.ttl {
		return "", false
	}

	return item.streamURL, true
}

func (c *StreamCache) Set(url, streamURL string) {
	if url == "" || streamURL == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[url] = cacheItem{
		streamURL:  streamURL,
		resolvedAt: time.Now(),
	}
}

func (c *StreamCache) startCleanupLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for url, item := range c.items {
			if now.Sub(item.resolvedAt) > c.ttl {
				delete(c.items, url)
			}
		}
		c.mu.Unlock()
	}
}
