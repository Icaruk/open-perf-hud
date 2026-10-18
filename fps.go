package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tekert/goetw/etw"
)

// Providers y eventos Present (manifests de GameTechDev/PresentMon, MIT).
const (
	provDXGI    = "{CA11C036-0102-4A2D-A6AD-F03CFED5D3C9}"
	provD3D9    = "{783ACA0A-790E-4D7F-8451-AA850511C6B9}"
	provDxgKrnl = "{802EC45A-1E99-4B83-9920-87C98277BA9D}"

	kwDXGI       = 0x8000000000000002 // Analytic|Events
	kwD3D9       = 0x8000000000000002
	kwDxgPresent = 0x8000000 // DxgKrnl "Present"
)

const (
	// Contamos eventos Stop/Detailed: en Win11 el juego emite Present_Stop pero
	// Present_Start no dispara (verificado con --probe); Stop = 1 por Present,
	// siempre. 0xD7 es la vía kernel (Vulkan/OpenGL). El dedup de 2ms del tracker
	// absorbe el solape runtime+kernel del mismo present.
	dxgiPresentStop     = 0x2B
	dxgiMPOPresentStop  = 0x38
	d3d9PresentStop     = 0x2
	dxgPresentHistStart = 0xAB // Win10
	dxgPresentHistDet   = 0xD7 // PresentHistoryDetailed_Start, Win11
)

// FPSTracker cuenta eventos Present por PID en ventana deslizante de 1s.
type FPSTracker struct {
	mu     sync.Mutex
	events map[uint32][]time.Time
	last   map[uint32]time.Time
}

func (t *FPSTracker) add(pid uint32, ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Un present windowed emite evento runtime (DXGI/D3D9) Y kernel (PresentHistory):
	// dedup simétrico de 2ms sobre el timestamp DEL EVENTO. El consumer entrega en
	// ráfagas de buffer; con time.Now() (hora de consumo) una ráfaga entera colapsa
	// a 1 y el FPS sale siempre 1. Techo ~500 FPS.
	if la, ok := t.last[pid]; ok && ts.Sub(la) < 2*time.Millisecond {
		t.last[pid] = ts
		return
	}
	t.last[pid] = ts
	ev := append(t.events[pid], ts)
	cut := 0
	for cut < len(ev) && ts.Sub(ev[cut]) > time.Second {
		cut++
	}
	t.events[pid] = ev[cut:]
	// ponytail: limpieza por tamaño, suficiente para sesiones largas
	if len(t.events) > 256 {
		for p, evs := range t.events {
			if len(evs) == 0 || ts.Sub(evs[len(evs)-1]) > 30*time.Second {
				delete(t.events, p)
				delete(t.last, p)
			}
		}
	}
}

// FPS frames del pid en el último segundo.
func (t *FPSTracker) FPS(pid uint32) int {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	ev := t.events[pid]
	cut := 0
	for cut < len(ev) && now.Sub(ev[cut]) > time.Second {
		cut++
	}
	ev = ev[cut:]
	if cut > 0 {
		t.events[pid] = ev
	}
	return len(ev)
}

func presentProvider(guidStr string, kw uint64, ids ...uint16) (etw.Provider, error) {
	g, err := etw.ParseGUID(guidStr)
	if err != nil {
		return etw.Provider{}, fmt.Errorf("GUID %s: %w", guidStr, err)
	}
	p := etw.Provider{
		GUID:            *g,
		Name:            guidStr,
		EnableLevel:     0xff,
		MatchAnyKeyword: kw,
	}
	if len(ids) > 0 {
		p.Filters = append(p.Filters, etw.NewEventIDFilter(true, ids...))
	}
	return p, nil
}

// providerDefs: los 3 providers de Present. Con filtered=true añade el filtro
// de event-ID (conteo); sin él llegan todos los eventos (probe).
var providerDefs = []struct {
	name string
	guid string
	kw   uint64
	ids  []uint16
}{
	{"DXGI", provDXGI, kwDXGI, []uint16{dxgiPresentStop, dxgiMPOPresentStop}},
	{"D3D9", provD3D9, kwD3D9, []uint16{d3d9PresentStop}},
	{"DxgKrnl", provDxgKrnl, kwDxgPresent, []uint16{dxgPresentHistStart, dxgPresentHistDet}},
}

func providers(filtered bool) ([]etw.Provider, error) {
	var out []etw.Provider
	for _, d := range providerDefs {
		ids := d.ids
		if !filtered {
			ids = nil
		}
		p, err := presentProvider(d.guid, d.kw, ids...)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// StartFPS crea la sesión ETW (requiere admin) y el consumer en background.
// Devuelve el tracker y una func de cleanup.
func StartFPS(ctx context.Context) (*FPSTracker, func(), error) {
	tracker := &FPSTracker{events: make(map[uint32][]time.Time), last: make(map[uint32]time.Time)}

	provs, err := providers(true)
	if err != nil {
		return nil, nil, err
	}
	s := etw.NewRealTimeSession("OpenPerfHudFPS")
	for _, p := range provs {
		if err := s.EnableProvider(p); err != nil {
			return nil, nil, fmt.Errorf("ETW enable %s: %w", p.Name, err)
		}
	}

	c := etw.NewConsumer(ctx)
	c.FromSessions(s)
	c.EventRecordCallback = func(er *etw.EventRecord) bool {
		tracker.add(er.EventHeader.ProcessId, er.Timestamp())
		return false
	}
	if err := c.Start(); err != nil {
		return nil, nil, fmt.Errorf("ETW consumer: %w", err)
	}

	cleanup := func() {
		c.Stop()
		s.Stop()
	}
	return tracker, cleanup, nil
}

// runProbe: habilita los 3 providers SIN filtro de ID y histograma lo que llega.
// Verificación empírica de los IDs de Present (ejecutar como admin con un juego abierto).
func runProbe(d time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()

	var mu sync.Mutex
	hist := make(map[string]map[uint16]int)

	provs, err := providers(false)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	s := etw.NewRealTimeSession("OpenPerfHudProbe")
	for _, prov := range provs {
		if err := s.EnableProvider(prov); err != nil {
			fmt.Printf("error ETW (¿no eres admin?): %v\n", err)
			return
		}
	}

	c := etw.NewConsumer(ctx)
	c.FromSessions(s)
	shortNames := map[etw.GUID]string{}
	for _, p := range providerDefs {
		g, _ := etw.ParseGUID(p.guid)
		shortNames[*g] = p.name
	}
	c.EventRecordCallback = func(er *etw.EventRecord) bool {
		mu.Lock()
		name := shortNames[er.EventHeader.ProviderId]
		if name == "" {
			name = er.EventHeader.ProviderId.StringU()
		}
		m := hist[name]
		if m == nil {
			m = make(map[uint16]int)
			hist[name] = m
		}
		m[er.EventHeader.EventDescriptor.Id]++
		mu.Unlock()
		return false
	}
	if err := c.Start(); err != nil {
		fmt.Println("error consumer:", err)
		return
	}
	defer c.Stop()
	defer s.Stop()

	fmt.Printf("Probing ETW events for %v (open/move a game or video)...\n", d)
	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()
	if len(hist) == 0 {
		fmt.Println("0 eventos. ¿Ejecutaste como admin? ¿Hay algo presentando frames?")
		return
	}
	names := make([]string, 0, len(hist))
	for n := range hist {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println("\nProvider:", n)
		ids := make([]int, 0, len(hist[n]))
		for id := range hist[n] {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		for _, id := range ids {
			fmt.Printf("  Id 0x%04X (%5d): %d eventos  %s\n", id, id, hist[n][uint16(id)], probeLabels[n][uint16(id)])
		}
	}
}

var probeLabels = map[string]map[uint16]string{
	"DXGI": {
		0x2A: "<-- Present_Start",
		0x2B: "Present_Stop",
		0x37: "<-- PresentMultiplaneOverlay_Start",
		0x38: "PresentMultiplaneOverlay_Stop",
	},
	"D3D9": {
		0x01: "<-- Present_Start",
		0x02: "Present_Stop",
	},
	"DxgKrnl": {
		0xAB: "<-- PresentHistory_Start",
		0xAC: "PresentHistory_Info",
		0xD7: "PresentHistoryDetailed_Start",
		0xB8: "Present_Info",
		0xA8: "Flip_Info",
		0xFC: "FlipMultiPlaneOverlay_Info",
	},
}
