package main

// Native settings dialog: programmatic Win32 controls, zero dependencies.
// One window at a time on the overlay's thread — the overlay's WM_TIMER
// keeps firing while it is open, so every change previews live.

import (
	"slices"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ---- Win32 constants for the dialog ----

const (
	wsOverlappedWindow = wsCaption | wsSysMenu | wsMinimizeBox
	wsCaption          = 0x00C00000
	wsSysMenu          = 0x00080000
	wsMinimizeBox      = 0x00020000
	wsChild            = 0x40000000
	wsVisible          = 0x10000000
	wsTabStop          = 0x00010000
	wsBorder           = 0x00800000
	wsVScroll          = 0x00200000

	wmCommand        = 0x0111
	wmCtlcolorstatic = 0x0138

	cbsDropdownlist = 0x0003
	bsGroupbox      = 0x0007
	bsAutocheckbox  = 0x0003
	bsPushbutton    = 0x0000
	esNumber        = 0x2000

	wmSetfont    = 0x0030
	cbAddstring  = 0x0143
	cbSetcursel  = 0x014E
	cbGetcursel  = 0x0147
	bmGetcheck   = 0x00F0
	bmSetcheck   = 0x00F1
	bstChecked   = 1
	enKillfocus  = 0x0200
	enChange     = 0x0300
	cbnSelchange = 1
	bnClicked    = 0

	whiteBrush   = 0 // GetStockObject
	colorWindow  = 5 // class background brush = COLOR_WINDOW + 1 (white)
	idcArrow     = 32512
	cwUseDefault = 0x80000000
	swShow       = 5

	idCombo = 100 // + index in comboDefs
	idOff   = 200 // + 0..3 (offLabels)
	idCheck = 300 // + index in metricLabels
	idClose = 400

	idxTextSize  = 1 // comboDefs indexes
	idxTextColor = 2
	idxBg        = 3

	maxOff = 9999
)

var (
	procSendMessageW     = moduser32.NewProc("SendMessageW")
	procEnableWindow     = moduser32.NewProc("EnableWindow")
	procGetWindowTextW   = moduser32.NewProc("GetWindowTextW")
	procSetWindowTextW   = moduser32.NewProc("SetWindowTextW")
	procAdjustWindowRect = moduser32.NewProc("AdjustWindowRect")
	procLoadCursorW      = moduser32.NewProc("LoadCursorW")
	procGetStockObject   = modgee32.NewProc("GetStockObject")
)

// comboDef: one COMBOBOX row — options, current index and setter. Successor
// of the old tray menuOpts, now against *Config.
type comboDef struct {
	label string
	opts  []string
	cur   func(*Config) int
	set   func(*overlay, int)
}

// curStr: current index of the config's value among opts.
func curStr(opts []string, get func(*Config) string) func(*Config) int {
	return func(c *Config) int { return slices.Index(opts, get(c)) }
}

var orientationNames = []string{orientations[0].name, orientations[1].name}

var comboDefs = []comboDef{
	{"Position", cornerNames[:], func(c *Config) int { return c.Corner },
		func(o *overlay, i int) { o.cfg.Corner = i }},
	{"Text size", fontSizes, curStr(fontSizes, func(c *Config) string { return c.FontSize }),
		func(o *overlay, i int) { o.cfg.FontSize = fontSizes[i]; o.makeFont() }},
	{"Text color", colorOpts, curStr(colorOpts, func(c *Config) string { return c.TextColor }),
		func(o *overlay, i int) { o.cfg.TextColor = colorOpts[i] }},
	{"Background", bgLabels, func(c *Config) int { return c.BgOpacity / 5 },
		func(o *overlay, i int) { o.cfg.BgOpacity = bgPcts[i] }},
	{"Layout", orientationNames, curStr(orientationNames, func(c *Config) string { return orientationOf(c.Orientation).name }),
		func(o *overlay, i int) { o.cfg.Orientation = orientations[i].val }},
	{"Margin", margins, curStr(margins, func(c *Config) string { return c.Margin }),
		func(o *overlay, i int) { o.cfg.Margin = margins[i] }},
	{"Inner margin", margins, curStr(margins, func(c *Config) string { return c.InnerMargin }),
		func(o *overlay, i int) { o.cfg.InnerMargin = margins[i] }},
	{"Item spacing", spacings, curStr(spacings, func(c *Config) string { return c.Spacing }),
		func(o *overlay, i int) { o.cfg.Spacing = spacings[i] }},
}

var offLabels = [4]string{"Left", "Right", "Top", "Bottom"}

func offPtrs(c *Config) [4]*offset {
	return [4]*offset{&c.OffLeft, &c.OffRight, &c.OffTop, &c.OffBottom}
}

type settingsDlg struct {
	ov     *overlay
	hwnd   uintptr
	font   uintptr
	combos []uintptr // len(comboDefs)
	edits  []uintptr // len(offLabels)
	checks []uintptr // len(metricLabels)
}

var (
	settingsInst       *settingsDlg // the open dialog, nil when closed
	settingsRegistered bool
)

// openSettings shows the settings window (or foregrounds the existing one).
func openSettings(ov *overlay) {
	if settingsInst != nil {
		procSetForegroundWindow.Call(settingsInst.hwnd)
		return
	}
	className, _ := syscall.UTF16PtrFromString("OpenPerfHudSettings")
	if !settingsRegistered {
		cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
		icon := loadAppIcon() // exe icon (rsrc.syso ID 1), same as the tray
		wc := wndClassExW{
			CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			LpfnWndProc:   syscall.NewCallback(settingsWndProc),
			HInstance:     hInstance(),
			HCursor:       cursor,
			HIcon:         icon,
			HIconSm:       icon,
			HBrBackground: colorWindow + 1, // white, modern flat look
			LpszClassName: className,
		}
		if r1, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r1 == 0 {
			return
		}
		settingsRegistered = true
	}
	// client 420x632 → total window size via AdjustWindowRect
	rc := rect{Right: 420, Bottom: 632}
	procAdjustWindowRect.Call(uintptr(unsafe.Pointer(&rc)), wsOverlappedWindow, 0)
	title, _ := syscall.UTF16PtrFromString("Open Perf HUD — Settings")
	hwnd, _, _ := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		uintptr(wsOverlappedWindow),
		cwUseDefault, cwUseDefault,
		uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top),
		0, 0, hInstance(), 0)
	if hwnd == 0 {
		return
	}
	d := &settingsDlg{ov: ov, hwnd: hwnd,
		combos: make([]uintptr, len(comboDefs)),
		edits:  make([]uintptr, len(offLabels)),
		checks: make([]uintptr, len(metricLabels))}
	d.font = d.makeFont()
	settingsInst = d
	d.buildControls()
	procShowWindow.Call(hwnd, swShow)
	procSetForegroundWindow.Call(hwnd)
}

// makeFont: Segoe UI 9pt (negative height = char height, not cell height),
// ClearType — the modern Windows UI font; DEFAULT_GUI_FONT looks dated.
func (d *settingsDlg) makeFont() uintptr {
	face, _ := syscall.UTF16PtrFromString("Segoe UI")
	h := int32(-12) // 9pt, negative = char height (variable: uintptr(-12) would overflow)
	f, _, _ := procCreateFontW.Call(
		uintptr(h), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0,
		uintptr(unsafe.Pointer(face)))
	return f
}

func settingsWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmCommand:
		if d := settingsInst; d != nil {
			d.command(wParam)
		}
	case wmCtlcolorstatic: // labels and group boxes: paint their text on white
		brush, _, _ := procGetStockObject.Call(whiteBrush)
		return brush
	case wmDestroy:
		if d := settingsInst; d != nil {
			procDeleteObject.Call(d.font)
		}
		settingsInst = nil // no PostQuitMessage: the overlay keeps running
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}

// command applies a WM_COMMAND notification: combo selection, offset edit,
// metric check or Close.
func (d *settingsDlg) command(wParam uintptr) {
	code, id := wParam>>16, wParam&0xffff
	ov := d.ov
	switch {
	case code == cbnSelchange && id >= idCombo && id < idCombo+uintptr(len(comboDefs)):
		i := int(id - idCombo)
		sel, _, _ := procSendMessageW.Call(d.combos[i], cbGetcursel, 0, 0)
		if int(sel) < 0 {
			return
		}
		comboDefs[i].set(ov, int(sel))
		if i == idxTextColor {
			d.syncBg()
		}
		ov.cfgChanged()
	case code == enChange && id >= idOff && id < idOff+uintptr(len(offLabels)):
		i := int(id - idOff)
		*offPtrs(&ov.cfg)[i] = offset(clampOff(winText(d.edits[i])))
		ov.cfgChanged()
	case code == enKillfocus && id >= idOff && id < idOff+uintptr(len(offLabels)):
		// EN_CHANGE already clamped the value; rewrite the text itself once
		// editing ends.
		if n := atoiOr0(winText(d.edits[id-idOff])); n > maxOff {
			setWinText(d.edits[id-idOff], strconv.Itoa(maxOff))
		}
	case code == bnClicked && id >= idCheck && id < idCheck+uintptr(len(metricLabels)):
		i := int(id - idCheck)
		chk, _, _ := procSendMessageW.Call(d.checks[i], bmGetcheck, 0, 0)
		*ov.cfg.metricPtrs()[i] = chk == bstChecked
		ov.cfgChanged()
	case code == bnClicked && id == idClose:
		procDestroyWindow.Call(d.hwnd)
	}
}

// syncBg: with Dynamic the background is forced to 0 (sampleGray needs the
// real background visible), so the combo locks at 0%; with a concrete gray
// it is enabled and shows the current value.
func (d *settingsDlg) syncBg() {
	dyn := d.ov.cfg.TextColor == "Dynamic"
	enable := uintptr(1)
	if dyn {
		enable = 0
		d.ov.cfg.BgOpacity = 0
	}
	procEnableWindow.Call(d.combos[idxBg], enable)
	sel := uintptr(comboDefs[idxBg].cur(&d.ov.cfg))
	if dyn {
		sel = 0
	}
	procSendMessageW.Call(d.combos[idxBg], cbSetcursel, sel, 0)
}

// buildControls creates the groups, combos, edits, checks and Close button.
func (d *settingsDlg) buildControls() {
	group := func(text string, y, h int32) {
		d.ctl("BUTTON", text, bsGroupbox, 8, y, 404, h, 0)
	}

	group("Layout", 8, 208)
	for i := range 5 { // Position … Layout
		d.combo(int32(i), 36+36*int32(i))
	}
	group("Spacing", 224, 132)
	for i := 5; i < len(comboDefs); i++ { // Margin, Inner margin, Item spacing
		d.combo(int32(i), 252+36*int32(i-5))
	}
	d.syncBg()

	group("Offsets (px)", 364, 68)
	ptrs := offPtrs(&d.ov.cfg)
	for i := range offLabels {
		x := int32(18 + i*94)
		d.ctl("STATIC", offLabels[i], 0, x, 396, 48, 20, 0)
		d.edits[i] = d.ctl("EDIT", "", wsBorder|wsTabStop|esNumber, x+48, 392, 44, 26, int32(idOff+i))
		setWinText(d.edits[i], strconv.Itoa(int(*ptrs[i])))
	}

	group("Metrics", 440, 144)
	mptrs := d.ov.cfg.metricPtrs()
	for i, label := range metricLabels {
		x := int32(20 + (i/5)*192)
		y := int32(466 + (i%5)*23)
		d.checks[i] = d.ctl("BUTTON", label, bsAutocheckbox|wsTabStop, x, y, 184, 22, int32(idCheck+i))
		if *mptrs[i] {
			procSendMessageW.Call(d.checks[i], bmSetcheck, bstChecked, 0)
		}
	}

	d.ctl("BUTTON", "Close", bsPushbutton|wsTabStop, 160, 594, 100, 30, idClose)
}

// combo creates the i-th combo row at y: label, options, current selection.
func (d *settingsDlg) combo(i, y int32) {
	def := &comboDefs[i]
	d.ctl("STATIC", def.label, 0, 20, y+4, 96, 20, 0)
	cb := d.ctl("COMBOBOX", "", cbsDropdownlist|wsTabStop|wsVScroll, 124, y, 272, 400, int32(idCombo+i))
	d.combos[i] = cb
	for _, o := range def.opts {
		t, _ := syscall.UTF16PtrFromString(o)
		procSendMessageW.Call(cb, cbAddstring, 0, uintptr(unsafe.Pointer(t)))
	}
	procSendMessageW.Call(cb, cbSetcursel, uintptr(def.cur(&d.ov.cfg)), 0)
}

func (d *settingsDlg) ctl(class, text string, style uintptr, x, y, w, h, id int32) uintptr {
	c, _ := syscall.UTF16PtrFromString(class)
	t, _ := syscall.UTF16PtrFromString(text)
	hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)),
		uintptr(wsChild|wsVisible|style), uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		d.hwnd, uintptr(id), hInstance(), 0)
	if hwnd != 0 {
		procSendMessageW.Call(hwnd, wmSetfont, d.font, 1)
	}
	return hwnd
}

// clampOff: text → 0-9999 (empty or garbage → 0).
func clampOff(s string) int {
	n, _ := strconv.Atoi(s)
	return min(max(n, 0), maxOff)
}

func atoiOr0(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// winText: current text of a control.
func winText(hwnd uintptr) string {
	var buf [16]uint16
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func setWinText(hwnd uintptr, s string) {
	t, _ := syscall.UTF16PtrFromString(s)
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(t)))
}
