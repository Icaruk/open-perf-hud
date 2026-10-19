package main

import (
	"testing"
	"time"
)

// Regression test for the "FPS always 1" bug: the ETW consumer delivers
// events in bursts; the dedup and the window must use the event timestamp,
// not the consumption time. With time.Now() a burst of N presents collapsed
// to 1.
func TestFPSTrackerBurst(t *testing.T) {
	tr := &FPSTracker{events: map[uint32][]time.Time{}, last: map[uint32]time.Time{}}
	now := time.Now()
	// 100 presents every 10ms, ALL delivered at once (what the consumer does).
	for i := 99; i >= 0; i-- {
		tr.add(42, now.Add(-time.Duration(i)*10*time.Millisecond))
	}
	if fps := tr.FPS(42); fps != 100 {
		t.Fatalf("FPS = %d, want 100 (the burst was deduplicated by consumption time)", fps)
	}
}

// The runtime+kernel pair of the same present (≤2ms) IS deduplicated.
func TestFPSTrackerDedupPair(t *testing.T) {
	tr := &FPSTracker{events: map[uint32][]time.Time{}, last: map[uint32]time.Time{}}
	base := time.Now()
	tr.add(7, base)
	tr.add(7, base.Add(1*time.Millisecond)) // correlated: same present
	tr.add(7, base.Add(5*time.Millisecond)) // new present
	if fps := tr.FPS(7); fps != 2 {
		t.Fatalf("FPS = %d, want 2 (correlated pair wrongly deduplicated)", fps)
	}
}
