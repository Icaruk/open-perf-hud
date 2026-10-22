package main

import (
	"testing"
	"time"
)

func newTracker() *FPSTracker {
	return &FPSTracker{
		events: map[uint32][]time.Time{},
		lastR:  map[uint32]time.Time{},
		lastK:  map[uint32]time.Time{},
	}
}

// Regression test for the "FPS always 1" bug: the ETW consumer delivers
// events in bursts; the dedup and the window must use the event timestamp,
// not the consumption time. With time.Now() a burst of N presents collapsed
// to 1.
func TestFPSTrackerBurst(t *testing.T) {
	tr := newTracker()
	now := time.Now()
	// 100 presents every 10ms, ALL delivered at once (what the consumer does).
	for i := 99; i >= 0; i-- {
		tr.add(42, now.Add(-time.Duration(i)*10*time.Millisecond), false)
	}
	if fps := tr.FPS(42); fps != 100 {
		t.Fatalf("FPS = %d, want 100 (the burst was deduplicated by consumption time)", fps)
	}
}

// Two REAL runtime frames <2ms apart (engine catching up after a hitch) both
// count. The old symmetric dedup swallowed the second one — Outer Wilds read
// 40 instead of ~60.
func TestFPSTrackerRuntimeFramesCloseTogether(t *testing.T) {
	tr := newTracker()
	base := time.Now()
	tr.add(7, base, false)
	tr.add(7, base.Add(1*time.Millisecond), false)
	if fps := tr.FPS(7); fps != 2 {
		t.Fatalf("FPS = %d, want 2 (real runtime frames wrongly deduplicated)", fps)
	}
}

// The runtime+kernel pair of the same present (≤2ms) IS deduplicated.
func TestFPSTrackerDedupPair(t *testing.T) {
	tr := newTracker()
	base := time.Now()
	tr.add(7, base, false)
	tr.add(7, base.Add(1*time.Millisecond), true) // kernel mirror: same present
	tr.add(7, base.Add(5*time.Millisecond), false)
	if fps := tr.FPS(7); fps != 2 {
		t.Fatalf("FPS = %d, want 2 (mirror pair not deduplicated)", fps)
	}
}

// Regression for the chain collapse: runtime frame, its kernel mirror, and a
// REAL runtime frame 1.5ms after the mirror must count 2. The old dedup
// updated `last` on the dropped mirror and swallowed the real frame → FPS
// read 8 after a stutter.
func TestFPSTrackerNoChainCollapse(t *testing.T) {
	tr := newTracker()
	base := time.Now()
	tr.add(7, base, false)                          // real frame
	tr.add(7, base.Add(500*time.Microsecond), true) // its kernel mirror (dropped)
	tr.add(7, base.Add(2*time.Millisecond), false)  // real frame, 1.5ms after the mirror
	if fps := tr.FPS(7); fps != 2 {
		t.Fatalf("FPS = %d, want 2 (chain collapse: dropped mirror swallowed the next real frame)", fps)
	}
}

// Kernel-only games (Vulkan/OpenGL): <2ms pairs still dedup (documented
// ~500 FPS ceiling, same as before).
func TestFPSTrackerKernelOnlyPair(t *testing.T) {
	tr := newTracker()
	base := time.Now()
	tr.add(9, base, true)
	tr.add(9, base.Add(1*time.Millisecond), true)
	if fps := tr.FPS(9); fps != 1 {
		t.Fatalf("FPS = %d, want 1 (kernel-only pair should dedup)", fps)
	}
}
