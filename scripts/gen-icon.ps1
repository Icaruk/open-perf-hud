# Genera icon.ico desde el diseño de icon.svg (gauge FPS).
# Uso: pwsh -NoProfile -File scripts/gen-icon.ps1
# ponytail: System.Drawing dibuja el mismo gauge que el SVG; sin dependencias externas.
Add-Type -AssemblyName System.Drawing

function New-RoundRect([float]$x, [float]$y, [float]$w, [float]$h, [float]$r) {
    $p = New-Object System.Drawing.Drawing2D.GraphicsPath
    $p.AddArc($x, $y, 2*$r, 2*$r, 180, 90)
    $p.AddArc($x+$w-2*$r, $y, 2*$r, 2*$r, 270, 90)
    $p.AddArc($x+$w-2*$r, $y+$h-2*$r, 2*$r, 2*$r, 0, 90)
    $p.AddArc($x, $y+$h-2*$r, 2*$r, 2*$r, 90, 90)
    $p.CloseFigure()
    $p
}

function New-IconBitmap([int]$size) {
    $s = $size / 128.0
    $b = New-Object System.Drawing.Bitmap($size, $size)
    $g = [System.Drawing.Graphics]::FromImage($b)
    $g.SmoothingMode = 'AntiAlias'
    $g.TextRenderingHint = 'AntiAlias'

    $bg = New-RoundRect 0 0 $size $size (28*$s)
    $g.FillPath((New-Object System.Drawing.SolidBrush ([System.Drawing.ColorTranslator]::FromHtml('#0B1220'))), $bg)

    function New-Pen([string]$hex, [float]$w) {
        $p = New-Object System.Drawing.Pen ([System.Drawing.ColorTranslator]::FromHtml($hex)), $w
        $p.StartCap = 'Round'; $p.EndCap = 'Round'
        $p
    }
    # circulo del gauge: centro (64,61) r=34, en coords 128 escaladas
    $cx = 64*$s; $cy = 61*$s; $R = 34*$s
    $rect = New-Object System.Drawing.RectangleF (($cx-$R), ($cy-$R), (2*$R), (2*$R))
    $g.DrawArc((New-Pen '#1E293B' (10*$s)), $rect, 135, 270)
    $g.DrawArc((New-Pen '#34D399' (10*$s)), $rect, 135, 185)
    $g.DrawLine((New-Pen '#F8FAFC' (6*$s)), $cx, $cy, (80.9*$s), (46.9*$s))

    $dotR = 5*$s
    $g.FillEllipse((New-Object System.Drawing.SolidBrush ([System.Drawing.ColorTranslator]::FromHtml('#F8FAFC'))),
        ($cx-$dotR), ($cy-$dotR), (2*$dotR), (2*$dotR))

    if ($size -ge 48) {
        $font = New-Object System.Drawing.Font('Consolas', (15*$s), [System.Drawing.FontStyle]::Regular, [System.Drawing.GraphicsUnit]::Pixel)
        $fmt = New-Object System.Drawing.StringFormat
        $fmt.Alignment = 'Center'
        $g.DrawString('FPS', $font, (New-Object System.Drawing.SolidBrush ([System.Drawing.ColorTranslator]::FromHtml('#7DD3FC'))), $cx, (99*$s), $fmt)
    }
    $g.Dispose()
    $b
}

function New-BmpEntry([System.Drawing.Bitmap]$bmp) {
    # entrada ICO formato BMP 32bpp (bottom-up BGRA + mascara AND en cero)
    $w = $bmp.Width; $h = $bmp.Height
    $ms = New-Object System.IO.MemoryStream
    $bw = New-Object System.IO.BinaryWriter($ms)
    $maskRow = [math]::Ceiling($w / 8); if ($maskRow % 4) { $maskRow += 4 - ($maskRow % 4) }
    $bw.Write([uint32]40); $bw.Write([int32]$w); $bw.Write([int32](2*$h))
    $bw.Write([uint16]1); $bw.Write([uint16]32)
    $bw.Write([uint32]0); $bw.Write([uint32]($w*$h*4 + $maskRow*$h)); $bw.Write([int32]0); $bw.Write([int32]0)
    $bw.Write([uint32]0); $bw.Write([uint32]0)
    $rect = New-Object System.Drawing.Rectangle(0, 0, $w, $h)
    $data = $bmp.LockBits($rect, 'ReadOnly', [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $row = New-Object byte[] ($w*4)
    for ($y = $h-1; $y -ge 0; $y--) {
        $ptr = [IntPtr]::Add($data.Scan0, ($y * $data.Stride))
        [System.Runtime.InteropServices.Marshal]::Copy($ptr, $row, 0, ($w*4))
        for ($i = 0; $i -lt $w*4; $i += 4) { $t = $row[$i]; $row[$i] = $row[$i+2]; $row[$i+2] = $t } # ARGB->BGRA
        $bw.Write($row)
    }
    $bmp.UnlockBits($data)
    $zero = New-Object byte[] $maskRow
    for ($y = 0; $y -lt $h; $y++) { $bw.Write($zero) }
    $bw.Flush(); $ms.ToArray()
}

$out = Join-Path $PSScriptRoot '..\icon.ico'
$sizes = 16, 32, 48, 256
$blobs = @(); foreach ($sz in $sizes) { $bmp = New-IconBitmap $sz; $blobs += , (New-BmpEntry $bmp); $bmp.Dispose() }

$ms = New-Object System.IO.MemoryStream
$bw = New-Object System.IO.BinaryWriter($ms)
$bw.Write([uint16]0); $bw.Write([uint16]1); $bw.Write([uint16]$blobs.Count)
$offset = 6 + 16 * $blobs.Count
for ($i = 0; $i -lt $blobs.Count; $i++) {
    $dim = if ($sizes[$i] -ge 256) { 0 } else { $sizes[$i] }
    $bw.Write([byte]$dim); $bw.Write([byte]$dim); $bw.Write([byte]0); $bw.Write([byte]0)
    $bw.Write([uint16]1); $bw.Write([uint16]32)
    $bw.Write([uint32]$blobs[$i].Length); $bw.Write([uint32]$offset)
    $offset += $blobs[$i].Length
}
foreach ($blob in $blobs) { $bw.Write([byte[]]$blob) }
$bw.Flush()
[System.IO.File]::WriteAllBytes(($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($out)), $ms.ToArray())
Write-Host "icon.ico generado: $((Get-Item $out).Length) bytes"
