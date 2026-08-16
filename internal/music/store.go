package music

import (
	"sync"
	"time"
)

type QueueStore struct {
	queues     map[string]*GuildQueue
	playCb     PlayCallback
	stopCb     StopCallback
	preFetchCb PreFetchCallback
	mu         sync.RWMutex
}

func NewQueueStore(playCb PlayCallback, stopCb StopCallback, preFetchCb PreFetchCallback) *QueueStore {
	s := &QueueStore{
		queues:     make(map[string]*GuildQueue),
		playCb:     playCb,
		stopCb:     stopCb,
		preFetchCb: preFetchCb,
	}
	go s.startGARoutine()
	return s
}

func (s *QueueStore) startGARoutine() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		for guildID, q := range s.queues {
			q.mu.RLock()
			isPlaying := q.IsPlaying
			hasSongs := len(q.Songs) > 0
			q.mu.RUnlock()

			if !isPlaying && !hasSongs {
				delete(s.queues, guildID)
			}
		}
		s.mu.Unlock()
	}
}

func (s *QueueStore) Get(guildID string) *GuildQueue {
	s.mu.Lock()
	defer s.mu.Unlock()

	queue, exists := s.queues[guildID]
	if !exists {
		queue = NewGuildQueue(guildID, s.playCb, s.stopCb, s.preFetchCb)
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

func (s *QueueStore) GetActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.queues)
}

func (s *QueueStore) GetAllGuildIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.queues))
	for id := range s.queues {
		ids = append(ids, id)
	}
	return ids
}
