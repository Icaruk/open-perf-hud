package main

import (
	"testing"
	"time"
)

// NVML: temperatura GPU real (hardware-dependiente, log-only).
func TestGPUTemp(t *testing.T) {
	if temp := GPUTempC(); temp >= 0 {
		t.Logf("GPU temp: %.0f°C", temp)
	} else {
		t.Log("NVML unavailable → n/a (AMD/Intel/no NVIDIA driver)")
	}
}

// Isolates the PDH buffer-dance outside the full process.
func TestPDHArray(t *testing.T) {
	g := NewGPUMeter()
	defer g.Close()
	if g.hq == 0 {
		t.Fatal("PDH query did not open")
	}
	procPdhCollectQueryData.Call(g.hq)
	time.Sleep(300 * time.Millisecond)
	procPdhCollectQueryData.Call(g.hq)

	if sum, ok := g.sumCounter(g.hcEng, nil); ok {
		t.Logf("engine3D sum=%.3f%%", sum)
	} else {
		t.Log("engine3D: no data")
	}

	total := vramTotalGB()
	t.Logf("vramTotalGB=%.2f LUIDs=%v", total, mainAdapterLUIDs)
	used := g.VRAMUsedGB()
	t.Logf("VRAMUsedGB=%.2f", used)
	if total > 0 && used >= 0 && used > total+0.5 {
		t.Errorf("VRAM used (%.2fGB) > total (%.2fGB): the LUID filter doesn't filter", used, total)
	}
}
