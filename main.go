package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/tekert/goetw/etw"
	"golang.org/x/sys/windows"
)

const version = "1.0.0"

func usage() {
	fmt.Print(`Open Perf HUD ` + version + ` — FPS/CPU/GPU/RAM overlay

Usage:
  open-perf-hud.exe            overlay (asks for UAC only if the system requires admin for ETW)
  open-perf-hud.exe --cli      console metrics (FPS only if ETW starts)
  open-perf-hud.exe --probe [dur]  ETW event histogram, e.g. --probe 10s

Config: config.json next to the exe, or the system tray menu.
`)
}

// elevate relanza el proceso pidiendo UAC. Devuelve false si el usuario cancela.
func elevate() bool {
	exe, _ := os.Executable()
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	return windows.ShellExecute(0, verb, file, nil, nil, 1 /*SW_SHOWNORMAL*/) == nil
}

func main() {
	etw.DisableLogging() // goetw loguea JSON a stderr; lo callamos (probe imprime lo suyo)
	args := os.Args[1:]
	switch {
	case len(args) == 0:
		runOverlayMode()
	case args[0] == "--cli":
		runCLI()
	case args[0] == "--probe":
		d := 15 * time.Second
		if len(args) > 1 {
			if parsed, err := time.ParseDuration(args[1]); err == nil {
				d = parsed
			} else if n, err := strconv.Atoi(args[1]); err == nil {
				d = time.Duration(n) * time.Second
			}
		}
		runProbe(d)
	default:
		usage()
	}
}

// startMeters: config y medidores comunes a overlay y CLI. Si ETW no arranca
// (normalmente sin admin) tracker y cleanup vuelven nil y cada modo decide:
// el overlay prueba UAC, la CLI avisa y sigue sin FPS.
func startMeters() (Config, *CPUMeter, *GPUMeter, *FPSTracker, func(), error) {
	cfg := LoadConfig()
	cpu := &CPUMeter{}
	gpu := NewGPUMeter()
	tracker, cleanup, err := StartFPS(context.Background())
	return cfg, cpu, gpu, tracker, cleanup, err
}

func runOverlayMode() {
	mu, existed := grabInstanceMutex()
	if existed {
		windows.CloseHandle(windows.Handle(mu)) // no retenemos el mutex ajeno
		if !askRestart() {
			return
		}
		restartRunning()
		// espera a que la instancia anterior cierre y suelte el mutex
		for range 30 {
			time.Sleep(100 * time.Millisecond)
			if mu, existed = grabInstanceMutex(); !existed {
				break
			}
			windows.CloseHandle(windows.Handle(mu))
		}
		if existed {
			return // no terminó a tiempo; no arrancamos una segunda instancia
		}
	}

	cfg, cpu, gpu, tracker, cleanup, err := startMeters()
	if err != nil {
		// En muchos sistemas la sesión ETW arranca sin admin; si este la rechaza, UAC.
		if elevate() {
			windows.CloseHandle(windows.Handle(mu)) // el hijo elevado debe poder crearlo
			return
		}
		fmt.Println("Warning: ETW unavailable, FPS disabled:", err)
	}
	defer gpu.Close()
	if err := runOverlay(cfg, tracker, cpu, gpu, cleanup); err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}

func runCLI() {
	cfg, cpu, gpu, tracker, cleanup, err := startMeters()
	defer gpu.Close()
	if err != nil {
		fmt.Println("Warning: no FPS without admin (", err, "). System metrics still work.")
	}
	if cleanup != nil {
		defer cleanup()
	}

	fmt.Println("Open Perf HUD CLI — Ctrl+C to exit")
	tick := time.NewTicker(time.Duration(cfg.RefreshMs) * time.Millisecond)
	defer tick.Stop()
	printLine := func() {
		s := collectSnapshot(tracker, cpu, gpu)
		fmt.Printf("\r%-24s FPS %s  CPU %5.1f%%  GPU %5s %s  VRAM %11s  RAM %2d%% %s",
			s.NameTxt(), s.FPSTxt(), s.CPU, s.GPUTxt(), s.TempTxt(), s.VRAMTxt(), s.RAMLoad, s.RAMTxt())
	}
	for range tick.C {
		printLine()
	}
}
