# Win32 settings dialog (Route A) + config changes

Native GUI settings window (programmatic Win32 controls, zero dependencies) replacing the tray icon submenus, with three config changes: Background in steps of 5, free numeric offsets 0-9999, and a "Dynamic" text color option (alert red).

## Steps

- [x] config.go — Background: 0..100 selector in steps of 5, snap in LoadConfig
- [x] config.go — Offsets: string → int (0-9999) with JSON migration from the old format
- [x] config.go — TextColor "Dynamic": validation, forces BgOpacity=0, base gray White-grey 2
- [x] overlay.go — alert red in every text color, tinted per gray by alertRed() + textGray()
- [x] settings.go — Win32 dialog: own window, combos, checks, offset EDITs, Close, live preview
- [x] overlay.go — tray reduced to Settings… / Show-Hide overlay / Exit; remove submenu machinery
- [x] pure-logic tests + verification GOOS=windows build, vet, gofmt

## Detail

### 1. config.go — Background (no breaking change)

- `bgPcts` goes from 6 fixed values to a generated series `0, 5, 10 … 100` (21 entries); `bgLabels` derived as today.
- `LoadConfig`: snap to the nearest multiple of 5 (`(bg+2)/5*5`, cap 100). An old `config.json` with `33` → `35`. Field stays `int`.

### 2. config.go — Offsets to int (breaking change in config.json)

- `OffLeft/OffRight/OffTop/OffBottom`: `string` (`"none"`, `"16"`) → `int`, 0 = none, range 0-9999.
- `offset int` type with a custom `UnmarshalJSON` accepting the old format (`"none"` → 0, `"36"` → 36) and the new one (number). `SaveConfig` writes numbers: existing `config.json` files keep working.
- `offOpts` and `offPx()` die; `position()` uses the ints directly. Clamp 0-9999 in LoadConfig.

### 3. config.go — TextColor "Dynamic"

- Add `"Dynamic"` to `colorOpts`. `fixStr` validation switches to `colorOpts` membership (not `colorGray`, which doesn't contain Dynamic).
- `textGray()`: 198 (White-grey 2) if Dynamic, `colorGray[TextColor]` otherwise. Dynamic needs a fixed base gray for normal text: **White-grey 2**.
- `LoadConfig`: if `TextColor == "Dynamic"` → force `BgOpacity = 0` (consistent with hand-edited configs too).

### 4. overlay.go — alert-red gating

- ~~Today red fires always (commit 6523ce5). With this plan: in `lines()`, `alert` is only set if `cfg.TextColor == "Dynamic"`; picking a concrete gray = pure text, no red.~~
- **Final decision (user)**: red fires with EVERY text color; `alertRed(gray)` tints it per gray for contrast (pure red at the dark end, clamped at the light end; with `Dynamic` it adapts to the sampled gray).
- `render`/`applyAlpha` use `cfg.textGray()` as the base gray.

### 5. settings.go — Win32 dialog (new, ~300-350 lines)

- Own window class + wndProc **on the same thread**: the existing `GetMessage` (hwnd=NULL) already serves it and the overlay's `WM_TIMER` keeps firing → **live preview** while adjusting.
- Normal window (`WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX`, ~410×560), not popup.
- `BS_GROUPBOX` groups:
  - **Layout**: Position, Text size, Text color, Background, Layout (COMBOBOX `CBS_DROPDOWNLIST`)
  - **Spacing**: Margin, Inner margin, Item spacing (COMBOBOX)
  - **Offsets**: 4 EDIT with `ES_NUMBER` (filters non-digits but not range: own 0-9999 clamp on `EN_CHANGE`; field rewritten on `EN_KILLFOCUS` if clamped)
  - **Metrics**: 9 `BS_AUTOCHECKBOX` (reuses `metricLabels`/`metricPtrs`)
  - **Close** button
- Controls fed from a successor table of `menuOpts`, adapted to `*Config` (label / kind / id / opts / cur / set): single source for combos and state.
- Every change → clamp → `SaveConfig` → `ov.position()`.
- Text color = Dynamic → Background `CB_SETCURSEL(0)` + `EnableWindow(FALSE)`; switching back to a gray re-enables it.
- Opening from tray: if already open, `SetForegroundWindow`.
- New Win32 constants: `wmCommand` 0x0111, `CBN_SELCHANGE` 1, `BN_CLICKED` 0, `EN_CHANGE` 0x0300, `EN_KILLFOCUS` 0x0200, `CB_ADDSTRING`/`CB_SETCURSEL`/`CB_GETCURSEL`, `BM_GETCHECK`/`BM_SETCHECK`, `ES_NUMBER`, `WS_CHILD|WS_VISIBLE|WS_TABSTOP`.

### 6. overlay.go — tray

- `buildMenu` → 3 entries: **Settings…**, **Show/Hide overlay**, **Exit**; `handleMenu` → 3 ids.
- Delete: `menuOpts`, `appendOptions`, `init()` bases, `showMenu`/`trackMenu` loop, `idMetric` base.
- Hotkey and `refreshMs` remain config.json-only (as until now).

### 7. Tests + verification

- Pure-logic tests (run on macOS): offset migration (old string, number, `"none"`, out of range), BgOpacity snap, Dynamic → BgOpacity 0, `textGray()`, 0-9999 clamp.
- `GOOS=windows GOARCH=amd64 go build ./...` + `go vet ./...` + `gofmt -l .` + `go test ./...`.
- The Win32 part of the dialog is only verifiable on real Windows.

## Decisions

- **Route A** (programmatic controls) over Fyne (cgo + OpenGL, +30 MB exe, second GL runtime in a 3 MB overlay) and over `.rc` template + `DialogBoxParam` (needs windres/go-winres: `rsrc` doesn't support dialogs; DLUs adjusted blind).
- Dynamic uses **White-grey 2** as the base gray for normal text.
- Tray stays as the entry point: Settings… / Show-Hide / Exit.
- Build unchanged: zero new dependencies, cross-compiles the same from macOS.
