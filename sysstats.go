package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modpdh      = windows.NewLazySystemDLL("pdh.dll")

	procGetSystemTimes               = modkernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx         = modkernel32.NewProc("GlobalMemoryStatusEx")
	procPdhOpenQuery                 = modpdh.NewProc("PdhOpenQuery")
	procPdhAddEnglishCounterW        = modpdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData          = modpdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterArrayW = modpdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhRemoveCounter             = modpdh.NewProc("PdhRemoveCounter")
	procPdhCloseQuery                = modpdh.NewProc("PdhCloseQuery")
)

// ---- CPU: GetSystemTimes ----

type CPUMeter struct {
	prevIdle, prevKern, prevU uint64
}

func ftou64(ft syscall.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

func (c *CPUMeter) Percent() float64 {
	var idle, kern, user syscall.Filetime
	r1, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kern)), uintptr(unsafe.Pointer(&user)))
	if r1 == 0 {
		return 0
	}
	i, k, u := ftou64(idle), ftou64(kern), ftou64(user)
	di, dk, du := i-c.prevIdle, k-c.prevKern, u-c.prevU
	c.prevIdle, c.prevKern, c.prevU = i, k, u
	if dk+du == 0 {
		return 0
	}
	busy := (dk - di) + du // kernel time incluye idle
	pct := float64(busy) / float64(dk+du) * 100
	if pct < 0 {
		pct = 0
	}
	return pct
}

// ---- RAM: GlobalMemoryStatusEx ----

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys, AvailPhys uint64
	TotalPageFile, AvailPageFile,
	TotalVirtual, AvailVirtual,
	AvailExtendedVirtual uint64
}

func ramStats() (loadPct uint32, usedGB, totalGB float64) {
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	r1, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if r1 == 0 {
		return 0, 0, 0
	}
	total := float64(ms.TotalPhys) / (1 << 30)
	used := total - float64(ms.AvailPhys)/(1<<30)
	return ms.MemoryLoad, used, total
}

// ---- GPU: PDH "GPU Engine" ----

const pdhFmtDouble = 0x00000200
const pdhMoreData = 0x800007D2

type pdhFmtCounterValue struct {
	CStatus uint32
	Value   float64 // con PDH_FMT_DOUBLE
}

type pdhFmtCounterValueItemW struct {
	Sz       *uint16
	FmtValue pdhFmtCounterValue
}

type GPUMeter struct {
	hq, hcEng, hcMem uintptr
	lastGpu, lastMem float64
}

func NewGPUMeter() *GPUMeter {
	g := &GPUMeter{}
	var hq uintptr
	if r1, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&hq))); r1 != 0 {
		return g
	}
	g.hq = hq
	for _, c := range []struct {
		path string
		dst  *uintptr
	}{
		{`\GPU Engine(*engtype_3D)\Utilization Percentage`, &g.hcEng},
		{`\GPU Adapter Memory(*)\Dedicated Usage`, &g.hcMem},
	} {
		p, _ := syscall.UTF16PtrFromString(c.path)
		var hc uintptr
		procPdhAddEnglishCounterW.Call(hq, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&hc)))
		*c.dst = hc // 0 si falla: esa métrica queda n/d
	}
	return g
}

func (g *GPUMeter) Close() {
	if g.hcEng != 0 {
		procPdhRemoveCounter.Call(g.hcEng)
	}
	if g.hcMem != 0 {
		procPdhRemoveCounter.Call(g.hcMem)
	}
	if g.hq != 0 {
		procPdhCloseQuery.Call(g.hq)
	}
	g.hq, g.hcEng, g.hcMem = 0, 0, 0
}

// sumCounter suma las instancias de un counter wildcard tras collect.
// keep filtra por nombre de instancia (nil = todas).
func (g *GPUMeter) sumCounter(hc uintptr, keep func(string) bool) (float64, bool) {
	if hc == 0 {
		return 0, false
	}
	var bufSize, itemCount uint32
	// ojo: la DLL escribe el tamaño en el 3er arg y la cuenta en el 4º
	// (verificado empíricamente; el nombre de los params de la MSDN engaña)
	r1, _, _ := procPdhGetFormattedCounterArrayW.Call(
		hc, pdhFmtDouble,
		uintptr(unsafe.Pointer(&bufSize)), uintptr(unsafe.Pointer(&itemCount)), 0)
	if r1 != pdhMoreData || bufSize == 0 {
		return 0, false
	}
	buf := make([]byte, bufSize)
	r1, _, _ = procPdhGetFormattedCounterArrayW.Call(
		hc, pdhFmtDouble,
		uintptr(unsafe.Pointer(&bufSize)), uintptr(unsafe.Pointer(&itemCount)), uintptr(unsafe.Pointer(&buf[0])))
	if r1 != 0 {
		return 0, false
	}
	sum := 0.0
	itemSize := unsafe.Sizeof(pdhFmtCounterValueItemW{})
	for i := uint32(0); i < itemCount; i++ {
		item := (*pdhFmtCounterValueItemW)(unsafe.Pointer(&buf[uintptr(i)*itemSize]))
		if item.FmtValue.CStatus == 0 && (keep == nil || keep(windows.UTF16PtrToString(item.Sz))) {
			sum += item.FmtValue.Value
		}
	}
	return sum, true
}

func (g *GPUMeter) Percent() float64 {
	// Collect siempre: VRAMUsedGB lee el counter de memoria de esta misma query.
	if g.hq != 0 {
		procPdhCollectQueryData.Call(g.hq)
	}
	// NVML (SM%, lo mismo que muestran Steam/panel NVIDIA) manda si está;
	// el counter GPU Engine 3D lee sistemáticamente más bajo.
	if d, ok := nvmlDev(); ok {
		var u struct{ GPU, Mem uint32 } // nvmlUtilization_t
		if r, _, _ := procNvUtil.Call(d, uintptr(unsafe.Pointer(&u))); r == 0 && u.GPU <= 100 {
			g.lastGpu = float64(u.GPU)
			return g.lastGpu
		}
	}
	if g.hq == 0 {
		return -1 // contador no disponible
	}
	if sum, ok := g.sumCounter(g.hcEng, nil); ok {
		if sum > 100 {
			sum = 100
		}
		g.lastGpu = sum
	}
	return g.lastGpu
}

// VRAMUsedGB devuelve GB dedicados usados por el adaptador principal; -1 si no hay counter.
// Llamar después de Percent (que hace el Collect de la query).
func (g *GPUMeter) VRAMUsedGB() float64 {
	if g.hq == 0 {
		return -1
	}
	if sum, ok := g.sumCounter(g.hcMem, func(name string) bool {
		// PDH escribe el LUID en hex mayúscula ("luid_0x..._0x000139DB_phys_0")
		name = strings.ToLower(name)
		for _, l := range mainAdapterLUIDs {
			if strings.Contains(name, l) {
				return true
			}
		}
		return false
	}); ok {
		g.lastMem = sum / (1 << 30)
	}
	return g.lastMem
}

// ---- VRAM total: DXGI ----

var (
	moddxgi                = windows.NewLazySystemDLL("dxgi.dll")
	procCreateDXGIFactory1 = moddxgi.NewProc("CreateDXGIFactory1")
)

var iidDXGIFactory1 = windows.GUID{
	Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba,
	Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87},
}

// comCall invoca el método COM del slot n del objeto (slot 2 = Release).
// obj se mantiene unsafe.Pointer para que el GC lo vea vivo durante la llamada.
func comCall(obj unsafe.Pointer, slot uintptr, args ...uintptr) uintptr {
	vt := *(*unsafe.Pointer)(obj)
	fp := *(*uintptr)(unsafe.Add(vt, int(slot*unsafe.Sizeof(uintptr(0)))))
	var a [5]uintptr
	copy(a[:], args)
	r1, _, _ := syscall.Syscall6(fp, uintptr(len(args)+1), uintptr(obj), a[0], a[1], a[2], a[3], a[4])
	return r1
}

var (
	vramTotalCache   = -1.0
	mainAdapterLUIDs []string // substrings que identifican el adaptador principal en PDH
)

// parseAdapterDesc saca VRAM dedicada y LUID del buffer de GetDesc1.
// Hay DOS layouts en la naturaleza: el canónico (Flags tras Description,
// DVM@280, Luid@304) y otro sin Flags (DVM@272, Luid@296), visto
// empíricamente en una VM con RTX 3080. Se elige por validación: Flags es
// siempre <=4; si [256] es mayor, ahí va el VendorId.
type adapterDesc struct {
	dedicated         uintptr
	luidLow, luidHigh uint32
}

func parseAdapterDesc(buf []byte) (adapterDesc, bool) {
	var d adapterDesc
	read := func(off int) uintptr {
		return *(*uintptr)(unsafe.Pointer(&buf[off]))
	}
	plausible := func(v uintptr) bool { return v >= 64<<20 && v <= 1<<40 }
	off := 280 // canónico
	if *(*uint32)(unsafe.Pointer(&buf[256])) > 4 {
		off = 272 // sin Flags
	}
	if !plausible(read(off)) {
		off = 552 - off // 272<->280: prueba el otro layout
		if !plausible(read(off)) {
			return d, false
		}
	}
	d.dedicated = read(off)
	d.luidLow = *(*uint32)(unsafe.Pointer(&buf[off+24]))
	d.luidHigh = *(*uint32)(unsafe.Pointer(&buf[off+28]))
	return d, true
}

// vramTotalGB: memoria dedicada del adaptador con más VRAM (0 si DXGI falla).
// No cambia en caliente: se calcula una vez.
func vramTotalGB() float64 {
	if vramTotalCache >= 0 {
		return vramTotalCache
	}
	var factory unsafe.Pointer
	if r1, _, _ := procCreateDXGIFactory1.Call(
		uintptr(unsafe.Pointer(&iidDXGIFactory1)), uintptr(unsafe.Pointer(&factory))); r1 != 0 {
		vramTotalCache = 0
		return 0
	}
	defer comCall(factory, 2) // Release
	var best uintptr
	for i := uintptr(0); ; i++ {
		var adapter unsafe.Pointer
		// vtable[12] = EnumAdapters1; 0x887A0002 DXGI_ERROR_NOT_FOUND rompe el bucle
		if comCall(factory, 12, i, uintptr(unsafe.Pointer(&adapter))) != 0 {
			break
		}
		var buf [320]byte
		comCall(adapter, 10, uintptr(unsafe.Pointer(&buf))) // vtable[10] = GetDesc1
		comCall(adapter, 2)                                 // Release
		d, ok := parseAdapterDesc(buf[:])
		if ok && d.dedicated > best {
			best = d.dedicated
			// PDH nombra instancias "luid_0x..._0x..._phys_N"; el orden high/low
			// no está documentado, guardamos ambos por si acaso.
			mainAdapterLUIDs = []string{
				fmt.Sprintf("luid_0x%08x_0x%08x", d.luidHigh, d.luidLow),
				fmt.Sprintf("luid_0x%08x_0x%08x", d.luidLow, d.luidHigh),
			}
		}
	}
	vramTotalCache = float64(best) / (1 << 30)
	return vramTotalCache
}

// ---- Proceso en foco ----

// ForegroundProcess devuelve PID y nombre del proceso con la ventana en primer plano.
// Cadena vacía si es el escritorio o nosotros mismos.
func ForegroundProcess() (uint32, string) {
	hwnd := windows.GetForegroundWindow()
	if hwnd == 0 {
		return 0, ""
	}
	var pid uint32
	windows.GetWindowThreadProcessId(hwnd, &pid)
	if pid == 0 || pid == uint32(windows.GetCurrentProcessId()) {
		return 0, ""
	}
	return pid, procName(pid)
}

func procName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 260)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	name := filepath.Base(windows.UTF16ToString(buf[:n]))
	return strings.TrimSuffix(name, ".exe")
}

// ---- GPU temp (NVML, la lib del propio driver NVIDIA; sin admin) ----

var (
	modnvml    = windows.NewLazySystemDLL("nvml.dll")
	procNvInit = modnvml.NewProc("nvmlInit_v2")
	procNvDev  = modnvml.NewProc("nvmlDeviceGetHandleByIndex_v2")
	procNvTemp = modnvml.NewProc("nvmlDeviceGetTemperature")
	procNvUtil = modnvml.NewProc("nvmlDeviceGetUtilizationRates")
)

var nvmlTried, nvmlOK bool
var nvmlDevice uintptr

// nvmlDev devuelve el handle del primer dispositivo, cacheado tras el primer intento.
func nvmlDev() (uintptr, bool) {
	if !nvmlTried {
		nvmlTried = true
		if r, _, _ := procNvInit.Call(); r == 0 {
			var d uintptr
			if r, _, _ := procNvDev.Call(0, uintptr(unsafe.Pointer(&d))); r == 0 {
				nvmlDevice, nvmlOK = d, true
			}
		}
	}
	return nvmlDevice, nvmlOK
}

// GPUTempC: °C del sensor de la GPU, -1 si NVML no disponible (AMD/Intel/sin driver).
func GPUTempC() float64 {
	d, ok := nvmlDev()
	if !ok {
		return -1
	}
	var t uint32
	if r, _, _ := procNvTemp.Call(d, 0 /*NVML_TEMPERATURE_GPU*/, uintptr(unsafe.Pointer(&t))); r != 0 {
		return -1
	}
	return float64(t)
}

// ---- Snapshot compartido por CLI y overlay ----

type Snapshot struct {
	ProcName  string
	FPS       int
	CPU       float64
	GPU       float64 // -1 = no disponible
	GPUTemp   float64 // °C, -1 = no disponible
	RAMLoad   uint32
	RAMUsed   float64
	RAMTotal  float64
	VRAMUsed  float64 // GB, -1 = no disponible
	VRAMTotal float64
}

func collectSnapshot(tracker *FPSTracker, cpu *CPUMeter, gpu *GPUMeter) Snapshot {
	var fps int
	name := ""
	if tracker != nil {
		if pid, pname := ForegroundProcess(); pid != 0 && pname != "" {
			fps = tracker.FPS(pid)
			name = pname
		}
	}
	ramLoad, ramUsed, ramTotal := ramStats()
	gpuPct := gpu.Percent()
	return Snapshot{
		ProcName: name,
		FPS:      fps,
		CPU:      cpu.Percent(),
		GPU:      gpuPct,
		RAMLoad:  ramLoad, RAMUsed: ramUsed, RAMTotal: ramTotal,
		VRAMUsed: gpu.VRAMUsedGB(), VRAMTotal: vramTotalGB(),
		GPUTemp: GPUTempC(),
	}
}

// ---- Texto de cada métrica, compartido por CLI y overlay ----
// ("—" / "n/d" si el sensor no reporta; una sola copia de cada decisión).

func fmtND(ok bool, f string, a ...any) string {
	if !ok {
		return "n/d"
	}
	return fmt.Sprintf(f, a...)
}

func (s Snapshot) NameTxt() string {
	if s.ProcName == "" {
		return "—"
	}
	return s.ProcName
}

func (s Snapshot) FPSTxt() string {
	if s.ProcName == "" || s.FPS <= 0 {
		return "  —"
	}
	return fmt.Sprintf("%3d", s.FPS)
}

func (s Snapshot) GPUTxt() string  { return fmtND(s.GPU >= 0, "%.0f%%", s.GPU) }
func (s Snapshot) TempTxt() string { return fmtND(s.GPUTemp >= 0, "%.0f°C", s.GPUTemp) }

func (s Snapshot) VRAMTxt() string {
	switch {
	case s.VRAMUsed >= 0 && s.VRAMTotal > 0:
		return fmt.Sprintf("%.1f/%.1f GB", s.VRAMUsed, s.VRAMTotal)
	case s.VRAMUsed >= 0:
		return fmt.Sprintf("%.1f GB", s.VRAMUsed)
	default:
		return "n/d"
	}
}

func (s Snapshot) RAMTxt() string {
	return fmt.Sprintf("%.1f/%.1f GB", s.RAMUsed, s.RAMTotal)
}
