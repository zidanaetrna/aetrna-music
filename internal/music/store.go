package music

import (
	"sync"

	"aetrna-music/internal/audio"
)

type QueueStore struct {
	queues   map[string]*GuildQueue
	streamer *audio.Streamer
	mu       sync.RWMutex
}

func NewQueueStore(streamer *audio.Streamer) *QueueStore {
	return &QueueStore{
		queues:   make(map[string]*GuildQueue),
		streamer: streamer,
	}
}

func (s *QueueStore) Get(guildID string) *GuildQueue {
	s.mu.Lock()
	defer s.mu.Unlock()

	queue, exists := s.queues[guildID]
	if !exists {
		queue = NewGuildQueue(guildID, s.streamer)
		s.queues[guildID] = queue
	}
	return queue
}

func (s *QueueStore) Remove(guildID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if q, exists := s.queues[guildID]; exists {
		q.Stop()
		delete(s.queues, guildID)
	}
}
