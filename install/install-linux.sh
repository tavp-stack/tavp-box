#!/usr/bin/env bash
# ============================================================
# tavpbox installer — Linux native (Ubuntu/Debian/Fedora/Arch)
# Run: sudo bash install/install-linux.sh
# ============================================================
set -eu

[ "$(id -u)" -eq 0 ] || { echo "ERROR: jalankan dengan sudo"; exit 1; }

echo "==> tavpbox installer (Linux native)"

# ── Install LXD ─────────────────────────────────────────────
if ! command -v lxc >/dev/null 2>&1; then
    echo "    install LXD..."
    if command -v snap >/dev/null 2>&1; then
        snap install lxd && lxd init --auto
    elif command -v apt-get >/dev/null 2>&1; then
        apt-get update && apt-get install -y lxd && lxd init --auto
    elif command -v dnf >/dev/null 2>&1; then
        dnf install -y lxd && lxd init --auto
    elif command -v pacman >/dev/null 2>&1; then
        pacman -S --noconfirm lxd && lxd init --auto
    else
        echo "ERROR: package manager tidak dikenali"; exit 1
    fi
    usermod -aG lxd "${SUDO_USER:-$USER}" 2>/dev/null || true
fi

# ── whiptail (TUI) + jq (parsing) ──────────────────────────
command -v whiptail >/dev/null 2>&1 || {
    apt-get install -y whiptail 2>/dev/null \
    || dnf install -y newt 2>/dev/null \
    || pacman -S --noconfirm libnewt 2>/dev/null || true
}
command -v jq >/dev/null 2>&1 || {
    apt-get install -y jq 2>/dev/null \
    || dnf install -y jq 2>/dev/null \
    || pacman -S --noconfirm jq 2>/dev/null || true
}

# ── Copy CLI + catalog ──────────────────────────────────────
SRC="$(cd "$(dirname "$0")/.." && pwd)"
install -Dm755 "${SRC}/bin/tavpbox" /usr/local/bin/tavpbox
mkdir -p ~/.tavpbox/services ~/.tavpbox/stacks
cp "${SRC}"/services/*.tavp.sh ~/.tavpbox/services/ 2>/dev/null || true
cp "${SRC}"/stacks/*.tavp.sh ~/.tavpbox/stacks/ 2>/dev/null || true

echo "==> selesai. Jalankan: tavpbox init"
