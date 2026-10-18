package main

import (
	"testing"
	"time"
)

// Regresión del bug "FPS siempre 1": el consumer ETW entrega los eventos en
// ráfagas; el dedup y la ventana deben usar el timestamp del evento, no la
// hora de consumo. Con time.Now() una ráfaga de N presentes colapsaba a 1.
func TestFPSTrackerBurst(t *testing.T) {
	tr := &FPSTracker{events: map[uint32][]time.Time{}, last: map[uint32]time.Time{}}
	now := time.Now()
	// 100 presents cada 10ms, entregados TODOS de golpe (lo que hace el consumer).
	for i := 99; i >= 0; i-- {
		tr.add(42, now.Add(-time.Duration(i)*10*time.Millisecond))
	}
	if fps := tr.FPS(42); fps != 100 {
		t.Fatalf("FPS = %d, want 100 (la ráfaga se deduplicó por hora de consumo)", fps)
	}
}

// El par runtime+kernel del mismo present (≤2ms) sí se deduplica.
func TestFPSTrackerDedupPair(t *testing.T) {
	tr := &FPSTracker{events: map[uint32][]time.Time{}, last: map[uint32]time.Time{}}
	base := time.Now()
	tr.add(7, base)
	tr.add(7, base.Add(1*time.Millisecond)) // correlacionado: mismo present
	tr.add(7, base.Add(5*time.Millisecond)) // present nuevo
	if fps := tr.FPS(7); fps != 2 {
		t.Fatalf("FPS = %d, want 2 (par correlacionado mal deduplicado)", fps)
	}
}
