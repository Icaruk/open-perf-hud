package main

import (
	"testing"
	"unsafe"
)

// applyAlpha: fondo negro con bgA, texto blanco con textA (premultiplicado).
func TestApplyAlpha(t *testing.T) {
	px := []byte{
		0, 0, 0, 0, // fondo
		255, 255, 255, 0, // texto
		128, 128, 128, 0, // antialias 50%
	}
	applyAlpha(px, 3, 1, 200, 64, 255, nil)
	// fondo: alpha=bgA, color 0
	if px[3] != 64 {
		t.Fatalf("bg alpha = %d, want 64", px[3])
	}
	// texto: color premult = 255*200/255 = 200; alpha = composite sobre el fondo
	wantTextA := byte(255*200/255 + 64*(255-255*200/255)/255)
	if px[7] != wantTextA {
		t.Fatalf("text alpha = %d, want %d", px[7], wantTextA)
	}
	if px[6] != 200 || px[5] != 200 || px[4] != 200 {
		t.Fatalf("text color = %v, want 200,200,200", px[4:7])
	}
	// aa 50%: a1 = 128*200/255 = 100; alpha = 100 + 64*155/255 = 138
	wantA := byte(128*200/255 + 64*(255-128*200/255)/255)
	if px[11] != wantA {
		t.Fatalf("aa alpha = %d, want %d", px[11], wantA)
	}
	if px[10] != 100 { // 128*200/255 = 100
		t.Fatalf("aa color = %d, want 100", px[10])
	}
}

// applyAlpha con brillo reducido: el color escala, el alpha no.
func TestApplyAlphaGray(t *testing.T) {
	px := []byte{255, 255, 255, 0, 0, 0, 0, 0} // texto, fondo
	applyAlpha(px, 2, 1, 255, 0, 100, nil)     // gris 100 (~39% blanco)
	if px[1] != byte(255*100/255) {
		t.Fatalf("text color = %d, want %d", px[1], 255*100/255)
	}
	if px[3] != 255 {
		t.Fatalf("text alpha = %d, want 255 (el gris no toca el alpha)", px[3])
	}
	if px[7] != 0 {
		t.Fatalf("bg alpha = %d, want 0", px[7])
	}
}

// applyAlpha en zona de alerta: el span se tiñe del rojo de alerta y el
// resto conserva el gris del tema.
func TestApplyAlphaAlert(t *testing.T) {
	px := []byte{255, 255, 255, 0, 255, 255, 255, 0} // dos píxeles de texto
	applyAlpha(px, 2, 1, 255, 0, 198, []alertSpan{{x0: 0, x1: 1, y0: 0, y1: 1}})
	_, gr, gb := alertRed(198)
	if px[0] != 255 || px[1] != uint8(gr) || px[2] != uint8(gb) {
		t.Errorf("píxel alerta = %v, want %d,%d,%d", px[0:3], 255, gr, gb)
	}
	if px[3] != 255 {
		t.Errorf("alpha alerta = %d, want 255", px[3])
	}
	if px[5] != 198 { // fuera del span: gris del tema
		t.Errorf("píxel normal = %d, want 198", px[5])
	}
}

// alertRed: rojo puro en el extremo oscuro, clamp de saturación en el claro,
// luminancia Rec.709 igualada al gris en la zona media y monotónico.
func TestAlertRed(t *testing.T) {
	if r, g, b := alertRed(0); r != 255 || g != 0 || b != 0 {
		t.Errorf("alertRed(0) = %d,%d,%d, want 255,0,0", r, g, b)
	}
	if _, g, _ := alertRed(255); g > 150 {
		t.Errorf("alertRed(255): g = %d, want <= 150 (que no degrade a rosa)", g)
	}
	_, g, _ := alertRed(0x9e) // Grey
	L := 0.2126*255 + 0.7874*float64(g)
	if L < 157 || L > 159 {
		t.Errorf("luminancia alertRed(Grey) = %.1f, want ~158", L)
	}
	_, g1, _ := alertRed(100)
	_, g2, _ := alertRed(140)
	if g1 >= g2 {
		t.Errorf("alertRed no monótono: %d >= %d", g1, g2)
	}
}

// alertOf: flag de alerta del último seg (el valor) de la línea con ese título.
func alertOf(t *testing.T, lines [][]seg, title string) bool {
	t.Helper()
	for _, l := range lines {
		if len(l) > 0 && l[0].s == title {
			return l[len(l)-1].alert
		}
	}
	t.Fatalf("línea %q no encontrada", title)
	return false
}

// Umbral de alerta por métrica: CPU/GPU ≥99, RAM/VRAM ≥95% del total; n/d
// y valores justo por debajo del umbral no alertan.
func TestLinesAlert(t *testing.T) {
	cfg := defaultCfg
	cfg.Orientation = "vertical" // una línea por métrica
	mk := func(f func(*Snapshot)) Snapshot {
		s := Snapshot{CPU: 50, GPU: 50, GPUTemp: -1,
			RAMLoad: 50, RAMUsed: 8, RAMTotal: 16,
			VRAMUsed: 4, VRAMTotal: 8}
		f(&s)
		return s
	}
	for _, c := range []struct {
		name string
		snap Snapshot
		line string
		want bool
	}{
		{"CPU 98.9", mk(func(s *Snapshot) { s.CPU = 98.9 }), "CPU", false},
		{"CPU 99", mk(func(s *Snapshot) { s.CPU = 99 }), "CPU", true},
		{"GPU n/d", mk(func(s *Snapshot) { s.GPU = -1 }), "GPU", false},
		{"GPU 99", mk(func(s *Snapshot) { s.GPU = 99 }), "GPU", true},
		{"VRAM 7.5/8", mk(func(s *Snapshot) { s.VRAMUsed = 7.5 }), "VRAM", false},
		{"VRAM 7.6/8", mk(func(s *Snapshot) { s.VRAMUsed = 7.6 }), "VRAM", true},
		{"VRAM sin total", mk(func(s *Snapshot) { s.VRAMUsed = 4; s.VRAMTotal = 0 }), "VRAM", false},
		{"RAM 94", mk(func(s *Snapshot) { s.RAMLoad = 94 }), "RAM", false},
		{"RAM 95", mk(func(s *Snapshot) { s.RAMLoad = 95 }), "RAM", true},
	} {
		ov := &overlay{cfg: cfg, snap: c.snap}
		if got := alertOf(t, ov.lines(), c.line); got != c.want {
			t.Errorf("%s: alerta = %v, want %v", c.name, got, c.want)
		}
	}
}

// Los structs deben replicar el layout C x64 exacto; un pad erróneo
// desplaza campos y las APIs Win32 leen basura (v. bug del tray icon).
func TestStructLayoutC(t *testing.T) {
	for _, c := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"notifyIconDataW", unsafe.Sizeof(notifyIconDataW{}), 976},
		{"monitorInfo", unsafe.Sizeof(monitorInfo{}), 40},
		{"msg", unsafe.Sizeof(msg{}), 48},
		{"wndClassExW", unsafe.Sizeof(wndClassExW{}), 80},
		{"paintStruct", unsafe.Sizeof(paintStruct{}), 72},
		{"pdhFmtCounterValueItemW", unsafe.Sizeof(pdhFmtCounterValueItemW{}), 24},
	} {
		if c.got != c.want {
			t.Errorf("%s: size %d, want %d (broken C layout)", c.name, c.got, c.want)
		}
	}
	// offsets críticos de NOTIFYICONDATAW
	var nid notifyIconDataW
	if o := unsafe.Offsetof(nid.UFlags); o != 20 {
		t.Errorf("UFlags en offset %d, esperado 20", o)
	}
	if o := unsafe.Offsetof(nid.UCallbackMessage); o != 24 {
		t.Errorf("UCallbackMessage en offset %d, esperado 24", o)
	}
	if o := unsafe.Offsetof(nid.HIcon); o != 32 {
		t.Errorf("HIcon en offset %d, esperado 32", o)
	}
	// DXGI_ADAPTER_DESC1 ya no se parsea con struct fijo: hay dos layouts en
	// la naturaleza (con/sin Flags) y parseAdapterDesc elige por validación.
}

func TestParseAdapterDesc(t *testing.T) {
	mk := func(flagsFirst bool) [320]byte {
		var b [320]byte
		ids := []uint32{0x10de, 0x2206, 0x87b01043, 0xa1} // NVIDIA 3080
		off := 256
		if flagsFirst {
			off = 260
		}
		for i, v := range ids {
			*(*uint32)(unsafe.Pointer(&b[off+4*i])) = v
		}
		vOff, lOff := 272, 296
		if flagsFirst {
			vOff, lOff = 280, 304
		}
		*(*uint64)(unsafe.Pointer(&b[vOff])) = 10 << 30
		*(*uint32)(unsafe.Pointer(&b[lOff])) = 0x139db
		*(*uint32)(unsafe.Pointer(&b[lOff+4])) = 0
		return b
	}
	for _, first := range []bool{true, false} {
		b := mk(first)
		d, ok := parseAdapterDesc(b[:])
		if !ok || d.dedicated != 10<<30 || d.luidLow != 0x139db || d.luidHigh != 0 {
			t.Errorf("flagsFirst=%v: got %+v ok=%v, querida 10GiB luid 139db-0", first, d, ok)
		}
	}
}

func TestColorGray(t *testing.T) {
	// Grey/Grey-dark 1 replican los grises de Steam (9e9e9e/8a8a8a).
	if colorGray["Grey"] != 0x9e || colorGray["Grey-dark 1"] != 0x8a {
		t.Fatalf("grises Steam mal: %d/%d", colorGray["Grey"], colorGray["Grey-dark 1"])
	}
	if len(colorOpts) != len(colorGray) || len(colorOpts) > 10 {
		t.Fatalf("paleta inconsistente: %d/%d", len(colorOpts), len(colorGray))
	}
}
