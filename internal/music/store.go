package music

import (
	"sync"
)

type QueueStore struct {
	queues map[string]*GuildQueue
	playCb PlayCallback
	stopCb StopCallback
	mu     sync.RWMutex
}

func NewQueueStore(playCb PlayCallback, stopCb StopCallback) *QueueStore {
	return &QueueStore{
		queues: make(map[string]*GuildQueue),
		playCb: playCb,
		stopCb: stopCb,
	}
}

func (s *QueueStore) Get(guildID string) *GuildQueue {
	s.mu.Lock()
	defer s.mu.Unlock()

	queue, exists := s.queues[guildID]
	if !exists {
		queue = NewGuildQueue(guildID, s.playCb, s.stopCb)
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
