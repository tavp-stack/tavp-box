# install-windows.ps1 — Install TAVP Box on Windows (via WSL2)
# Requires Windows 10 version 2004 or higher (Build 19041 or higher)
# Run as Administrator

[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

Write-Host "=== TAVP Box Windows Installer ===" -ForegroundColor Cyan
Write-Host ""

# ── 1. Check if running as Administrator ─────────────────────
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "Please run this script as Administrator." -ForegroundColor Red
    exit 1
}

# ── 2. Enable WSL2 and install Ubuntu ────────────────────────
Write-Host "Step 1: Checking WSL2..." -ForegroundColor Yellow
$wslStatus = wsl --status 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "WSL2 not installed. Installing..." -ForegroundColor Yellow
    # Enable WSL feature
    Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Windows-Subsystem-Linux -NoRestart
    Enable-WindowsOptionalFeature -Online -FeatureName VirtualMachinePlatform -NoRestart
    # Set WSL2 as default
    wsl --set-default-version 2
    Write-Host "WSL2 enabled. Please reboot and run this script again." -ForegroundColor Green
    exit 0
} else {
    Write-Host "WSL2 is installed." -ForegroundColor Green
}

# Check if Ubuntu is installed
$ubuntuInstalled = wsl -l -q | Where-Object { $_ -match "Ubuntu" }
if (-not $ubuntuInstalled) {
    Write-Host "Ubuntu not found. Installing Ubuntu 22.04..." -ForegroundColor Yellow
    # Download Ubuntu 2204
    $ubuntuUrl = "https://aka.ms/wslubuntu2204"
    $ubuntuMsi = "$env:TEMP\ubuntu2204.appx"
    Invoke-WebRequest -Uri $ubuntuUrl -OutFile $ubuntuMsi
    Add-AppxPackage -Path $ubuntuMsi
    Write-Host "Ubuntu installed. Please launch Ubuntu once to complete setup, then run this script again." -ForegroundColor Green
    exit 0
} else {
    Write-Host "Ubuntu is installed." -ForegroundColor Green
}

# ── 3. Inside Ubuntu, run install-linux.sh ────────────────────
Write-Host "Step 2: Installing TAVP Box inside Ubuntu..." -ForegroundColor Yellow
$wslCommand = @"
cd ~
if [ ! -f tavp-box/install-linux.sh ]; then
    sudo apt-get update
    sudo apt-get install -y git whiptail jq
    git clone https://github.com/tavp-stack/tavp-box.git
fi
cd tavp-box
sudo bash install-linux.sh
"@
wsl -d Ubuntu -u root -- bash -c "$wslCommand"

if ($LASTEXITCODE -eq 0) {
    Write-Host "TAVP Box installed inside WSL2." -ForegroundColor Green
} else {
    Write-Host "Failed to install TAVP Box inside WSL2." -ForegroundColor Red
    exit 1
}

# ── 4. Configure Windows hosts file for *.tavp.local ────────
# Since wildcard hosts is not supported, we'll create a scheduled task that updates hosts file when a box starts.
# For now, we'll add a static entry for the default box (if any).
Write-Host "Step 3: Configuring Windows hosts file..." -ForegroundColor Yellow
$hostsFile = "$env:SystemRoot\System32\drivers\etc\hosts"
# Check if entry already exists
$entry = "127.0.0.1   tavp.local"
if (Select-String -Path $hostsFile -Pattern "tavp.local" -Quiet) {
    Write-Host "Entry for tavp.local already exists." -ForegroundColor Green
} else {
    Add-Content -Path $hostsFile -Value "`n# TAVP Box`n$entry`n" -Encoding ASCII
    Write-Host "Added tavp.local to hosts file." -ForegroundColor Green
}

# ── 5. Create a scheduled task to start LXD on boot ──────────
Write-Host "Step 4: Creating scheduled task to start LXD on boot..." -ForegroundColor Yellow
$taskName = "TAVP Box LXD Startup"
$taskExists = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if (-not $taskExists) {
    $action = New-ScheduledTaskAction -Execute "wsl" -Argument "-d Ubuntu -u root -- systemctl start lxd"
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM"
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Principal $principal -Description "Start LXD service inside WSL2 at boot"
    Write-Host "Scheduled task created." -ForegroundColor Green
} else {
    Write-Host "Scheduled task already exists." -ForegroundColor Green
}

# ── 6. Install tavpbox CLI on Windows (optional) ─────────────
# We can create a wrapper script that calls wsl -d Ubuntu -u root -- tavpbox
Write-Host "Step 5: Creating tavpbox wrapper for Windows..." -ForegroundColor Yellow
$wrapperPath = "$env:ProgramFiles\TAVP Box\tavpbox.cmd"
New-Item -ItemType Directory -Path "$env:ProgramFiles\TAVP Box" -Force | Out-Null
$cmdContent = '@echo off
wsl -d Ubuntu -u root -- tavpbox %*'
Set-Content -Path $wrapperPath -Value $cmdContent -Encoding ASCII
# Add to PATH (system environment variable)
$envPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::Machine)
if ($envPath -notlike "*TAVP Box*") {
    [Environment]::SetEnvironmentVariable("Path", "$envPath;$env:ProgramFiles\TAVP Box", [EnvironmentVariableTarget]::Machine)
    Write-Host "Added tavpbox to PATH." -ForegroundColor Green
}

# ── 7. Done ──────────────────────────────────────────────────
Write-Host ""
Write-Host "=== Installation complete ===" -ForegroundColor Cyan
Write-Host "You can now use 'tavpbox' command from PowerShell." -ForegroundColor Green
Write-Host "To create a box, run: tavpbox create" -ForegroundColor Green
Write-Host ""
Write-Host "Note: DNS wildcard (*.tavp.local) is not automatically configured." -ForegroundColor Yellow
Write-Host "You may need to add hosts entries for each box manually." -ForegroundColor Yellow
Write-Host "Or use the TAVP Box Desktop app (recommended)." -ForegroundColor Yellow
