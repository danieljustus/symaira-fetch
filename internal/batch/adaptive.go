package batch

import (
	"sync"
	"time"
)

type hostStats struct {
	mu           sync.Mutex
	concurrency  int
	latencySum   time.Duration
	latencyCount int
	errorCount   int
	successCount int
	lastUpdate   time.Time
}

type AdaptivePool struct {
	minPerHost int
	maxPerHost int
	stats      map[string]*hostStats
	mu         sync.Mutex
}

func NewAdaptivePool(minPerHost, maxPerHost int) *AdaptivePool {
	if minPerHost <= 0 {
		minPerHost = 2
	}
	if maxPerHost <= 0 {
		maxPerHost = 8
	}
	return &AdaptivePool{
		minPerHost: minPerHost,
		maxPerHost: maxPerHost,
		stats:      make(map[string]*hostStats),
	}
}

func (ap *AdaptivePool) getOrCreateStats(host string) *hostStats {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	s, ok := ap.stats[host]
	if !ok {
		s = &hostStats{
			concurrency: ap.minPerHost,
			lastUpdate:  time.Now(),
		}
		ap.stats[host] = s
	}
	return s
}

func (ap *AdaptivePool) GetConcurrency(host string) int {
	s := ap.getOrCreateStats(host)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.concurrency
}

func (ap *AdaptivePool) RecordSuccess(host string, latency time.Duration) {
	s := ap.getOrCreateStats(host)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.successCount++
	s.latencySum += latency
	s.latencyCount++
	s.lastUpdate = time.Now()

	if s.successCount >= 5 && s.errorCount == 0 {
		if s.concurrency < ap.maxPerHost {
			s.concurrency++
		}
		s.successCount = 0
		s.errorCount = 0
		s.latencySum = 0
		s.latencyCount = 0
	}
}

func (ap *AdaptivePool) RecordFailure(host string) {
	s := ap.getOrCreateStats(host)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errorCount++
	s.successCount = 0
	s.lastUpdate = time.Now()

	if s.errorCount >= 2 {
		if s.concurrency > ap.minPerHost {
			s.concurrency--
		}
		s.errorCount = 0
	}
}

func (ap *AdaptivePool) Reset(host string) {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	delete(ap.stats, host)
}
