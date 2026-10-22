package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// Overlay config (config.json next to the exe).
type Config struct {
	Corner      int     `json:"corner"`      // 0=top-left 1=top-right 2=bottom-left 3=bottom-right
	Margin      string  `json:"margin"`      // none|xs|sm|md|lg
	Opacity     uint8   `json:"opacity"`     // 0-255
	FontSize    string  `json:"fontSize"`    // xs|sm|md|lg
	Orientation string  `json:"orientation"` // vertical|horizontal
	Metrics     Metrics `json:"metrics"`     // which lines the overlay shows
	BgOpacity   int     `json:"bgOpacity"`   // 0-100 (%), opacity of the background only
	TextColor   string  `json:"textColor"`   // text color (colorOpts)
	InnerMargin string  `json:"innerMargin"` // none|xs|sm|md|lg, inner padding
	Spacing     string  `json:"spacing"`     // none|xs|sm|md|lg, gap between lines
	OffLeft     offset  `json:"offsetLeft"`  // extra px from the left edge (0 = none)
	OffRight    offset  `json:"offsetRight"` // extra px from the right edge
	OffTop      offset  `json:"offsetTop"`   // extra px from the top edge
	OffBottom   offset  `json:"offsetBottom"`
	RefreshMs   uint32  `json:"refreshMs"` // overlay refresh rate
	Hotkey      string  `json:"hotkey"`    // "CTRL+SHIFT+O"
}

// Metrics: per-metric toggles (tray menu ▸ Metrics).
type Metrics struct {
	Name    bool `json:"name"` // name of the focused executable
	FPS     bool `json:"fps"`
	API     bool `json:"api"` // graphics API of the focused game (DX9/DX11/…)
	CPUPct  bool `json:"cpuPct"`
	GPUPct  bool `json:"gpuPct"`
	GPUTemp bool `json:"gpuTemp"` // °C via NVML (NVIDIA)
	VRAMPct bool `json:"vramPct"`
	VRAMGB  bool `json:"vramGB"`
	RAMPct  bool `json:"ramPct"`
	RAMGB   bool `json:"ramGB"`
}

var metricLabels = []string{"Executable name", "FPS", "Graphics API", "CPU %", "GPU %", "GPU temp", "VRAM %", "VRAM GB", "RAM %", "RAM GB"}

// metricPtrs aligns the toggles with metricLabels (fixed order).
func (c *Config) metricPtrs() [10]*bool {
	return [10]*bool{&c.Metrics.Name, &c.Metrics.FPS, &c.Metrics.API, &c.Metrics.CPUPct, &c.Metrics.GPUPct,
		&c.Metrics.GPUTemp, &c.Metrics.VRAMPct, &c.Metrics.VRAMGB, &c.Metrics.RAMPct, &c.Metrics.RAMGB}
}

var fontSizes = []string{"xs", "sm", "md", "lg"}
var fontPx = map[string]int{"xs": 13, "sm": 15, "md": 18, "lg": 24}

var margins = []string{"none", "xs", "sm", "md", "lg"}
var marginPx = map[string]int{"none": 0, "xs": 4, "sm": 8, "md": 12, "lg": 24}

var spacings = []string{"xl", "lg", "md", "sm", "xs", "none"} // menu order
var spacingPx = map[string]int{"none": 0, "xs": 2, "sm": 5, "md": 8, "lg": 12, "xl": 20}

// orientation: value in config.json + name in the menu.
type orientation struct {
	val, name string
}

var orientations = []orientation{{"vertical", "Vertical"}, {"horizontal", "Horizontal"}}

// orientationOf returns the orientation with that val (the first one if it
// doesn't exist; LoadConfig validates beforehand).
func orientationOf(val string) orientation {
	for _, o := range orientations {
		if o.val == val {
			return o
		}
	}
	return orientations[0]
}

var bgPcts = func() (out []int) {
	for p := range 21 { // 0, 5, 10 … 100 (steps of 5)
		out = append(out, p*5)
	}
	return
}()

// bgLabels: menu labels for bgPcts.
var bgLabels = func() (out []string) {
	for _, p := range bgPcts {
		out = append(out, fmt.Sprintf("%d%%", p))
	}
	return out
}()

// snapBg rounds bg to the nearest multiple of 5 (the dialog steps by 5) and
// keeps it 0-100; a hand-edited 33 becomes 35.
func snapBg(bg int) int {
	bg = (bg + 2) / 5 * 5
	return min(max(bg, 0), 100)
}

// offset: per-edge px offset, 0 = none, 0-9999 (clamped in normalize).
// Unmarshals the current numeric format and the old string one ("none", "16").
type offset int

func (o *offset) UnmarshalJSON(b []byte) error {
	var n int
	if json.Unmarshal(b, &n) == nil {
		*o = offset(n)
		return nil
	}
	var s string
	if json.Unmarshal(b, &s) == nil && s != "none" {
		if n, err := strconv.Atoi(s); err == nil {
			*o = offset(n)
			return nil
		}
	}
	*o = 0 // "none", empty or garbage
	return nil
}

// clampOffset keeps a hand-edited offset inside 0-9999.
func clampOffset(o offset) offset {
	return offset(min(max(int(o), 0), 9999))
}

// Text grayscale; Grey/Grey-dark 1 are Steam's (9e9e9e/8a8a8a).
// "Dynamic" has no entry in colorGray: sampleGray picks its gray per
// background; textGray() is its base/fallback (White-grey 2).
var colorOpts = []string{"Dynamic", "White", "White-grey 1", "White-grey 2", "White-grey 3", "Grey", "Grey-dark 1", "Grey-dark 2", "Grey-black", "Black"}
var colorGray = map[string]int{"White": 255, "White-grey 1": 224, "White-grey 2": 198, "White-grey 3": 171, "Grey": 158, "Grey-dark 1": 138, "Grey-dark 2": 110, "Grey-black": 74, "Black": 0}

// textGray: base gray for normal text. Dynamic has no fixed gray —
// White-grey 2 is its base (sampleGray refines it per background, and it is
// the fallback when the screen can't be captured).
func (c *Config) textGray() int {
	if c.TextColor == "Dynamic" {
		return colorGray["White-grey 2"]
	}
	return colorGray[c.TextColor]
}

var defaultCfg = Config{Corner: 1, Margin: "none", Opacity: 180, FontSize: "sm", Orientation: "horizontal",
	RefreshMs: 500, Hotkey: "CTRL+SHIFT+O", BgOpacity: 25, InnerMargin: "none", Spacing: "xs", TextColor: "White-grey 2",
	Metrics: Metrics{Name: false, FPS: true, CPUPct: true, GPUPct: true, GPUTemp: true, VRAMPct: true, VRAMGB: true, RAMPct: true, RAMGB: true}}

func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func LoadConfig() Config {
	cfg := defaultCfg
	b, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	cfg.normalize()
	return cfg
}

// normalize validates/fixes hand-edited fields (pure, so tests can hit it).
func (c *Config) normalize() {
	if c.RefreshMs < 100 {
		c.RefreshMs = defaultCfg.RefreshMs
	}
	// out-of-range corner would panic in the settings dialog (cornerNames[Corner])
	if c.Corner < 0 || c.Corner > 3 {
		c.Corner = defaultCfg.Corner
	}
	c.BgOpacity = snapBg(c.BgOpacity)
	if orientationOf(c.Orientation).val != c.Orientation {
		c.Orientation = defaultCfg.Orientation
	}
	fixStr(&c.FontSize, fontPx, defaultCfg.FontSize)
	fixStr(&c.Margin, marginPx, defaultCfg.Margin)
	fixStr(&c.InnerMargin, marginPx, defaultCfg.InnerMargin)
	fixStr(&c.Spacing, spacingPx, defaultCfg.Spacing)
	if !slices.Contains(colorOpts, c.TextColor) {
		c.TextColor = defaultCfg.TextColor
	}
	if c.TextColor == "Dynamic" {
		c.BgOpacity = 0 // sampleGray needs the real background visible
	}
	for _, f := range []*offset{&c.OffLeft, &c.OffRight, &c.OffTop, &c.OffBottom} {
		*f = clampOffset(*f)
	}
}

// fixStr returns the default value if the loaded one is not in the valid table.
func fixStr(dst *string, valid map[string]int, def string) {
	if _, ok := valid[*dst]; !ok {
		*dst = def
	}
}

func SaveConfig(cfg Config) {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err == nil {
		_ = os.WriteFile(configPath(), b, 0o644)
	}
}
