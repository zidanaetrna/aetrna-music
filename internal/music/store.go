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
	trackDoneCb TrackDoneCallback
	haltCb      HaltCallback
	mu         sync.RWMutex
}

// SetTrackDoneCallback sets the track-done callback on every current and future queue.
func (s *QueueStore) SetTrackDoneCallback(cb TrackDoneCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trackDoneCb = cb
	for _, q := range s.queues {
		q.mu.Lock()
		q.TrackDoneCb = cb
		q.mu.Unlock()
	}
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
			// Keep queues touched recently: a /play may still be searching
			// before it adds its song, and would otherwise land in an orphan.
			idle := !q.isLooping && len(q.Songs) == 0 && q.idleTimer == nil &&
				time.Since(q.lastAccess) > 10*time.Minute
			q.mu.RUnlock()

			if idle {
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
		queue.TrackDoneCb = s.trackDoneCb
		queue.HaltCb = s.haltCb
		s.queues[guildID] = queue
	}
	queue.Touch()
	return queue
}

// SetHaltCallback sets the halt callback on every current and future queue.
func (s *QueueStore) SetHaltCallback(cb HaltCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.haltCb = cb
	for _, q := range s.queues {
		q.mu.Lock()
		q.HaltCb = cb
		q.mu.Unlock()
	}
}

// Lookup returns an existing queue (or nil) without creating one or marking it
// as used. Use it for background readers like the dashboard and snapshotter so
// they don't keep idle queues alive.
func (s *QueueStore) Lookup(guildID string) *GuildQueue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.queues[guildID]
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
