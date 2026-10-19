package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/tekert/goetw/etw"
)

// Present providers and events (GameTechDev/PresentMon manifests, MIT).
const (
	provDXGI    = "{CA11C036-0102-4A2D-A6AD-F03CFED5D3C9}"
	provD3D9    = "{783ACA0A-790E-4D7F-8451-AA850511C6B9}"
	provDxgKrnl = "{802EC45A-1E99-4B83-9920-87C98277BA9D}"

	kwDXGI       = 0x8000000000000002 // Analytic|Events
	kwD3D9       = 0x8000000000000002
	kwDxgPresent = 0x8000000 // DxgKrnl "Present"
)

const (
	// We count Stop/Detailed events: on Win11 the game emits Present_Stop but
	// Present_Start never fires (verified with --probe); Stop = 1 per Present,
	// always. 0xD7 is the kernel path (Vulkan/OpenGL). The tracker's 2ms dedup
	// absorbs the runtime+kernel overlap of the same present.
	dxgiPresentStop     = 0x2B
	dxgiMPOPresentStop  = 0x38
	d3d9PresentStop     = 0x2
	dxgPresentHistStart = 0xAB // Win10
	dxgPresentHistDet   = 0xD7 // PresentHistoryDetailed_Start, Win11
)

// FPSTracker counts Present events per PID in a 1s sliding window.
type FPSTracker struct {
	mu     sync.Mutex
	events map[uint32][]time.Time
	last   map[uint32]time.Time
}

func (t *FPSTracker) add(pid uint32, ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// A windowed present emits a runtime event (DXGI/D3D9) AND a kernel one
	// (PresentHistory): symmetric 2ms dedup on the EVENT timestamp. The consumer
	// delivers in buffer bursts; with time.Now() (consumption time) a whole burst
	// collapsed to 1 and FPS was always 1. Ceiling ~500 FPS.
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
	// ponytail: cleanup by size, enough for long sessions
	if len(t.events) > 256 {
		for p, evs := range t.events {
			if len(evs) == 0 || ts.Sub(evs[len(evs)-1]) > 30*time.Second {
				delete(t.events, p)
				delete(t.last, p)
			}
		}
	}
}

// FPS frames of pid in the last second.
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

// providerDefs: the 3 Present providers. With filtered=true adds the event-ID
// filter (counting); without it all events arrive (probe).
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

// StartFPS creates the ETW session (admin required) and a background consumer.
// Returns the tracker and a cleanup func.
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

// runProbe: enables the 3 providers WITHOUT an ID filter and histograms what
// arrives. Empirical verification of the Present IDs (run as admin with a game
// open).
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
			fmt.Printf("ETW error (not admin?): %v\n", err)
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
		fmt.Println("consumer error:", err)
		return
	}
	defer c.Stop()
	defer s.Stop()

	fmt.Printf("Probing ETW events for %v (open/move a game or video)...\n", d)
	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()
	if len(hist) == 0 {
		fmt.Println("0 events. Did you run as admin? Is anything presenting frames?")
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
			fmt.Printf("  Id 0x%04X (%5d): %d events  %s\n", id, id, hist[n][uint16(id)], probeLabels[n][uint16(id)])
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
