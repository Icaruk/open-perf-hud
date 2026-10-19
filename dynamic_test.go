package main

import "testing"

// avgLuma: BGRA order + Rec.601 coefficients (the 114 caught by review:
// a missing blue coefficient made blue backgrounds read as near-black).
func TestAvgLuma(t *testing.T) {
	if got := avgLuma([]byte{255, 0, 0, 255, 255, 0, 0, 255}); got != 29 { // pure blue
		t.Fatalf("avgLuma(blue) = %d, want 29", got)
	}
	if got := avgLuma([]byte{0, 0, 255, 255}); got != 76 { // pure red
		t.Fatalf("avgLuma(red) = %d, want 76 (BGRA order?)", got)
	}
}

// dynamicGray: thresholds with hysteresis, gray per background.
func TestDynamicGray(t *testing.T) {
	cases := []struct {
		lum             int
		darkIn, darkOut bool
		gray            int
	}{
		{50, false, true, 255},  // dark bg → switches to light text
		{50, true, true, 255},   // stays dark
		{200, true, false, 74},  // light bg → switches to dark text
		{200, false, false, 74}, // stays light
		{125, true, true, 255},  // dead band: keeps state
		{125, false, false, 74}, // dead band: keeps state
		{107, false, true, 255}, // lower edge: switches
		{149, true, false, 74},  // upper edge: switches
		{108, false, false, 74}, // inside the band: keeps state
		{148, true, true, 255},  // inside the band: keeps state
	}
	for _, c := range cases {
		dark := c.darkIn
		if g := dynamicGray(c.lum, &dark); g != c.gray || dark != c.darkOut {
			t.Fatalf("dynamicGray(%d, dark=%v) = %d, dark=%v; want %d, %v",
				c.lum, c.darkIn, g, dark, c.gray, c.darkOut)
		}
	}
}
