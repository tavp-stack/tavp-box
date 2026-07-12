#!/usr/bin/env bash
# ============================================================
# tavpbox installer — macOS (via Lima VM + LXD)
# Run: bash install/install-mac.sh
# ============================================================
set -eu

echo "==> tavpbox installer (macOS)"

command -v brew >/dev/null 2>&1 || { echo "ERROR: install Homebrew dulu: https://brew.sh"; exit 1; }

# ── Lima (Linux VM host for LXC) ────────────────────────────
brew install lima
limactl start template://ubuntu --name=tavpbox 2>/dev/null || limactl start tavpbox

# ── Bootstrap inside Lima: LXD + tavpbox ────────────────────
SRC="$(cd "$(dirname "$0")/.." && pwd)"
limactl shell tavpbox -- bash -c '
    set -e
    if ! command -v lxc >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y lxd whiptail jq && sudo lxd init --auto
        sudo usermod -aG lxd "$USER"
    fi
    sudo install -Dm755 '"${SRC}"'/bin/tavpbox /usr/local/bin/tavpbox
    mkdir -p ~/.tavpbox/services ~/.tavpbox/stacks
    cp '"${SRC}"'/services/*.tavp.sh ~/.tavpbox/services/ 2>/dev/null || true
    cp '"${SRC}"'/stacks/*.tavp.sh ~/.tavpbox/stacks/ 2>/dev/null || true
'

# ── macOS resolver: *.tavp.local -> Lima IP ─────────────────
LIMA_IP="$(limactl list tavpbox --format '{{.IPAddress}}' 2>/dev/null | head -1)"
sudo mkdir -p /etc/resolver
echo "nameserver ${LIMA_IP}" | sudo tee /etc/resolver/tavp.local >/dev/null

# ── macOS wrapper: `tavpbox` dari Terminal ──────────────────
cat > /usr/local/bin/tavpbox <<EOF
#!/usr/bin/env bash
exec limactl shell tavpbox -- tavpbox "\$@"
EOF
chmod +x /usr/local/bin/tavpbox

echo "==> selesai. Jalankan: tavpbox init"
