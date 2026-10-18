# <img src="icon.svg" width="28" align="top" alt=""> Open Perf HUD

FPS / CPU / GPU / RAM overlay for windowed and borderless games. A single .exe with no external dependencies (no RTSS, no PresentMon).

## Usage

| Command | What it does |
|---|---|
| `open-perf-hud.exe` | Overlay. Asks for UAC only on systems that require admin to open the ETW session. |
| `open-perf-hud.exe --cli` | Console metrics, no UAC. FPS only if ETW starts; CPU/GPU/RAM always. |
| `open-perf-hud.exe --probe 10s` | Diagnostic tool: histogram of received ETW events. |

- Single instance: launching a second time asks **Retry** (close the running one and take over) or **Cancel** (keep it).
- The overlay is click-through: control it from the **tray icon** (click for the menu) or the configured **hotkey** (`CTRL+SHIFT+O` by default).
- It draws on the monitor of the game (it follows the foreground window).
- FPS = frames presented by the foreground process in the last second. On the desktop it shows `—`. Apps that present from a child process (Chromium/Electron browsers) may also show `—`; games present from their own process and count fine.

## Config

`config.json` next to the exe (created with defaults on the first menu change):

```json
{
  "corner": 1,
  "margin": "none",
  "opacity": 180,
  "fontSize": "sm",
  "orientation": "horizontal",
  "bgOpacity": 25,
  "textColor": "White-grey 2",
  "innerMargin": "none",
  "spacing": "xs",
  "offsetLeft": "none",
  "offsetRight": "none",
  "offsetTop": "none",
  "offsetBottom": "none",
  "metrics": {
    "name": false, "fps": true, "cpuPct": true, "gpuPct": true, "gpuTemp": true,
    "vramPct": true, "vramGB": true, "ramPct": true, "ramGB": true
  },
  "refreshMs": 500,
  "hotkey": "CTRL+SHIFT+O"
}
```

`corner`: 0 top-left · 1 top-right · 2 bottom-left · 3 bottom-right. `opacity`: 0–255 (text). `bgOpacity`: 0–100 (% of the black background; 0 = text only). `textColor`: `White|White-grey 1..3|Grey|Grey-dark 1..2|Grey-black|Black` — text grayscale (255/224/198/171/158/138/110/74/0); `Grey` and `Grey-dark 1` are Steam's greys (9e9e9e/8a8a8a). `gpuTemp` (`GPU 58°C`): via NVML from the NVIDIA driver, no admin; on AMD/Intel it shows `n/d`. CPU temperature: not available without a kernel driver (out of scope by design). `innerMargin`: `none|xs|sm|md|lg` (inner padding). `spacing`: `none|xs|sm|md|lg|xl` (0/2/5/8/12/20 px between lines; the step is based on font size, not the GDI cell, so it is properly compact; in horizontal layout it scales the `·` separator; `none` = no spaces at all). `fontSize`: `xs|sm|md|lg` (12/15/18/24 px). `margin`: `none|xs|sm|md|lg` (0/4/8/12/24 px). `offsetLeft|offsetRight|offsetTop|offsetBottom`: `none|2|4|8|10|12|14|16|20|24|30|40|50` px extra from each edge on top of `margin` (they push the overlay inwards from that edge; combinable). `orientation`: `vertical` (one metric per line) or `horizontal` (everything on one line). `metrics`: one boolean per metric; VRAM and RAM merge `%` and GB when both are on (`VRAM 35% 8.8/9.8 GB`). Consecutive items of the same kind do not repeat the label (`GPU 10% · 64°C | CPU 10%`); labels (CPU, GPU…) are bold. Everything is also in the tray menu (version title, Metrics section, Show/Hide, Position/Text size/Text color/Background, separator, Layout/Margin/Inner margin/Item spacing/Offset left-right-top-bottom, Exit).

## Known limitations

- **Exclusive fullscreen**: not supported (an overlay window cannot draw over an exclusive mode). Use borderless windowed. This is the same reason other overlays inject DLLs into the game; this app injects nothing.
- **GPU%**: NVML (NVIDIA) when available — the same number Steam and the NVIDIA panel show. Otherwise PDH "GPU Engine" counters, which read lower under load.
- **FPS**: captures Present events from DXGI, D3D9 and DxgKrnl (PresentHistory) — covers DXGI/D3D9 windowed and borderless. **Vulkan/OpenGL may not be counted on Windows 11**: some builds do not emit DxgKrnl's PresentHistory_Start (0xAB). If a Vulkan game shows `—`, run `--probe 10s` with it in the foreground to see which event IDs DxgKrnl emits.

## Development

Requires Go 1.26+ and [Task](https://taskfile.dev). `task build` (both exes), `task check` (gofmt/vet/test), `task smoke` (overlay for 6s + CLI for 4s), `task run`, `task cli`, `task probe -- 10s`, `task clean`, `task icon` (regenerates icon.ico + rsrc.syso from the icon.svg design; requires `go install github.com/akavel/rsrc@latest`).

## How it works

- **FPS**: real-time ETW session subscribed to Microsoft-Windows-DXGI, -D3D9 and -DxgKrnl, filtered by event ID to the present events that are actually emitted (DXGI Present_Stop / PresentMultiplaneOverlay_Stop, D3D9 Present_Stop, DxgKrnl PresentHistory_Start / PresentHistoryDetailed_Start). One-second sliding counter per PID; PID = process owning the foreground window. Pairs reported by both runtime and kernel are deduplicated within 2 ms.
- **CPU**: `GetSystemTimes` (kernel32). **RAM**: `GlobalMemoryStatusEx`. **GPU**: NVML utilization, falling back to PDH `\GPU Engine(*engtype_3D)\Utilization Percentage`.
- **VRAM**: used = PDH `\GPU Adapter Memory(*)\Dedicated Usage` filtered by the main adapter LUID; total = DXGI `EnumAdapters1`/`GetDesc1`. The DXGI desc is parsed by validation because two layouts exist in the wild (with and without the `Flags` field, verified empirically).
- **Overlay**: Win32 layered + topmost + click-through window, painted with GDI (Consolas), pushed with `UpdateLayeredWindow` and per-pixel alpha so text and background have independent opacity.
