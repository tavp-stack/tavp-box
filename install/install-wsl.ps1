# ============================================================
# tavpbox installer — Windows (WSL2 + LXD)
# Jalankan di PowerShell SEBAGAI ADMINISTRATOR:
#   powershell -ExecutionPolicy Bypass -File install/install-wsl.ps1
# ============================================================
$ErrorActionPreference = "Stop"

Write-Host "==> tavpbox installer (Windows / WSL2)" -ForegroundColor Cyan

# ── 1. Pastikan WSL2 + distro Ubuntu ────────────────────────
                     Write-Host "    cek WSL2..."
                     if (-not (wsl --list 2>$null | Select-String -Quiet "Ubuntu")) {
    Write-Host "    install WSL2 + Ubuntu (bisa restart)..."
    wsl --install -d Ubuntu
    Write-Host "    Setelah reboot, jalankan installer ini lagi." -ForegroundColor Yellow
    pause
    exit 0
}

# ── 2. Bootstrap di dalam WSL2: LXD + tavpbox ───────────────
$ScriptDir = (Get-Item $PSScriptRoot).Parent.FullName -replace '\\', '/'
Write-Host "    setup LXD + tavpbox di WSL2..."
wsl bash -lc @"
set -e
    if ! command -v lxc >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y lxd whiptail jq && sudo lxd init --auto
        sudo usermod -aG lxd "\$USER"
    fi
sudo install -Dm755 '$ScriptDir/bin/tavpbox' /usr/local/bin/tavpbox
mkdir -p ~/.tavpbox/services ~/.tavpbox/stacks
cp '$ScriptDir'/services/*.tavp.sh ~/.tavpbox/services/ 2>/dev/null || true
cp '$ScriptDir'/stacks/*.tavp.sh ~/.tavpbox/stacks/ 2>/dev/null || true
"@

# ── 3. Windows shim: `tavpbox` bisa dipanggil dari PowerShell ─
                     Write-Host "    buat shim Windows..."
                     $wrapDir = "$env:USERPROFILE\tavpbox"
New-Item -ItemType Directory -Force -Path $wrapDir | Out-Null
@"
@echo off
wsl tavpbox %*
"@ | Set-Content -Encoding ascii "$wrapDir\tavpbox.cmd"

# Tambah ke PATH user
$envPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($envPath -notlike "*$wrapDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$envPath;$wrapDir", "User")
}

# ── 4. DNS: *.tavp.local -> IP WSL2 (NRPT) ──────────────────
$wslIp = (wsl hostname -I).Trim().Split()[0]
Write-Host "    set DNS *.tavp.local -> $wslIp"
Remove-DnsClientNrptRule -Namespace ".tavp.local" -ErrorAction SilentlyContinue
Add-DnsClientNrptRule -Namespace ".tavp.local" -NameServers @($wslIp) -ErrorAction SilentlyContinue

Write-Host "==> selesai. Buka PowerShell baru, jalankan: tavpbox init" -ForegroundColor Green
Write-Host "    (catatan: IP WSL2 bisa berubah tiap reboot; jalankan installer ini lagi bila domain tak resolve)" -ForegroundColor Yellow
