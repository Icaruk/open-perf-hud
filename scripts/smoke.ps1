# Smoke: overlay 6s sin morir + CLI 4s con linea de metricas.
$ErrorActionPreference = 'Stop'

# instancia previa: el arranque preguntaría reiniciar/cancelar y colgaría el smoke
Stop-Process -Name open-perf-hud -Force -ErrorAction SilentlyContinue

$p = Start-Process -PassThru -FilePath .\open-perf-hud.exe -WorkingDirectory (Get-Location)
Start-Sleep 6
if ($p.HasExited) { throw "overlay murio (code $($p.ExitCode))" }
Stop-Process -Id $p.Id -Force
Write-Output 'overlay OK 6s'

$job = Start-Job { Set-Location $using:PWD; .\oph-cli.exe --cli }
Start-Sleep 8
Stop-Job $job
$out = (Receive-Job $job) -join "`n"
Remove-Job $job -Force
if ($out -notmatch 'FPS') { throw 'CLI sin linea de metricas' }
Write-Output 'CLI OK'
