# Changelog

## [1.4.0] - 2026-10-22

### Changed

- Replace the "already running" warning: launching the app again now closes the old instance and opens the new one

### Added

- Add a "Graphics API" line showing the API the game uses (DX9, DX11, DX12, Vulkan or OpenGL); off by default, enable it in Settings → Metrics
- Add a contrast outline around the text so it stays readable on any background, light or dark, in any text color (including "Dynamic"); thickness configurable in Settings → Outline (none / sm / md / lg)

### Fixed

- Fix the FPS counter showing lower numbers than the real ones in some games (Outer Wilds was the reported case); it now matches other counters like Steam's

## [1.3.1] - 2026-10-22

### Fixed

- Fix the FPS misreading frames in some games: the two notifications Windows sends for each frame are now told apart, so each frame counts exactly once
- Report lost events in diagnostic mode (`--probe`), to rule out that cause in future reports

## [1.3.0] - 2026-10-21

### Changed

- Turn alert values red in every text color (CPU/GPU at 99 %, RAM/VRAM at 95 %), with the red adapting its brightness to stay readable

### Added

- Add a settings window (tray icon → Settings…)
- Add free pixel offsets from each screen edge (0–9999), replacing the fixed list
- Add background opacity in steps of 5 %, replacing six fixed values

## [1.2.0] - 2026-10-20

### Fixed

- Fix automatic text color staying white over light mixed backgrounds (a light green with darker areas, for example); the overlay now measures the background correctly and switches to dark text when the area under it is light

## [1.1.0] - 2026-10-19

### Added

- Add "Dynamic" text color (new default): samples the background each refresh and picks white on dark / dark-grey on light backgrounds

## [1.0.0] - 2026-10-18

_First release: single 4 MB exe, no installer, no dependencies. Windows 10/11 x64. Exclusive fullscreen is not supported — use borderless windowed._

### Added

- Add an always-on-top, click-through overlay with per-pixel alpha showing FPS of the focused app (from ETW present events, no injected DLL), CPU %, RAM, GPU %, VRAM and GPU temperature
- Add tray menu, `CTRL+SHIFT+O` hotkey, single instance, and settings saved to `config.json` next to the exe

[1.4.0]: https://github.com/Icaruk/open-perf-hud/releases/tag/v1.4.0
[1.3.1]: https://github.com/Icaruk/open-perf-hud/releases/tag/v1.3.1
[1.3.0]: https://github.com/Icaruk/open-perf-hud/releases/tag/v1.3.0
[1.2.0]: https://github.com/Icaruk/open-perf-hud/releases/tag/v1.2.0
[1.1.0]: https://github.com/Icaruk/open-perf-hud/releases/tag/v1.1.0
[1.0.0]: https://github.com/Icaruk/open-perf-hud/releases/tag/v1.0.0
