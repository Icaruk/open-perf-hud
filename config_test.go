package main

import (
	"encoding/json"
	"testing"
)

// offset migration: numbers in, old string format ("none"/"16") and garbage
// still accepted.
func TestOffsetUnmarshal(t *testing.T) {
	for _, c := range []struct {
		json string
		want offset
	}{
		{`36`, 36},
		{`0`, 0},
		{`"36"`, 36},
		{`"none"`, 0},
		{`""`, 0},
		{`"abc"`, 0},
	} {
		var o offset
		if err := json.Unmarshal([]byte(c.json), &o); err != nil || o != c.want {
			t.Errorf("Unmarshal(%s) = %d, %v; want %d", c.json, o, err, c.want)
		}
	}
	// full Config wiring; clamping to 9999 is normalize's job, not the unmarshal's
	var cfg Config
	err := json.Unmarshal([]byte(`{"offsetLeft":"36","offsetRight":"none","offsetTop":7,"offsetBottom":"12000"}`), &cfg)
	if err != nil || cfg.OffLeft != 36 || cfg.OffRight != 0 || cfg.OffTop != 7 || cfg.OffBottom != 12000 {
		t.Fatalf("Config offsets = %d %d %d %d (%v)", cfg.OffLeft, cfg.OffRight, cfg.OffTop, cfg.OffBottom, err)
	}
	// and back out: saving writes numbers
	out, err := json.Marshal(cfg.OffLeft)
	if err != nil || string(out) != "36" {
		t.Fatalf("Marshal(offset) = %s (%v), want 36", out, err)
	}
}

func TestSnapBg(t *testing.T) {
	for _, c := range []struct{ in, want int }{
		{0, 0}, {2, 0}, {3, 5}, {33, 35}, {37, 35}, {38, 40},
		{50, 50}, {99, 100}, {100, 100}, {103, 100}, {-9, 0},
	} {
		if got := snapBg(c.in); got != c.want {
			t.Errorf("snapBg(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// normalize: hand-edited configs come back inside the valid ranges, and
// Dynamic forces the background off (sampleGray needs the real background).
func TestNormalize(t *testing.T) {
	cfg := defaultCfg
	cfg.BgOpacity = 33
	cfg.OffLeft = -5
	cfg.OffBottom = 12000
	cfg.normalize()
	if cfg.BgOpacity != 35 {
		t.Errorf("bgOpacity = %d, want 35", cfg.BgOpacity)
	}
	if cfg.OffLeft != 0 || cfg.OffBottom != 9999 {
		t.Errorf("offsets = %d/%d, want 0/9999", cfg.OffLeft, cfg.OffBottom)
	}

	cfg = defaultCfg
	cfg.TextColor = "Dynamic"
	cfg.BgOpacity = 40
	cfg.normalize()
	if cfg.BgOpacity != 0 {
		t.Errorf("Dynamic bgOpacity = %d, want 0", cfg.BgOpacity)
	}
}

func TestTextGray(t *testing.T) {
	cfg := defaultCfg // White-grey 2
	if g := cfg.textGray(); g != 198 {
		t.Errorf("textGray(default) = %d, want 198", g)
	}
	cfg.TextColor = "Dynamic"
	if g := cfg.textGray(); g != 198 { // base/fallback gray of Dynamic
		t.Errorf("textGray(Dynamic) = %d, want 198", g)
	}
	cfg.TextColor = "White"
	if g := cfg.textGray(); g != 255 {
		t.Errorf("textGray(White) = %d, want 255", g)
	}
}
