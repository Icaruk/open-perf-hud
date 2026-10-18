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

// ---- constantes Win32 (lo mínimo) ----

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

	trayMsg     = wmApp + 1
	idTimer     = 1
	idHotkey    = 1
	menuToggle  = 1001
	menuQuit    = 1003
	idCorner    = 1100 // + índice
	idFont      = 1200 // + índice
	idMargin    = 1300 // + índice
	idOrient    = 1400 // + índice
	idMetric    = 1500 // + índice (metricLabels)
	idBg        = 1600 // + índice (bgPcts)
	idInner     = 1700 // + índice (margins, reutilizado como inner margin)
	idSpacing   = 1800 // + índice (spacings)
	idColor     = 1900 // + índice (colorOpts)
	idOffLeft   = 2000 // + índice (offOpts)
	idOffRight  = 2100
	idOffTop    = 2200
	idOffBottom = 2300

	swHide           = 0
	swShowNoActivate = 4

	monitorDefaultToNearest = 2

	mbRetryCancel  = 0x0005 // MB_RETRYCANCEL
	mbIconQuestion = 0x0020
	idRetry        = 4

	ulwAlpha    = 0x2
	acSrcAlpha  = 0x1
	dibRGBColor = 0x0

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
	_           [4]uint8 // tamaño 72 como PAINTSTRUCT en C
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

	memDC  uintptr // DC en memoria donde se pinta el DIB 32bpp
	memBmp uintptr
	bits   unsafe.Pointer // pixels del DIB (BGRA premultiplicado tras applyAlpha)
	bw, bh int32
}

func hInstance() uintptr {
	h, _, _ := procGetModuleHandleW.Call(0)
	return h
}

// grabInstanceMutex crea el mutex de instancia única del overlay.
// existed=true si ya hay otro proceso ejecutándolo.
func grabInstanceMutex() (mu uintptr, existed bool) {
	name, _ := syscall.UTF16PtrFromString(`Local\OpenPerfHudOverlay`)
	mu, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	return mu, err == windows.ERROR_ALREADY_EXISTS
}

// askRestart pregunta si reiniciar la instancia que ya corre. true = reiniciar.
func askRestart() bool {
	t, _ := syscall.UTF16PtrFromString("Open Perf HUD is already running.\n\nRetry: restart it\nCancel: keep the current one")
	c, _ := syscall.UTF16PtrFromString("Open Perf HUD")
	r1, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), mbRetryCancel|mbIconQuestion)
	return r1 == idRetry
}

// restartRunning pide a la instancia existente que cierre (WM_CLOSE); su mutex
// queda libre al terminar el proceso.
func restartRunning() bool {
	cname, _ := syscall.UTF16PtrFromString("OpenPerfHudWnd")
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cname)), 0)
	if hwnd == 0 {
		return false
	}
	procPostMessageW.Call(hwnd, wmClose, 0, 0)
	return true
}

// runOverlay crea ventana, bandeja y hotkey y ejecuta el message loop. Bloquea.
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
		if r1 == 0 || r1 == ^uintptr(0) { // WM_QUIT o error
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
		return 1 // pintamos todo en WM_PAINT

	case wmPaint:
		var ps paintStruct
		procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps))) // el pintado real va por UpdateLayeredWindow

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

// seg: trozo de línea; los títulos de métrica (CPU, GPU…) van en negrita.
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
		name := "—"
		if s.ProcName != "" {
			name = s.ProcName
		}
		its = append(its, item{val: name})
	}
	if m.FPS {
		fps := "  —"
		if s.ProcName != "" && s.FPS > 0 {
			fps = fmt.Sprintf("%3d", s.FPS)
		}
		its = append(its, item{title: "FPS", val: fps})
	}
	if m.CPUPct {
		its = append(its, item{title: "CPU", val: fmt.Sprintf("%.0f%%", s.CPU)})
	}
	if m.GPUPct {
		gpu := "n/d"
		if s.GPU >= 0 {
			gpu = fmt.Sprintf("%.0f%%", s.GPU)
		}
		its = append(its, item{title: "GPU", val: gpu})
	}
	if m.GPUTemp {
		t := "n/d"
		if s.GPUTemp >= 0 {
			t = fmt.Sprintf("%.0f°C", s.GPUTemp)
		}
		its = append(its, item{title: "GPU", val: t})
	}
	if m.VRAMPct || m.VRAMGB {
		var parts []string
		if m.VRAMPct {
			if s.VRAMUsed >= 0 && s.VRAMTotal > 0 {
				parts = append(parts, fmt.Sprintf("%.0f%%", s.VRAMUsed/s.VRAMTotal*100))
			} else {
				parts = append(parts, "n/d")
			}
		}
		if m.VRAMGB {
			switch {
			case s.VRAMUsed >= 0 && s.VRAMTotal > 0:
				parts = append(parts, fmt.Sprintf("%.1f/%.1f GB", s.VRAMUsed, s.VRAMTotal))
			case s.VRAMUsed >= 0:
				parts = append(parts, fmt.Sprintf("%.1f GB", s.VRAMUsed))
			default:
				parts = append(parts, "n/d")
			}
		}
		its = append(its, item{title: "VRAM", val: strings.Join(parts, " ")})
	}
	if m.RAMPct || m.RAMGB {
		var parts []string
		if m.RAMPct {
			parts = append(parts, fmt.Sprintf("%.0f%%", float64(s.RAMLoad)))
		}
		if m.RAMGB {
			parts = append(parts, fmt.Sprintf("%.1f/%.1f GB", s.RAMUsed, s.RAMTotal))
		}
		its = append(its, item{title: "RAM", val: strings.Join(parts, " ")})
	}

	// Agrupación: ítems consecutivos del mismo prefijo (CPU, GPU…) no repiten la
	// palabra; en horizontal, dentro del grupo se separa con · y entre grupos con |.
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
		// px → espacios (~½ carácter): none pegado del todo, xs ajustado, xl amplio
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
				if it.title == "" { // misma sección que el anterior
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

// step: alto de línea = fuente + separación (compacto, no la celda GDI completa).
func (ov *overlay) step() int32 {
	return int32(fontPx[ov.cfg.FontSize] + spacingPx[ov.cfg.Spacing])
}

// textExtent devuelve el ancho/altura máximos de las líneas actuales
// (por segmento: títulos en negrita miden distinto).
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

// position reposiciona/redimensiona en el monitor del proceso en foco y repinta.
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
	if wa == (rect{}) { // fallback pantalla primaria
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
	// Offsets: px extra desde cada borde (offLeft/offTop empujan hacia dentro
	// desde izquierda/arriba; offRight/offBottom desde derecha/abajo).
	x += int32(offPx(ov.cfg.OffLeft) - offPx(ov.cfg.OffRight))
	y += int32(offPx(ov.cfg.OffTop) - offPx(ov.cfg.OffBottom))

	// siempre: el margen/corner cambian x,y sin cambiar el tamaño
	ov.render(lines, x, y, w, h)
	ov.w, ov.h = w, h
}

// ensureSurface crea/redimensiona el DIB 32bpp donde se pinta.
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

// applyAlpha: texto con alpha textA y gris gray (255=blanco puro), fondo
// negro con alpha bgA (premultiplicado). GDI no escribe el canal alpha; lo
// derivamos de la cobertura del píxel.
func applyAlpha(px []byte, textA, bgA, gray int) {
	for i := 0; i < len(px); i += 4 {
		b, g, r := int(px[i]), int(px[i+1]), int(px[i+2])
		m := b
		if g > m {
			m = g
		}
		if r > m {
			m = r
		}
		a1 := m * textA / 255
		px[i] = uint8(b * textA / 255 * gray / 255)
		px[i+1] = uint8(g * textA / 255 * gray / 255)
		px[i+2] = uint8(r * textA / 255 * gray / 255)
		px[i+3] = uint8(a1 + bgA*(255-a1)/255)
	}
}

// render pinta las líneas en el DIB y lo presenta con UpdateLayeredWindow
// (mueve, redimensiona y pinta en una sola llamada, alpha por píxel).
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
	applyAlpha(px, int(ov.cfg.Opacity), int(ov.cfg.BgOpacity)*255/100, colorGray[ov.cfg.TextColor])

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
	// icono embebido en el exe (rsrc.syso, ID 1); fallback stock si falta (go run)
	icon := loadAppIcon()
	nid.HIcon = icon
	copy(nid.SzTip[:], syscall.StringToUTF16("Open Perf HUD"))
	procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
}

var cornerNames = [4]string{"Top-left", "Top-right", "Bottom-left", "Bottom-right"}

// loadAppIcon carga el icono del exe (recurso ID 1, embebido via rsrc.syso).
func loadAppIcon() uintptr {
	if hmod, _, _ := procGetModuleHandleW.Call(0); hmod != 0 {
		if icon, _, _ := procLoadIconW.Call(hmod, 1); icon != 0 {
			return icon
		}
	}
	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	return icon
}

// appendOptions añade un submenú con una opción marcada.
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

	add("Open Perf HUD v"+version, mfGrayed, 0) // título, no clicable
	sep()

	// Metrics: sección propia arriba del todo
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
	appendOptions(menu, "Position", cornerNames[:], cornerNames[ov.cfg.Corner], idCorner)
	appendOptions(menu, "Text size", fontSizes, ov.cfg.FontSize, idFont)
	appendOptions(menu, "Text color", colorOpts, ov.cfg.TextColor, idColor)
	bgLabels := make([]string, len(bgPcts))
	for i, p := range bgPcts {
		bgLabels[i] = fmt.Sprintf("%d%%", p)
	}
	appendOptions(menu, "Background", bgLabels, fmt.Sprintf("%d%%", ov.cfg.BgOpacity), idBg)
	sep() // — ajustes finos —
	curOrient := orientNames[0]
	if ov.cfg.Orientation == "horizontal" {
		curOrient = orientNames[1]
	}
	appendOptions(menu, "Layout", orientNames[:], curOrient, idOrient)
	appendOptions(menu, "Margin", margins, ov.cfg.Margin, idMargin)
	appendOptions(menu, "Inner margin", margins, ov.cfg.InnerMargin, idInner)
	appendOptions(menu, "Item spacing", spacings, ov.cfg.Spacing, idSpacing)
	appendOptions(menu, "Offset left", offOpts, ov.cfg.OffLeft, idOffLeft)
	appendOptions(menu, "Offset right", offOpts, ov.cfg.OffRight, idOffRight)
	appendOptions(menu, "Offset top", offOpts, ov.cfg.OffTop, idOffTop)
	appendOptions(menu, "Offset bottom", offOpts, ov.cfg.OffBottom, idOffBottom)
	sep()
	add("Exit", 0, menuQuit)
	return menu
}

// trackMenu muestra el menú en el cursor y devuelve el comando elegido (0 = cerrar).
func (ov *overlay) trackMenu(menu uintptr) uintptr {
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(ov.hwnd) // truco para que el menú se cierre al clicar fuera
	cmd, _, _ := procTrackPopupMenu.Call(
		menu, tpmRightButton|tpmReturnCmd|tpmNonotify,
		uintptr(pt.X), uintptr(pt.Y), 0, ov.hwnd, 0)
	procPostMessageW.Call(ov.hwnd, wmNull, 0, 0)
	return cmd
}

// showMenu reabre el menú tras cada selección: varios cambios sin re-clicar la bandeja.
func (ov *overlay) showMenu() {
	for {
		menu := ov.buildMenu()
		if menu == 0 {
			return
		}
		cmd := ov.trackMenu(menu)
		procDestroyMenu.Call(menu)
		if cmd == 0 { // Esc o clic fuera
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
	case cmd >= idCorner && cmd < idCorner+4:
		ov.cfg.Corner = int(cmd - idCorner)
		ov.cfgChanged()
	case cmd >= idFont && cmd < idFont+uintptr(len(fontSizes)):
		ov.cfg.FontSize = fontSizes[cmd-idFont]
		ov.makeFont()
		ov.cfgChanged()
	case cmd >= idMargin && cmd < idMargin+uintptr(len(margins)):
		ov.cfg.Margin = margins[cmd-idMargin]
		ov.cfgChanged()
	case cmd >= idOrient && cmd < idOrient+uintptr(len(orientations)):
		ov.cfg.Orientation = orientations[cmd-idOrient]
		ov.cfgChanged()
	case cmd >= idMetric && cmd < idMetric+uintptr(len(metricLabels)):
		p := ov.cfg.metricPtrs()[cmd-idMetric]
		*p = !*p
		ov.cfgChanged()
	case cmd >= idBg && cmd < idBg+uintptr(len(bgPcts)):
		ov.cfg.BgOpacity = bgPcts[cmd-idBg]
		ov.cfgChanged()
	case cmd >= idInner && cmd < idInner+uintptr(len(margins)):
		ov.cfg.InnerMargin = margins[cmd-idInner]
		ov.cfgChanged()
	case cmd >= idSpacing && cmd < idSpacing+uintptr(len(spacings)):
		ov.cfg.Spacing = spacings[cmd-idSpacing]
		ov.cfgChanged()
	case cmd >= idColor && cmd < idColor+uintptr(len(colorOpts)):
		ov.cfg.TextColor = colorOpts[cmd-idColor]
		ov.cfgChanged()
	case cmd >= idOffLeft && cmd < idOffLeft+uintptr(len(offOpts)):
		ov.cfg.OffLeft = offOpts[cmd-idOffLeft]
		ov.cfgChanged()
	case cmd >= idOffRight && cmd < idOffRight+uintptr(len(offOpts)):
		ov.cfg.OffRight = offOpts[cmd-idOffRight]
		ov.cfgChanged()
	case cmd >= idOffTop && cmd < idOffTop+uintptr(len(offOpts)):
		ov.cfg.OffTop = offOpts[cmd-idOffTop]
		ov.cfgChanged()
	case cmd >= idOffBottom && cmd < idOffBottom+uintptr(len(offOpts)):
		ov.cfg.OffBottom = offOpts[cmd-idOffBottom]
		ov.cfgChanged()
	}
}

// cfgChanged aplica el cambio de config: guarda, reposiciona (y repinta vía ULW).
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
				return 0, 0, fmt.Errorf("modificador desconocido: %q", p)
			}
			r := []rune(strings.TrimSpace(p))
			if len(r) != 1 || !unicode.IsLetter(r[0]) && !unicode.IsDigit(r[0]) {
				return 0, 0, fmt.Errorf("invalid key: %q", p)
			}
			vk = uint32(r[0]) // letras/dígitos: VK == ASCII
		}
	}
	return mods, vk, nil
}
