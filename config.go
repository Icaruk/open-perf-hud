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
	OffLeft     string  `json:"offsetLeft"`  // extra px from the left edge (offOpts)
	OffRight    string  `json:"offsetRight"` // extra px from the right edge
	OffTop      string  `json:"offsetTop"`   // extra px from the top edge
	OffBottom   string  `json:"offsetBottom"`
	RefreshMs   uint32  `json:"refreshMs"` // overlay refresh rate
	Hotkey      string  `json:"hotkey"`    // "CTRL+SHIFT+O"
}

// Metrics: per-metric toggles (tray menu ▸ Metrics).
type Metrics struct {
	Name    bool `json:"name"` // name of the focused executable
	FPS     bool `json:"fps"`
	CPUPct  bool `json:"cpuPct"`
	GPUPct  bool `json:"gpuPct"`
	GPUTemp bool `json:"gpuTemp"` // °C via NVML (NVIDIA)
	VRAMPct bool `json:"vramPct"`
	VRAMGB  bool `json:"vramGB"`
	RAMPct  bool `json:"ramPct"`
	RAMGB   bool `json:"ramGB"`
}

var metricLabels = []string{"Executable name", "FPS", "CPU %", "GPU %", "GPU temp", "VRAM %", "VRAM GB", "RAM %", "RAM GB"}

// metricPtrs aligns the toggles with metricLabels (fixed order).
func (c *Config) metricPtrs() [9]*bool {
	return [9]*bool{&c.Metrics.Name, &c.Metrics.FPS, &c.Metrics.CPUPct, &c.Metrics.GPUPct,
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

var bgPcts = []int{0, 10, 25, 50, 75, 100}

// bgLabels: menu labels for bgPcts.
var bgLabels = func() (out []string) {
	for _, p := range bgPcts {
		out = append(out, fmt.Sprintf("%d%%", p))
	}
	return out
}()

// Per-edge offsets: "none" = 0, the rest are the px in their own label.
var offOpts = []string{"none", "2", "4", "8", "10", "12", "14", "16", "20", "24", "30", "40", "50"}

func offPx(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// Text grayscale; Grey/Grey-dark 1 are Steam's (9e9e9e/8a8a8a).
// "Dynamic" has no entry in colorGray: its gray is picked by sampleGray based on the background.
var colorOpts = []string{"Dynamic", "White", "White-grey 1", "White-grey 2", "White-grey 3", "Grey", "Grey-dark 1", "Grey-dark 2", "Grey-black", "Black"}
var colorGray = map[string]int{"White": 255, "White-grey 1": 224, "White-grey 2": 198, "White-grey 3": 171, "Grey": 158, "Grey-dark 1": 138, "Grey-dark 2": 110, "Grey-black": 74, "Black": 0}

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
	if cfg.RefreshMs < 100 {
		cfg.RefreshMs = defaultCfg.RefreshMs
	}
	// out-of-range corner would panic when opening the menu (cornerNames[Corner])
	if cfg.Corner < 0 || cfg.Corner > 3 {
		cfg.Corner = defaultCfg.Corner
	}
	if cfg.BgOpacity < 0 || cfg.BgOpacity > 100 {
		cfg.BgOpacity = defaultCfg.BgOpacity
	}
	if orientationOf(cfg.Orientation).val != cfg.Orientation {
		cfg.Orientation = defaultCfg.Orientation
	}
	fixStr(&cfg.FontSize, fontPx, defaultCfg.FontSize)
	fixStr(&cfg.Margin, marginPx, defaultCfg.Margin)
	fixStr(&cfg.InnerMargin, marginPx, defaultCfg.InnerMargin)
	fixStr(&cfg.Spacing, spacingPx, defaultCfg.Spacing)
	if !slices.Contains(colorOpts, cfg.TextColor) {
		cfg.TextColor = defaultCfg.TextColor
	}
	for _, f := range []*string{&cfg.OffLeft, &cfg.OffRight, &cfg.OffTop, &cfg.OffBottom} {
		if !slices.Contains(offOpts, *f) {
			*f = "none"
		}
	}
	return cfg
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
