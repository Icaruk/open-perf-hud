# Test visual del color de alerta: 9 colores de texto x 5 fondos, con la
# matemática exacta de alertRed + applyAlpha (alpha Opacity sobre fondo
# BgOpacity) y texto GDI real (Consolas px, como el HUD).
# Cierra la ventana: la consola imprime el mapping para reportar
# ("la fila 3 se ve mal" → fila 3 = White-grey 3).
param(
    [int]$Opacity = 180,   # 0-255, igual que config.json
    [int]$BgOpacity = 25,  # 0-100, igual que config.json
    [switch]$PngOnly       # guarda el PNG y no abre ventana
)
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Windows.Forms

# palette: MISMA lista y orden que colorGray en config.go
$palette = [ordered]@{
    'White'        = 255
    'White-grey 1' = 224
    'White-grey 2' = 198
    'White-grey 3' = 171
    'Grey'         = 158
    'Grey-dark 1'  = 138
    'Grey-dark 2'  = 110
    'Grey-black'   = 74
    'Black'        = 0
}
# fondos de escritorio para juzgar el contraste
$desktops = @(
    @('Negro', 0, 0, 0),
    @('Gris oscuro', 64, 64, 64),
    @('Gris medio', 128, 128, 128),
    @('Gris claro', 192, 192, 192),
    @('Blanco', 255, 255, 255)
)

function AlertRed([int]$gray) {
    # idéntico a alertRed() en overlay.go (Truncate, clamp 0-150)
    $x = [int][Math]::Truncate(($gray - 54.2) / 0.7874)
    if ($x -lt 0) { $x = 0 } elseif ($x -gt 150) { $x = 150 }
    ,@(255, $x, $x)
}

$cellW = 150; $cellH = 52; $labelW = 250; $headH = 28; $pad = 8
$W = $labelW + $cellW * $desktops.Count + $pad * 2
$H = $headH + $cellH * $palette.Count + $pad * 2

$bmp = New-Object System.Drawing.Bitmap($W, $H)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.Clear([System.Drawing.Color]::FromArgb(255, 24, 24, 24))

# fuente real del HUD (overlay.go): Consolas, px, títulos en bold
$fontT = New-Object System.Drawing.Font('Consolas', 15, [System.Drawing.FontStyle]::Bold, [System.Drawing.GraphicsUnit]::Pixel)
$fontV = New-Object System.Drawing.Font('Consolas', 15, [System.Drawing.FontStyle]::Regular, [System.Drawing.GraphicsUnit]::Pixel)
$fontH = New-Object System.Drawing.Font('Consolas', 11, [System.Drawing.FontStyle]::Regular, [System.Drawing.GraphicsUnit]::Pixel)
$fontL = New-Object System.Drawing.Font('Consolas', 11, [System.Drawing.FontStyle]::Regular, [System.Drawing.GraphicsUnit]::Pixel)
$brushHead = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 150, 150, 150))

# cabecera: fondos
$x0 = $pad + $labelW
for ($j = 0; $j -lt $desktops.Count; $j++) {
    $g.DrawString($desktops[$j][0], $fontH, $brushHead, $x0 + $j * $cellW + 8, 6)
}

for ($i = 0; $i -lt $palette.Count; $i++) {
    $name = [string]$palette.Keys[$i]
    $gray = $palette.Values[$i]
    $ar = AlertRed $gray
    $y = $headH + $i * $cellH

    # etiqueta de fila: nº + nombre + gris + rgb de alerta
    $g.DrawString("Fila $($i+1)  $name ($gray)", $fontL, $brushHead, $pad, $y + 6)
    $g.DrawString("alerta rgb($($ar -join ','))", $fontL, $brushHead, $pad, $y + 24)

    for ($j = 0; $j -lt $desktops.Count; $j++) {
        # fondo real: escritorio con el negro del HUD (BgOpacity) encima
        $k = (100 - $BgOpacity) / 100.0
        $back = @([int]($desktops[$j][1] * $k), [int]($desktops[$j][2] * $k), [int]($desktops[$j][3] * $k))
        $rect = New-Object System.Drawing.Rectangle(($x0 + $j * $cellW), $y, ($cellW - 2), ($cellH - 2))
        $bgBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, $back[0], $back[1], $back[2]))
        $g.FillRectangle($bgBrush, $rect)

        # texto real: título en el color elegido, valor en el rojo de alerta,
        # ambos con alpha = Opacity (misma composición que applyAlpha)
        $cx = $rect.X + 8
        $brT = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb($Opacity, $gray, $gray, $gray))
        $brA = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb($Opacity, $ar[0], $ar[1], $ar[2]))
        $g.DrawString('CPU', $fontT, $brT, $cx, $y + 10)
        $cx += [int]$g.MeasureString('CPU', $fontT).Width
        $g.DrawString('99%', $fontV, $brA, $cx, $y + 10)
        # referencia sin alerta al lado
        $cx += [int]$g.MeasureString('99%', $fontV).Width + 6
        $g.DrawString('50%', $fontV, $brT, $cx, $y + 10)
    }
}
$g.Dispose()

$png = Join-Path $PSScriptRoot 'alert-test.png'
$bmp.Save($png, [System.Drawing.Imaging.ImageFormat]::Png)
Write-Host "PNG: $png"

if ($PngOnly) { return }

$form = New-Object System.Windows.Forms.Form
$form.Text = 'Alert test - fila + fondo que se vea mal = ajuste necesario'
$form.FormBorderStyle = [System.Windows.Forms.FormBorderStyle]::FixedSingle
$form.MaximizeBox = $false
$form.ClientSize = New-Object System.Drawing.Size($W, $H)
$pb = New-Object System.Windows.Forms.PictureBox
$pb.Image = $bmp
$pb.Dock = [System.Windows.Forms.DockStyle]::Fill
$form.Controls.Add($pb)
[void]$form.ShowDialog()

Write-Host "`nMapping para reportar (fila → color de texto → rojo de alerta):"
for ($i = 0; $i -lt $palette.Count; $i++) {
    $ar = AlertRed $palette.Values[$i]
    Write-Host ("  fila {0,2} = {1,-13} ({2,3}) → rgb({3})" -f ($i + 1), [string]$palette.Keys[$i], $palette.Values[$i], ($ar -join ','))
}
