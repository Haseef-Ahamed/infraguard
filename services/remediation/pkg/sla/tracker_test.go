package sla_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/infraguard/remediation/pkg/sla"
)

func TestTrackEvent_AddsToTracker(t *testing.T) {
	tr := sla.NewTracker(30, nil)
	tr.TrackEvent("sg-abc123")
	assert.Equal(t, 1, tr.TrackedCount())
}

func TestTrackEvent_Idempotent(t *testing.T) {
	tr := sla.NewTracker(30, nil)
	tr.TrackEvent("sg-abc123")
	tr.TrackEvent("sg-abc123") // duplicate — should not reset timer or double-count
	assert.Equal(t, 1, tr.TrackedCount())
}

func TestResolveEvent_RemovesFromTracker(t *testing.T) {
	tr := sla.NewTracker(30, nil)
	tr.TrackEvent("sg-abc123")
	tr.ResolveEvent("sg-abc123")
	assert.Equal(t, 0, tr.TrackedCount())
}

func TestCheckBreaches_FiresCallbackAfterSLA(t *testing.T) {
	var mu sync.Mutex
	breached := make(map[string]int)

	// Use a 0-minute SLA so any tracked event immediately breaches
	tr := sla.NewTracker(0, func(resourceID string, minutes int) {
		mu.Lock()
		defer mu.Unlock()
		breached[resourceID] = minutes
	})

	tr.TrackEvent("sg-abc123")
	time.Sleep(10 * time.Millisecond)
	tr.CheckBreaches()

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, breached, "sg-abc123")
}

func TestCheckBreaches_DoesNotFireBeforeSLA(t *testing.T) {
	var mu sync.Mutex
	breached := make(map[string]int)

	// 60-minute SLA — should not breach immediately
	tr := sla.NewTracker(60, func(resourceID string, minutes int) {
		mu.Lock()
		defer mu.Unlock()
		breached[resourceID] = minutes
	})

	tr.TrackEvent("sg-abc123")
	tr.CheckBreaches()

	mu.Lock()
	defer mu.Unlock()
	assert.NotContains(t, breached, "sg-abc123")
}

func TestResolveEvent_PreventsFutureBreach(t *testing.T) {
	var mu sync.Mutex
	breached := make(map[string]int)

	tr := sla.NewTracker(0, func(resourceID string, minutes int) {
		mu.Lock()
		defer mu.Unlock()
		breached[resourceID] = minutes
	})

	tr.TrackEvent("sg-abc123")
	tr.ResolveEvent("sg-abc123")
	time.Sleep(10 * time.Millisecond)
	tr.CheckBreaches()

	mu.Lock()
	defer mu.Unlock()
	assert.NotContains(t, breached, "sg-abc123")
}
