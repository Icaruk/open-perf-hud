# Test visual del rojo de alerta (pendiente de ejecutar en Windows)

Estado: **escrito a ciegas** (este host es macOS, sin PowerShell; System.Drawing es
Windows-only). Pendiente ejecutar y ajustar si alguna fila/fondo falla.

## Cómo correrlo (host Windows)

```powershell
task alerttest            # ventana + PNG
# o directo:
pwsh -NoProfile -File scripts/alert-test.ps1
pwsh -NoProfile -File scripts/alert-test.ps1 -PngOnly
pwsh -NoProfile -File scripts/alert-test.ps1 -Opacity 255 -BgOpacity 0
```

Qué pinta: 9 filas (los colores de `colorGray` en config.go) × 5 fondos de
escritorio. Cada celda: `CPU` en el color de texto elegido, `99%` en el rojo de
alerta, `50%` de referencia sin alerta. Usa la matemática exacta de
`alertRed` + `applyAlpha` (alpha `Opacity` sobre fondo `BgOpacity`) y la fuente
real (Consolas 15px, bold en títulos).

## Qué reportar

Cierra la ventana: la consola imprime `fila N = nombre (gris) → rgb(...)`.
Con decir *"la fila 3 sobre Gris medio se ve mal"* ya se localiza el caso.

## Valores esperados del rojo (verificación cruzada del port)

`x = trunc((gray−54.2)/0.7874)` clamp 0..150 → rgb(255,x,x). En las etiquetas
de fila debe aparecer:

| fila | color         | gris | alerta esperada |
|-----:|---------------|-----:|-----------------|
| 1    | White         | 255  | rgb(255,150,150) (clamp) |
| 2    | White-grey 1  | 224  | rgb(255,150,150) (clamp) |
| 3    | White-grey 2  | 198  | rgb(255,150,150) (clamp) |
| 4    | White-grey 3  | 171  | rgb(255,148,148) |
| 5    | Grey          | 158  | rgb(255,131,131) |
| 6    | Grey-dark 1   | 138  | rgb(255,106,106) |
| 7    | Grey-dark 2   | 110  | rgb(255,70,70)   |
| 8    | Grey-black    | 74   | rgb(255,25,25)   |
| 9    | Black         | 0    | rgb(255,0,0)     |

## Dónde ajustar si algo falla

- Tono/clamp del rojo: `alertRed()` en `overlay.go` (y su espejo en
  `scripts/alert-test.ps1`, función `AlertRed`).
- Umbral de alerta: consts `alertCPUPct/alertGPUPct/alertRAMLoad/alertVRAMFrac`
  en `overlay.go`.

## Checklist pendiente (otro host)

- [ ] `pwsh -NoProfile -File scripts/alert-test.ps1` abre sin error de sintaxis
- [ ] etiquetas de fila coinciden con la tabla de valores esperados
- [ ] PNG `scripts/alert-test.png` generado
- [ ] juicio visual: alguna fila/fondo pide ajuste de `alertRed` (reportar y retocar)
