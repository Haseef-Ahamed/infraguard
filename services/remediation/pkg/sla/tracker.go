package sla

import (
	"sync"
	"time"
)

// Tracker monitors open drift events and fires a callback when
// they exceed their SLA window without being resolved.
type Tracker struct {
	mu       sync.Mutex
	entries  map[string]time.Time // resourceID -> detected_at
	slaMins  int
	onBreach func(resourceID string, minutesElapsed int)
	stopCh   chan struct{}
}

// NewTracker creates an SLA tracker. slaMinutes is the breach threshold.
func NewTracker(slaMinutes int, onBreach func(string, int)) *Tracker {
	return &Tracker{
		entries:  make(map[string]time.Time),
		slaMins:  slaMinutes,
		onBreach: onBreach,
		stopCh:   make(chan struct{}),
	}
}

// TrackEvent registers a new drift event to be monitored for SLA breach
func (t *Tracker) TrackEvent(resourceID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.entries[resourceID]; !exists {
		t.entries[resourceID] = time.Now()
	}
}

// ResolveEvent removes an event from tracking (e.g. PR merged, drift reverted)
func (t *Tracker) ResolveEvent(resourceID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, resourceID)
}

// CheckBreaches scans all tracked events and fires onBreach for any
// that have exceeded the SLA window. Safe to call repeatedly (e.g. on a ticker).
func (t *Tracker) CheckBreaches() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for resourceID, detectedAt := range t.entries {
		elapsed := now.Sub(detectedAt)
		if elapsed >= time.Duration(t.slaMins)*time.Minute {
			minutesElapsed := int(elapsed.Minutes())
			if t.onBreach != nil {
				t.onBreach(resourceID, minutesElapsed)
			}
		}
	}
}

// TrackedCount returns how many events are currently being monitored
func (t *Tracker) TrackedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.entries)
}

// Run starts a background loop that calls CheckBreaches every interval.
// Call Stop() to terminate it.
func (t *Tracker) Run(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.CheckBreaches()
			case <-t.stopCh:
				return
			}
		}
	}()
}

// Stop terminates the background Run loop
func (t *Tracker) Stop() {
	close(t.stopCh)
}
