# agentrig installer for Windows:
#   irm https://tabhishekpaul.github.io/agentrig/install.ps1 | iex
# Installs arig to %LOCALAPPDATA%\arig\bin, adds it to your PATH and starts the dashboard.
$ErrorActionPreference = 'Stop'

$repo = 'tabhishekpaul/agentrig'
$dir = Join-Path $env:LOCALAPPDATA 'arig\bin'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$asset = "arig_windows_$arch.zip"
$base = if ($env:ARIG_DOWNLOAD) { $env:ARIG_DOWNLOAD } else { "https://github.com/$repo/releases/latest/download" }
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("arig-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
  Write-Host "Downloading arig for windows/$arch..."
  Invoke-WebRequest "$base/$asset" -OutFile "$tmp\$asset" -UseBasicParsing
  Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt" -UseBasicParsing

  # Never install something that doesn't match the published checksum.
  $want = (Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match "\s\*?$([regex]::Escape($asset))$" } | ForEach-Object { ($_ -split '\s+')[0] })
  if (-not $want) { throw "$asset is not listed in checksums.txt" }
  $got = (Get-FileHash "$tmp\$asset" -Algorithm SHA256).Hash.ToLower()
  if ($got -ne $want.ToLower()) { throw "checksum mismatch for $asset" }

  Expand-Archive "$tmp\$asset" -DestinationPath $tmp -Force
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  Get-Process arig -ErrorAction SilentlyContinue | Stop-Process -Force  # an old arig holds the file open
  Copy-Item "$tmp\arig_windows_$arch\arig.exe" "$dir\arig.exe" -Force
} finally {
  Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dir) {
  [Environment]::SetEnvironmentVariable('Path', "$dir;$userPath", 'User')
  Write-Host "Added $dir to your PATH"
}

Write-Host "`narig installed to $dir\arig.exe"
if ($env:ARIG_NO_START -ne '1') { & "$dir\arig.exe" }
Write-Host 'Dashboard: http://arig.localhost:7377 (open a new terminal to use the arig command)'
