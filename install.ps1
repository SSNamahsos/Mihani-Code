# Mihani Code installer for Windows (PowerShell 5+).
# Downloads the latest release binary, or falls back to building from source.
$ErrorActionPreference = "Stop"

$repo = "SSNamahsos/Mihani-Code"
$dest = Join-Path $env:USERPROFILE ".mihani\bin"
New-Item -ItemType Directory -Force -Path $dest | Out-Null

$arch = "amd64"
if ([Environment]::Is64BitOperatingSystem -eq $false) { $arch = "386" }
$assetName = "mihani-windows-$arch.exe"
$target = Join-Path $dest "mihani.exe"

function Add-ToUserPath([string]$dir) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$dir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
        Write-Host "Added $dir to your user PATH (restart the terminal to use 'mihani' anywhere)."
    }
}

Write-Host "Installing Mihani Code..."
try {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest"
    $asset = $release.assets | Where-Object { $_.name -eq $assetName } | Select-Object -First 1
    if (-not $asset) { throw "asset $assetName not found in release $($release.tag_name)" }
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $target -UseBasicParsing
    Write-Host "Installed $($release.tag_name) -> $target"
} catch {
    Write-Host "Release download unavailable: $($_.Exception.Message)"
    Write-Host "Falling back to go install (requires Go 1.24+)..."
    go install github.com/SSNamahsos/Mihani-Code/cmd/mihani@latest
    if ($LASTEXITCODE -ne 0) { Write-Error "go install failed"; exit 1 }
}

Add-ToUserPath $dest
Write-Host ""
Write-Host "Done. Run 'mihani' inside any project directory."
