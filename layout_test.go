package main

import (
	"testing"
	"unsafe"
)

// applyAlpha: black background with bgA, white text with textA (premultiplied).
func TestApplyAlpha(t *testing.T) {
	px := []byte{
		0, 0, 0, 0, // background
		255, 255, 255, 0, // text
		128, 128, 128, 0, // 50% antialias
	}
	applyAlpha(px, 200, 64, 255)
	// background: alpha=bgA, color 0
	if px[3] != 64 {
		t.Fatalf("bg alpha = %d, want 64", px[3])
	}
	// text: premult color = 255*200/255 = 200; alpha = composite over the background
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

// applyAlpha with reduced brightness: color scales, alpha doesn't.
func TestApplyAlphaGray(t *testing.T) {
	px := []byte{255, 255, 255, 0, 0, 0, 0, 0} // text, background
	applyAlpha(px, 255, 0, 100)                // gray 100 (~39% white)
	if px[1] != byte(255*100/255) {
		t.Fatalf("text color = %d, want %d", px[1], 255*100/255)
	}
	if px[3] != 255 {
		t.Fatalf("text alpha = %d, want 255 (gray doesn't touch alpha)", px[3])
	}
	if px[7] != 0 {
		t.Fatalf("bg alpha = %d, want 0", px[7])
	}
}

// Structs must replicate the exact x64 C layout; a wrong pad shifts fields
// and the Win32 APIs read garbage (see the tray icon bug).
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
	// critical NOTIFYICONDATAW offsets
	var nid notifyIconDataW
	if o := unsafe.Offsetof(nid.UFlags); o != 20 {
		t.Errorf("UFlags at offset %d, want 20", o)
	}
	if o := unsafe.Offsetof(nid.UCallbackMessage); o != 24 {
		t.Errorf("UCallbackMessage at offset %d, want 24", o)
	}
	if o := unsafe.Offsetof(nid.HIcon); o != 32 {
		t.Errorf("HIcon at offset %d, want 32", o)
	}
	// DXGI_ADAPTER_DESC1 is no longer parsed with a fixed struct: there are
	// two layouts in the wild (with/without Flags) and parseAdapterDesc picks
	// by validation.
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
			t.Errorf("flagsFirst=%v: got %+v ok=%v, want 10GiB luid 139db-0", first, d, ok)
		}
	}
}

func TestColorGray(t *testing.T) {
	// Grey/Grey-dark 1 replicate Steam's greys (9e9e9e/8a8a8a).
	if colorGray["Grey"] != 0x9e || colorGray["Grey-dark 1"] != 0x8a {
		t.Fatalf("Steam greys wrong: %d/%d", colorGray["Grey"], colorGray["Grey-dark 1"])
	}
	// Dynamic has no fixed gray (sampleGray picks it); the rest map 1:1.
	if len(colorOpts) != len(colorGray)+1 || colorOpts[0] != "Dynamic" || len(colorOpts) > 10 {
		t.Fatalf("inconsistent palette: %d/%d (+Dynamic)", len(colorOpts), len(colorGray))
	}
}
