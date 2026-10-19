# Smoke: overlay alive for 6s + CLI printing a metrics line for 4s.
$ErrorActionPreference = 'Stop'

# a previous instance would make startup ask restart/cancel and hang the smoke
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
