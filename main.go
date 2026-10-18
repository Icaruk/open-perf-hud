package main

import "fmt"

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

func main() {
	usage()
}
