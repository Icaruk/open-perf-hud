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

// Kernel-only games (Vulkan/OpenGL): with no runtime stream at all, kernel
// events ARE the channel and count normally.
func TestFPSTrackerKernelFallback(t *testing.T) {
	tr := newTracker()
	base := time.Now()
	for i := range 3 {
		tr.add(9, base.Add(time.Duration(i)*16*time.Millisecond), true)
	}
	if fps := tr.FPS(9); fps != 3 {
		t.Fatalf("FPS = %d, want 3 (kernel fallback channel not counting)", fps)
	}
}

// The Outer Wilds regression: while the runtime stream is alive, kernel
// events near real frames (DWM noise or mirrors) are ALL ignored — none of
// them may eat a runtime frame.
func TestFPSTrackerKernelNoiseNeverEatsRuntime(t *testing.T) {
	tr := newTracker()
	base := time.Now()
	tr.add(7, base, false)                         // real frame
	tr.add(7, base.Add(1*time.Millisecond), true)  // mirror
	tr.add(7, base.Add(8*time.Millisecond), true)  // DWM noise
	tr.add(7, base.Add(15*time.Millisecond), true) // noise, 1ms before the next real frame
	tr.add(7, base.Add(16*time.Millisecond), false)
	if fps := tr.FPS(7); fps != 2 {
		t.Fatalf("FPS = %d, want 2 (kernel noise ate or added frames)", fps)
	}
}

// Mode switch / runtime death: after runtime is silent past runtimeGate,
// kernel events take over. The stale runtime frame is correctly expired from
// the 1s window by then; what matters is that the kernel event counts
// (a broken gate would drop it and read 0).
func TestFPSTrackerModeSwitch(t *testing.T) {
	tr := newTracker()
	base := time.Now()
	tr.add(7, base, false)
	tr.add(7, base.Add(2*runtimeGate), true) // runtime silent: kernel adopted
	if fps := tr.FPS(7); fps != 1 {
		t.Fatalf("FPS = %d, want 1 (kernel fallback not adopted after runtime silence)", fps)
	}
}
