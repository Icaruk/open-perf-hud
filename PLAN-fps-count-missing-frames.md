# Plan: FPS mal leídos en Outer Wilds (DX11)

Reporte: «no lee bien los fps en el outer wilds que usa DX11, lee pero muestra 40 fps, baja a 8 (mentira) y a veces pone -».

Los 3 síntomas son **subconteo**: 40 cuando debería haber ~60, caídas a 8, y `—` (0 eventos en la ventana de 1s).

## Diagnóstico

Flujo actual: sesión ETW real-time (DXGI 0x2B/0x38 + D3D9 0x2 + DxgKrnl 0xAB/0xD7, filtro por event-ID) → `FPSTracker.add(pid, ts)` → overlay/CLI cada 500ms cuenta eventos del PID en foco en ventana deslizante de 1s.

Outer Wilds (Unity, DX11) emite **por cada present**: DXGI `Present_Stop` (0x2B) **y** DxgKrnl `PresentHistoryDetailed_Start` (0xD7), casi al mismo timestamp.

Dos causas plausibles en el código:

1. **El dedup colapsa frames reales** (`fps.go:50`). El dedup es simétrico: descarta *cualquier* evento ≤2ms del anterior del mismo PID, sin mirar el proveedor. Tras un tirón de frame, el motor recupera presentando frames reales a <2ms → se tragan. Además `t.last` se actualiza también al deduplicar, así que una ráfaga a ≤2ms colapsa **en cadena a 1**. Explica el 40 fijo (microstutters regulares), los picos de 8 y parte de los `—`.

2. **Pérdida de eventos ETW real-time**. La sesión usa defaults (BufferSize=64KB, sin Minimum/MaximumBuffers). goetw expone `Consumer.LostEvents` y hoy no se mide. Explicaría los `—` y las caídas bruscas.

Descartado por evidencia: conversión de timestamps (goetw convierte QPC→FILETIME correctamente, con o sin `PROCESS_TRACE_MODE_RAW_TIMESTAMP`) y PID equivocado (juego Unity de un solo proceso).

No se puede medir desde macOS (ETW no existe ahí): los cambios se compilan cruzados y se validan en la máquina Windows.

## Cambios

- [ ] **1. Dedup por clase de proveedor** (`fps.go`)
  - `add(pid, ts, kernel bool)`; el callback pasa `er.EventHeader.ProviderId == GUID DxgKrnl` (GUID parseado una vez en `StartFPS`).
  - Por PID, dos slots: `lastR` (runtime) y `lastK` (kernel), en lugar del `last` único.
  - **Runtime** (DXGI/D3D9): cuenta siempre; solo se descarta si llega ≤2ms tras un evento kernel (espejo del mismo present). Al contar o descartarse, `lastR = ts`.
  - **Kernel** (DxgKrnl): se descarta si llega ≤2ms tras `lastR` o `lastK`; al descartarse **no** actualiza ningún slot (esto mata el colapso en cadena). Si cuenta, `lastK = ts`.
  - `ponytail:` los juegos kernel-only (Vulkan/OpenGL) con frames <2ms siguen colapsando — techo documentado, igual que hoy.
  - Ventana de 1s, pruning y cleanup: sin cambios.
- [ ] **2. Buffers de sesión** (`fps.go`, `StartFPS`): tras `NewRealTimeSession`, `props := s.TraceProperties()` → `props.MinimumBuffers = 32; props.MaximumBuffers = 64`. Elimina la clase de pérdida por buffer lleno.
- [ ] **3. `--probe` diagnóstico** (`fps.go`, `runProbe`): al final imprimir `c.LostEvents.Load()` (y RTLostBuffer si está expuesto) y la tasa/segundo del 0x2B (eventos/segundos de probe ≈ FPS real si el juego está en foco). Así el reportero pinza el culpable sin más código.
- [ ] **4. Tests de regresión** (`fps_test.go`):
  - Dos frames runtime a 1ms entre sí → cuentan ambos (hoy fallaría: el 2º se deduplica).
  - Par runtime+kernel ≤2ms → 1 (regresión del dedup del par).
  - Cadena runtime → kernel espejo → runtime real 1,5ms después → 2 (no colapso en cadena).
  - Par kernel-only ≤2ms → 1 (techo documentado).

## Verificación

- [ ] macOS: `GOOS=windows GOARCH=amd64 go vet ./... && GOOS=windows GOARCH=amd64 go test -c -o /dev/null .`
- [ ] Windows: `task check` (gofmt/vet/test — los tests corren aquí)
- [ ] Windows, con Outer Wilds en foco:
  - `oph-cli.exe --cli` al lado del contador de Steam: los números deben coincidir en juego estable y en tirones.
  - `oph-cli.exe --probe 10s`: `LostEvents` debe ser 0 y el 0x2B/segundo ≈ FPS real.
  - Si sigue mal: pegar la salida del probe (histograma + lost events) y diagnosticar con datos.
