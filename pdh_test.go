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
		t.Log("NVML no disponible → n/d (AMD/Intel/sin driver NVIDIA)")
	}
}

// Aísla el buffer-dance de PDH fuera del proceso completo.
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
		t.Log("engine3D: sin datos")
	}

	total := vramTotalGB()
	t.Logf("vramTotalGB=%.2f LUIDs=%v", total, mainAdapterLUIDs)
	used := g.VRAMUsedGB()
	t.Logf("VRAMUsedGB=%.2f", used)
	if total > 0 && used >= 0 && used > total+0.5 {
		t.Errorf("VRAM usada (%.2fGB) > total (%.2fGB): el filtro LUID no filtra", used, total)
	}
}
