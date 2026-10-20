package main

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---- Win32 constants (the minimum) ----

const (
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExTopmost     = 0x00000008
	wsExNoActivate  = 0x08000000
	wsExToolWindow  = 0x00000080
	wsPopup         = 0x80000000

	wmDestroy    = 0x0002
	wmPaint      = 0x000F
	wmEraseBkgnd = 0x0014
	wmTimer      = 0x0113
	wmHotkey     = 0x0312
	wmApp        = 0x8000
	wmNull       = 0x0000
	wmClose      = 0x0010
	wmRButtonUp  = 0x0205
	wmLButtonUp  = 0x0202

	trayMsg    = wmApp + 1
	idTimer    = 1
	idHotkey   = 1
	menuToggle = 1001
	menuQuit   = 1003
	idMetric   = 1500 // + index (metricLabels); the other bases are assigned by init() over menuOpts

	swHide           = 0
	swShowNoActivate = 4

	monitorDefaultToNearest = 2

	mbRetryCancel  = 0x0005 // MB_RETRYCANCEL
	mbIconQuestion = 0x0020
	idRetry        = 4

	ulwAlpha     = 0x2
	acSrcAlpha   = 0x1
	dibRGBColor  = 0x0
	srcCopy      = 0x00CC0020
	coloroncolor = 0x3 // STRETCH_DELETESCANS: shrink without the default
	// BLACKONWHITE AND of eliminated pixels, which darkens mixed colors/text

	smpW, smpH = 64, 8 // size of the Dynamic sampling DIB

	nimAdd         = 0
	nifMessage     = 0x1
	nifIcon        = 0x2
	nifTip         = 0x4
	idiApplication = 32512

	modAlt     = 0x0001
	modControl = 0x0002
	modShift   = 0x0004
	modWin     = 0x0008

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
	tpmNonotify    = 0x0080

	mfChecked   = 0x0008
	mfPopup     = 0x0010
	mfSeparator = 0x0800
	mfGrayed    = 0x0001
)

var (
	moduser32  = windows.NewLazySystemDLL("user32.dll")
	modgee32   = windows.NewLazySystemDLL("gdi32.dll")
	modshell32 = windows.NewLazySystemDLL("shell32.dll")

	procGetModuleHandleW    = modkernel32.NewProc("GetModuleHandleW")
	procRegisterClassExW    = moduser32.NewProc("RegisterClassExW")
	procCreateWindowExW     = moduser32.NewProc("CreateWindowExW")
	procDefWindowProcW      = moduser32.NewProc("DefWindowProcW")
	procGetMessageW         = moduser32.NewProc("GetMessageW")
	procTranslateMessage    = moduser32.NewProc("TranslateMessage")
	procDispatchMessageW    = moduser32.NewProc("DispatchMessageW")
	procPostQuitMessage     = moduser32.NewProc("PostQuitMessage")
	procDestroyWindow       = moduser32.NewProc("DestroyWindow")
	procShowWindow          = moduser32.NewProc("ShowWindow")
	procFindWindowW         = moduser32.NewProc("FindWindowW")
	procSetTimer            = moduser32.NewProc("SetTimer")
	procKillTimer           = moduser32.NewProc("KillTimer")
	procBeginPaint          = moduser32.NewProc("BeginPaint")
	procEndPaint            = moduser32.NewProc("EndPaint")
	procCreateFontW         = modgee32.NewProc("CreateFontW")
	procSelectObject        = modgee32.NewProc("SelectObject")
	procDeleteObject        = modgee32.NewProc("DeleteObject")
	procSetBkMode           = modgee32.NewProc("SetBkMode")
	procSetTextColor        = modgee32.NewProc("SetTextColor")
	procTextOutW            = modgee32.NewProc("TextOutW")
	procGetTextExtentPoint  = modgee32.NewProc("GetTextExtentPoint32W")
	procGetDC               = moduser32.NewProc("GetDC")
	procReleaseDC           = moduser32.NewProc("ReleaseDC")
	procFillRect            = moduser32.NewProc("FillRect")
	procCreateSolidBrush    = modgee32.NewProc("CreateSolidBrush")
	procCreateCompatDC      = modgee32.NewProc("CreateCompatibleDC")
	procCreateDIBSection    = modgee32.NewProc("CreateDIBSection")
	procStretchBlt          = modgee32.NewProc("StretchBlt")
	procSetStretchBltMode   = modgee32.NewProc("SetStretchBltMode")
	procUpdateLayeredWnd    = moduser32.NewProc("UpdateLayeredWindow")
	procShellNotifyIconW    = modshell32.NewProc("Shell_NotifyIconW")
	procLoadIconW           = moduser32.NewProc("LoadIconW")
	procCreatePopupMenu     = moduser32.NewProc("CreatePopupMenu")
	procAppendMenuW         = moduser32.NewProc("AppendMenuW")
	procTrackPopupMenu      = moduser32.NewProc("TrackPopupMenu")
	procDestroyMenu         = moduser32.NewProc("DestroyMenu")
	procSetForegroundWindow = moduser32.NewProc("SetForegroundWindow")
	procPostMessageW        = moduser32.NewProc("PostMessageW")
	procGetSystemMetrics    = moduser32.NewProc("GetSystemMetrics")
	procMonitorFromWindow   = moduser32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW     = moduser32.NewProc("GetMonitorInfoW")
	procRegisterHotKey      = moduser32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = moduser32.NewProc("UnregisterHotKey")
	procGetCursorPos        = moduser32.NewProc("GetCursorPos")

	procCreateMutexW = modkernel32.NewProc("CreateMutexW")
	procMessageBoxW  = moduser32.NewProc("MessageBoxW")
)

// ---- structs con layout C exacto (x64) ----

type rect struct{ Left, Top, Right, Bottom int32 }

type paintStruct struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     rect
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]uint8
	_           [4]uint8 // size 72 like PAINTSTRUCT in C
}

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HBrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type msg struct {
	HWnd    uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
	Private uint32
}

type monitorInfo struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
}

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

type sizeStruct struct{ Cx, Cy int32 }

// blendFunction = BLENDFUNCTION de winuser (4 bytes).
type blendFunction struct {
	BlendOp            uint8
	BlendFlags         uint8
	SourceConstantAlph uint8
	AlphaFormat        uint8
}

// bitmapInfoHeader = BITMAPINFOHEADER (40 bytes).
type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct{ Header bitmapInfoHeader }

// ---- overlay ----

type overlay struct {
	cfg     Config
	tracker *FPSTracker
	cpu     *CPUMeter
	gpu     *GPUMeter
	cleanup func()

	hwnd      uintptr
	hfont     uintptr
	hfontBold uintptr
	snap      Snapshot
	visible   bool
	w, h      int32
	lineH     int32

	memDC  uintptr // memory DC where the 32bpp DIB is painted
	memBmp uintptr
	bits   unsafe.Pointer // DIB pixels (premultiplied BGRA after applyAlpha)
	bw, bh int32

	smpDC   uintptr // sampling DC+DIB for Dynamic (StretchBlt of the background)
	smpBmp  uintptr
	smpBits unsafe.Pointer
	dynDark bool // current background is considered dark (Dynamic hysteresis)
}

func hInstance() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}

// grabInstanceMutex creates the overlay's single-instance mutex.
// existed=true if another process is already running it.
func grabInstanceMutex() (mu uintptr, existed bool) {
	name, _ := syscall.UTF16PtrFromString(`Local\OpenPerfHudOverlay`)
	mu, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	return mu, err == windows.ERROR_ALREADY_EXISTS
}

// askRestart asks whether to restart the already-running instance. true = restart.
func askRestart() bool {
	t, _ := syscall.UTF16PtrFromString("Open Perf HUD is already running.\n\nRetry: restart it\nCancel: keep the current one")
	c, _ := syscall.UTF16PtrFromString("Open Perf HUD")
	r1, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), mbRetryCancel|mbIconQuestion)
	return r1 == idRetry
}

// restartRunning asks the existing instance to close (WM_CLOSE); its mutex
// is freed when the process ends.
func restartRunning() bool {
	cname, _ := syscall.UTF16PtrFromString("OpenPerfHudWnd")
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cname)), 0)
	if hwnd == 0 {
		return false
	}
	procPostMessageW.Call(hwnd, wmClose, 0, 0)
	return true
}

// runOverlay creates window, tray and hotkey and runs the message loop. Blocks.
func runOverlay(cfg Config, tracker *FPSTracker, cpu *CPUMeter, gpu *GPUMeter, fpsCleanup func()) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ov := &overlay{cfg: cfg, tracker: tracker, cpu: cpu, gpu: gpu, cleanup: fpsCleanup, visible: true}
	ov.snap = collectSnapshot(tracker, cpu, gpu)

	className, _ := syscall.UTF16PtrFromString("OpenPerfHudWnd")
	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		LpfnWndProc:   syscall.NewCallback(ov.wndProc),
		HInstance:     hInstance(),
		LpszClassName: className,
	}
	if r1, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r1 == 0 {
		return fmt.Errorf("RegisterClassExW failed")
	}

	exStyle := uintptr(wsExLayered | wsExTransparent | wsExTopmost | wsExNoActivate | wsExToolWindow)
	title, _ := syscall.UTF16PtrFromString("OpenPerfHud")
	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(wsPopup),
		0, 0, 300, 120,
		0, 0, wc.HInstance, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed")
	}
	ov.hwnd = hwnd

	ov.memDC, _, _ = procCreateCompatDC.Call(0)
	ov.makeFont()
	ov.position()
	procShowWindow.Call(hwnd, swShowNoActivate)
	ov.addTray()

	if mods, vk, err := parseHotkey(cfg.Hotkey); err == nil {
		procRegisterHotKey.Call(hwnd, idHotkey, uintptr(mods), uintptr(vk))
	}
	procSetTimer.Call(hwnd, idTimer, uintptr(cfg.RefreshMs), 0)

	var m msg
	for {
		r1, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r1 == 0 || r1 == ^uintptr(0) { // WM_QUIT or error
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

func (ov *overlay) wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTimer:
		ov.snap = collectSnapshot(ov.tracker, ov.cpu, ov.gpu)
		ov.position()

	case wmEraseBkgnd:
		return 1 // we paint everything in WM_PAINT

	case wmPaint:
		var ps paintStruct
		procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps))) // real painting goes through UpdateLayeredWindow

	case trayMsg:
		if lParam == wmRButtonUp || lParam == wmLButtonUp {
			ov.showMenu()
		}

	case wmHotkey:
		ov.toggle()

	case wmDestroy:
		nid := notifyIconDataW{HWnd: hwnd, UID: 1}
		nid.CbSize = uint32(unsafe.Sizeof(nid))
		procShellNotifyIconW.Call(1, uintptr(unsafe.Pointer(&nid))) // NIM_DELETE
		procKillTimer.Call(hwnd, idTimer)
		procUnregisterHotKey.Call(hwnd, idHotkey)
		if ov.cleanup != nil {
			ov.cleanup()
		}
		procPostQuitMessage.Call(0)
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

// seg: a piece of a line; metric titles (CPU, GPU…) are bold.
type seg struct {
	s    string
	bold bool
}

func (ov *overlay) lines() [][]seg {
	s := ov.snap
	m := ov.cfg.Metrics
	type item struct {
		title, val string
	}
	var its []item

	if m.Name {
		its = append(its, item{val: s.NameTxt()})
	}
	if m.FPS {
		its = append(its, item{title: "FPS", val: s.FPSTxt()})
	}
	if m.CPUPct {
		its = append(its, item{title: "CPU", val: fmt.Sprintf("%.0f%%", s.CPU)})
	}
	if m.GPUPct {
		its = append(its, item{title: "GPU", val: s.GPUTxt()})
	}
	if m.GPUTemp {
		its = append(its, item{title: "GPU", val: s.TempTxt()})
	}
	if m.VRAMPct || m.VRAMGB {
		var parts []string
		if m.VRAMPct {
			if s.VRAMUsed >= 0 && s.VRAMTotal > 0 {
				parts = append(parts, fmt.Sprintf("%.0f%%", s.VRAMUsed/s.VRAMTotal*100))
			} else {
				parts = append(parts, "n/a")
			}
		}
		if m.VRAMGB {
			parts = append(parts, s.VRAMTxt())
		}
		its = append(its, item{title: "VRAM", val: strings.Join(parts, " ")})
	}
	if m.RAMPct || m.RAMGB {
		var parts []string
		if m.RAMPct {
			parts = append(parts, fmt.Sprintf("%.0f%%", float64(s.RAMLoad)))
		}
		if m.RAMGB {
			parts = append(parts, s.RAMTxt())
		}
		its = append(its, item{title: "RAM", val: strings.Join(parts, " ")})
	}

	// Grouping: consecutive items with the same prefix (CPU, GPU…) don't repeat
	// the word; in horizontal, within a group they separate with · and between
	// groups with |.
	prev := ""
	for i := range its {
		if its[i].title == prev {
			its[i].title = ""
		}
		if its[i].title != "" {
			prev = its[i].title
		}
	}
	segOf := func(it item) []seg {
		if it.title == "" {
			return []seg{{s: it.val}}
		}
		return []seg{{s: it.title, bold: true}, {s: " " + it.val}}
	}
	if ov.cfg.Orientation == "horizontal" {
		// px → spaces (~½ character): none fully tight, xs snug, xl wide
		g := spacingPx[ov.cfg.Spacing]
		n := 0
		if g > 0 {
			n = 1 + g/4
		}
		pad := strings.Repeat(" ", n)
		var out []seg
		for i, it := range its {
			if i > 0 {
				sep := "|"
				if it.title == "" { // same section as the previous one
					sep = "·"
				}
				out = append(out, seg{s: pad + sep + pad})
			}
			out = append(out, segOf(it)...)
		}
		return [][]seg{out}
	}
	var out [][]seg
	for _, it := range its {
		out = append(out, segOf(it))
	}
	return out
}

func (ov *overlay) makeFont() {
	if ov.hfont != 0 {
		procDeleteObject.Call(ov.hfont)
	}
	if ov.hfontBold != 0 {
		procDeleteObject.Call(ov.hfontBold)
	}
	face, _ := syscall.UTF16PtrFromString("Consolas")
	h := uintptr(-int32(fontPx[ov.cfg.FontSize]))
	fh, _, _ := procCreateFontW.Call(
		h, 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(face)))
	fb, _, _ := procCreateFontW.Call(
		h, 0, 0, 0, 700, 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(face)))
	ov.hfont, ov.hfontBold = fh, fb
}

// step: line height = font + gap (compact, not the full GDI cell).
func (ov *overlay) step() int32 {
	return int32(fontPx[ov.cfg.FontSize] + spacingPx[ov.cfg.Spacing])
}

// textExtent returns the max width/height of the current lines
// (per segment: bold titles measure differently).
func (ov *overlay) textExtent(lines [][]seg) (int32, int32) {
	hdc, _, _ := procGetDC.Call(ov.hwnd)
	var sz sizeStruct
	maxW := int32(0)
	h := int32(0)
	for _, l := range lines {
		w := int32(0)
		for _, g := range l {
			f := ov.hfont
			if g.bold {
				f = ov.hfontBold
			}
			old, _, _ := procSelectObject.Call(hdc, f)
			w16, _ := syscall.UTF16FromString(g.s)
			procGetTextExtentPoint.Call(hdc, uintptr(unsafe.Pointer(&w16[0])), uintptr(len(w16)-1), uintptr(unsafe.Pointer(&sz)))
			procSelectObject.Call(hdc, old)
			w += sz.Cx
			if sz.Cy > h {
				h = sz.Cy
			}
		}
		if w > maxW {
			maxW = w
		}
	}
	procReleaseDC.Call(ov.hwnd, hdc)
	return maxW, h
}

// position repositions/resizes on the focused process's monitor and repaints.
func (ov *overlay) position() {
	lines := ov.lines()
	w, lh := ov.textExtent(lines)
	ov.lineH = lh
	im := int32(marginPx[ov.cfg.InnerMargin])
	step := ov.step()
	w += 2 * im
	h := lh + step*int32(len(lines)-1) + 2*im

	fg := uintptr(windows.GetForegroundWindow())
	mon, _, _ := procMonitorFromWindow.Call(fg, monitorDefaultToNearest)

	var wa rect
	if mon != 0 {
		mi := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
		if r1, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r1 != 0 {
			wa = mi.RcWork
		}
	}
	if wa == (rect{}) { // primary screen fallback
		cx, _, _ := procGetSystemMetrics.Call(0)
		cy, _, _ := procGetSystemMetrics.Call(1)
		wa = rect{Right: int32(cx), Bottom: int32(cy)}
	}

	m := int32(marginPx[ov.cfg.Margin])
	var x, y int32
	switch ov.cfg.Corner {
	case 0:
		x, y = wa.Left+m, wa.Top+m
	case 1:
		x, y = wa.Right-m-w, wa.Top+m
	case 2:
		x, y = wa.Left+m, wa.Bottom-m-h
	default:
		x, y = wa.Right-m-w, wa.Bottom-m-h
	}
	// Offsets: extra px from each edge (offLeft/offTop push inwards from
	// left/top; offRight/offBottom from right/bottom).
	x += int32(offPx(ov.cfg.OffLeft) - offPx(ov.cfg.OffRight))
	y += int32(offPx(ov.cfg.OffTop) - offPx(ov.cfg.OffBottom))

	// always: margin/corner change x,y without changing the size
	ov.render(lines, x, y, w, h)
	ov.w, ov.h = w, h
}

// ensureSurface creates/resizes the 32bpp DIB where painting happens.
func (ov *overlay) ensureSurface(w, h int32) {
	if ov.memBmp != 0 && ov.bw == w && ov.bh == h {
		return
	}
	if ov.memBmp != 0 {
		procDeleteObject.Call(ov.memBmp)
	}
	bi := bitmapInfo{Header: bitmapInfoHeader{
		BiSize: 40, BiWidth: w, BiHeight: -h, BiPlanes: 1, BiBitCount: 32,
	}}
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(ov.memDC, uintptr(unsafe.Pointer(&bi)), dibRGBColor,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 {
		return
	}
	ov.memBmp, ov.bits, ov.bw, ov.bh = bmp, bits, w, h
	procSelectObject.Call(ov.memDC, bmp)
}

// applyAlpha: text with alpha textA and gray gray (255=pure white), black
// background with alpha bgA (premultiplied). GDI doesn't write the alpha
// channel; we derive it from pixel coverage.
func applyAlpha(px []byte, textA, bgA, gray int) {
	for i := 0; i < len(px); i += 4 {
		b, g, r := int(px[i]), int(px[i+1]), int(px[i+2])
		m := max(r, max(g, b))
		a1 := m * textA / 255
		px[i] = uint8(b * textA / 255 * gray / 255)
		px[i+1] = uint8(g * textA / 255 * gray / 255)
		px[i+2] = uint8(r * textA / 255 * gray / 255)
		px[i+3] = uint8(a1 + bgA*(255-a1)/255)
	}
}

// dynamicGray: gray for Dynamic text based on the background's average
// luminance. dark remembers whether the background is considered dark; the
// 108-148 dead band avoids flicker on contrast edges.
func dynamicGray(lum int, dark *bool) int {
	switch {
	case lum > 148:
		*dark = false // light background → dark text
	case lum < 108:
		*dark = true // dark background → light text
	}
	if *dark {
		return 255 // White
	}
	return 74 // Grey-black
}

// ensureSample creates, once, the small DIB used to sample the background.
func (ov *overlay) ensureSample() {
	if ov.smpDC == 0 {
		ov.smpDC, _, _ = procCreateCompatDC.Call(0)
	}
	if ov.smpBmp != 0 {
		return
	}
	bi := bitmapInfo{Header: bitmapInfoHeader{
		BiSize: 40, BiWidth: smpW, BiHeight: -smpH, BiPlanes: 1, BiBitCount: 32,
	}}
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(ov.smpDC, uintptr(unsafe.Pointer(&bi)), dibRGBColor,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	ov.smpBmp, ov.smpBits = bmp, bits
	if bmp != 0 {
		procSelectObject.Call(ov.smpDC, bmp)
		// default BLACKONWHITE ANDs eliminated scanlines together, biasing
		// mixed-color and text-covered regions dark and breaking the switch
		// to dark text on light backgrounds
		procSetStretchBltMode.Call(ov.smpDC, coloroncolor)
	}
}

// avgLuma: average Rec.601 luma of a BGRA buffer (the 114 on blue matters:
// without it, blue-heavy backgrounds read as near-black).
func avgLuma(px []byte) int {
	lum, n := 0, 0
	for i := 0; i+3 < len(px); i += 4 {
		lum += (int(px[i+2])*299 + int(px[i+1])*587 + int(px[i])*114) / 1000
		n++
	}
	if n == 0 {
		return 0
	}
	return lum / n
}

// sampleGray samples the screen region under the overlay and returns the
// Dynamic gray. Falls back to 255 (light text) if capture is unavailable.
func (ov *overlay) sampleGray(x, y, w, h int32) int {
	ov.ensureSample()
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return 255
	}
	defer procReleaseDC.Call(0, hdc)
	if ov.smpBmp == 0 {
		return 255
	}
	// ponytail: the capture includes the overlay itself (layered window); its
	// black fill dims the sample by exactly (100-bg)%, so we undo that below;
	// text pixels are sparse and hysteresis absorbs them — no masking tricks.
	procStretchBlt.Call(ov.smpDC, 0, 0, smpW, smpH, hdc,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), srcCopy)
	avg := avgLuma(unsafe.Slice((*byte)(ov.smpBits), smpW*smpH*4))
	if bg := ov.cfg.BgOpacity; bg < 90 {
		avg = avg * 100 / (100 - bg) // undo the fill's dimming; >=90 is an opaque box: dark, white text is fine
	}
	return dynamicGray(avg, &ov.dynDark)
}

// render paints the lines on the DIB and presents it with UpdateLayeredWindow
// (moves, resizes and paints in a single call, per-pixel alpha).
func (ov *overlay) render(lines [][]seg, x, y, w, h int32) {
	ov.ensureSurface(w, h)
	if ov.memBmp == 0 {
		return
	}

	old, _, _ := procSelectObject.Call(ov.memDC, ov.hfont)
	procSetBkMode.Call(ov.memDC, 1) // TRANSPARENT
	procSetTextColor.Call(ov.memDC, 0x00FFFFFF)

	brush, _, _ := procCreateSolidBrush.Call(0x00000000)
	var full rect
	full.Right, full.Bottom = w, h
	procFillRect.Call(ov.memDC, uintptr(unsafe.Pointer(&full)), brush)
	procDeleteObject.Call(brush)

	im := int32(marginPx[ov.cfg.InnerMargin])
	step := ov.step()
	var cy int32
	var sz sizeStruct
	for _, l := range lines {
		var cx int32
		for _, g := range l {
			f := ov.hfont
			if g.bold {
				f = ov.hfontBold
			}
			procSelectObject.Call(ov.memDC, f)
			w16, _ := syscall.UTF16FromString(g.s)
			procTextOutW.Call(ov.memDC, uintptr(im+cx), uintptr(im+cy),
				uintptr(unsafe.Pointer(&w16[0])), uintptr(len(w16)-1))
			procGetTextExtentPoint.Call(ov.memDC, uintptr(unsafe.Pointer(&w16[0])), uintptr(len(w16)-1), uintptr(unsafe.Pointer(&sz)))
			cx += sz.Cx
		}
		cy += step
	}
	procSelectObject.Call(ov.memDC, old)

	px := unsafe.Slice((*byte)(ov.bits), int(w)*int(h)*4)
	gray := colorGray[ov.cfg.TextColor]
	if ov.cfg.TextColor == "Dynamic" {
		gray = ov.sampleGray(x, y, w, h)
	}
	applyAlpha(px, int(ov.cfg.Opacity), int(ov.cfg.BgOpacity)*255/100, gray)

	blend := blendFunction{BlendOp: 1 /*AC_SRC_OVER*/, SourceConstantAlph: 255, AlphaFormat: acSrcAlpha}
	ptDst := sizeStruct{Cx: x, Cy: y}
	dstSz := sizeStruct{Cx: w, Cy: h}
	ptSrc := sizeStruct{}
	procUpdateLayeredWnd.Call(ov.hwnd, 0,
		uintptr(unsafe.Pointer(&ptDst)), uintptr(unsafe.Pointer(&dstSz)),
		ov.memDC, uintptr(unsafe.Pointer(&ptSrc)), 0,
		uintptr(unsafe.Pointer(&blend)), ulwAlpha)
}

func (ov *overlay) addTray() {
	nid := notifyIconDataW{
		HWnd:             ov.hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: trayMsg,
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	// icon embedded in the exe (rsrc.syso, ID 1); stock fallback if missing (go run)
	icon := loadAppIcon()
	nid.HIcon = icon
	copy(nid.SzTip[:], syscall.StringToUTF16("Open Perf HUD"))
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

var cornerNames = [4]string{"Top-left", "Top-right", "Bottom-left", "Bottom-right"}

// loadAppIcon loads the exe's icon (resource ID 1, embedded via rsrc.syso).
func loadAppIcon() uintptr {
	if hmod, _, _ := procGetModuleHandleW.Call(0); hmod != 0 {
		if icon, _, _ := procLoadIconW.Call(hmod, 1); icon != 0 {
			return icon
		}
	}
	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	return icon
}

// appendOptions adds a submenu with one checked option.
func appendOptions(menu uintptr, label string, opts []string, cur string, base int) {
	sub, _, _ := procCreatePopupMenu.Call()
	for i, o := range opts {
		t, _ := syscall.UTF16PtrFromString(o)
		var flags uintptr
		if o == cur {
			flags = mfChecked
		}
		procAppendMenuW.Call(sub, flags, uintptr(base+i), uintptr(unsafe.Pointer(t)))
	}
	t, _ := syscall.UTF16PtrFromString(label)
	procAppendMenuW.Call(menu, mfPopup, sub, uintptr(unsafe.Pointer(t)))
}

// menuOpt: exclusive-choice tray submenu. This table feeds buildMenu
// (options and check mark from cur()) and handleMenu (set(i) applies the
// chosen index); init() assigns non-overlapping id bases starting at 2000.
type menuOpt struct {
	label string
	opts  []string
	cur   func(*overlay) string
	set   func(*overlay, int)
	base  int
	sep   bool // separator before this entry
}

var menuOpts = []menuOpt{
	{"Position", cornerNames[:], func(o *overlay) string { return cornerNames[o.cfg.Corner] }, func(o *overlay, i int) { o.cfg.Corner = i }, 0, false},
	{"Text size", fontSizes, func(o *overlay) string { return o.cfg.FontSize }, func(o *overlay, i int) { o.cfg.FontSize = fontSizes[i]; o.makeFont() }, 0, false},
	{"Text color", colorOpts, func(o *overlay) string { return o.cfg.TextColor }, func(o *overlay, i int) { o.cfg.TextColor = colorOpts[i] }, 0, false},
	{"Background", bgLabels, func(o *overlay) string { return fmt.Sprintf("%d%%", o.cfg.BgOpacity) }, func(o *overlay, i int) { o.cfg.BgOpacity = bgPcts[i] }, 0, false},
	{"Layout", []string{orientations[0].name, orientations[1].name},
		func(o *overlay) string { return orientationOf(o.cfg.Orientation).name },
		func(o *overlay, i int) { o.cfg.Orientation = orientations[i].val }, 0, true},
	{"Margin", margins, func(o *overlay) string { return o.cfg.Margin }, func(o *overlay, i int) { o.cfg.Margin = margins[i] }, 0, false},
	{"Inner margin", margins, func(o *overlay) string { return o.cfg.InnerMargin }, func(o *overlay, i int) { o.cfg.InnerMargin = margins[i] }, 0, false},
	{"Item spacing", spacings, func(o *overlay) string { return o.cfg.Spacing }, func(o *overlay, i int) { o.cfg.Spacing = spacings[i] }, 0, false},
	{"Offset left", offOpts, func(o *overlay) string { return o.cfg.OffLeft }, func(o *overlay, i int) { o.cfg.OffLeft = offOpts[i] }, 0, false},
	{"Offset right", offOpts, func(o *overlay) string { return o.cfg.OffRight }, func(o *overlay, i int) { o.cfg.OffRight = offOpts[i] }, 0, false},
	{"Offset top", offOpts, func(o *overlay) string { return o.cfg.OffTop }, func(o *overlay, i int) { o.cfg.OffTop = offOpts[i] }, 0, false},
	{"Offset bottom", offOpts, func(o *overlay) string { return o.cfg.OffBottom }, func(o *overlay, i int) { o.cfg.OffBottom = offOpts[i] }, 0, false},
}

func init() {
	base := 2000 // > menuToggle/menuQuit/idMetric+len(metricLabels)
	for i := range menuOpts {
		menuOpts[i].base = base
		base += len(menuOpts[i].opts)
	}
}

func (ov *overlay) buildMenu() uintptr {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0
	}
	add := func(text string, flags, id uintptr) {
		t, _ := syscall.UTF16PtrFromString(text)
		procAppendMenuW.Call(menu, flags, id, uintptr(unsafe.Pointer(t)))
	}
	sep := func() { procAppendMenuW.Call(menu, mfSeparator, 0, 0) }

	add("Open Perf HUD v"+version, mfGrayed, 0) // title, not clickable
	sep()

	// Metrics: bool toggles, own section at the top
	sub, _, _ := procCreatePopupMenu.Call()
	ptrs := ov.cfg.metricPtrs()
	for i, label := range metricLabels {
		t, _ := syscall.UTF16PtrFromString(label)
		var flags uintptr
		if *ptrs[i] {
			flags = mfChecked
		}
		procAppendMenuW.Call(sub, flags, uintptr(idMetric+i), uintptr(unsafe.Pointer(t)))
	}
	tm, _ := syscall.UTF16PtrFromString("Metrics")
	procAppendMenuW.Call(menu, mfPopup, sub, uintptr(unsafe.Pointer(tm)))
	sep()

	toggleTxt := "Hide overlay"
	if !ov.visible {
		toggleTxt = "Show overlay"
	}
	add(toggleTxt, 0, menuToggle)
	for _, m := range menuOpts {
		if m.sep {
			sep() // — fine tuning —
		}
		appendOptions(menu, m.label, m.opts, m.cur(ov), m.base)
	}
	sep()
	add("Exit", 0, menuQuit)
	return menu
}

// trackMenu shows the menu at the cursor and returns the chosen command (0 = dismissed).
func (ov *overlay) trackMenu(menu uintptr) uintptr {
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(ov.hwnd) // trick so the menu closes when clicking outside
	cmd, _, _ := procTrackPopupMenu.Call(
		menu, tpmRightButton|tpmReturnCmd|tpmNonotify,
		uintptr(pt.X), uintptr(pt.Y), 0, ov.hwnd, 0)
	procPostMessageW.Call(ov.hwnd, wmNull, 0, 0)
	return cmd
}

// showMenu reopens the menu after each selection: several changes without re-clicking the tray.
func (ov *overlay) showMenu() {
	for {
		menu := ov.buildMenu()
		if menu == 0 {
			return
		}
		cmd := ov.trackMenu(menu)
		procDestroyMenu.Call(menu)
		if cmd == 0 { // Esc or click outside
			return
		}
		ov.handleMenu(cmd)
		if cmd == menuQuit || cmd == menuToggle {
			return
		}
	}
}

func (ov *overlay) handleMenu(cmd uintptr) {
	switch {
	case cmd == menuToggle:
		ov.toggle()
	case cmd == menuQuit:
		procDestroyWindow.Call(ov.hwnd)
	case cmd >= idMetric && cmd < idMetric+uintptr(len(metricLabels)):
		p := ov.cfg.metricPtrs()[cmd-idMetric]
		*p = !*p
		ov.cfgChanged()
	default:
		for _, m := range menuOpts {
			if cmd >= uintptr(m.base) && cmd < uintptr(m.base)+uintptr(len(m.opts)) {
				m.set(ov, int(cmd)-m.base)
				ov.cfgChanged()
				return
			}
		}
	}
}

// cfgChanged applies the config change: saves, repositions (and repaints via ULW).
func (ov *overlay) cfgChanged() {
	SaveConfig(ov.cfg)
	ov.position()
}

func (ov *overlay) toggle() {
	ov.visible = !ov.visible
	if ov.visible {
		procShowWindow.Call(ov.hwnd, swShowNoActivate)
	} else {
		procShowWindow.Call(ov.hwnd, swHide)
	}
}

// parseHotkey: "CTRL+SHIFT+O" → mods, vk.
func parseHotkey(s string) (mods, vk uint32, err error) {
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(s)), "+")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid hotkey: %q", s)
	}
	for i, p := range parts {
		switch strings.TrimSpace(p) {
		case "CTRL", "CONTROL":
			mods |= modControl
		case "SHIFT":
			mods |= modShift
		case "ALT":
			mods |= modAlt
		case "WIN":
			mods |= modWin
		default:
			if i != len(parts)-1 {
				return 0, 0, fmt.Errorf("unknown modifier: %q", p)
			}
			r := []rune(strings.TrimSpace(p))
			if len(r) != 1 || !unicode.IsLetter(r[0]) && !unicode.IsDigit(r[0]) {
				return 0, 0, fmt.Errorf("invalid key: %q", p)
			}
			vk = uint32(r[0]) // letters/digits: VK == ASCII
		}
	}
	return mods, vk, nil
}
