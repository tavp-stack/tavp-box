# ============================================================
# tavpbox — Caddy (reverse proxy + wildcard routing)
# ============================================================
TVP_CADDY_BIN="/usr/local/bin/caddy"
TVP_CADDY_DIR="/etc/caddy"
TVP_CADDY_BOXES="${TVP_CADDY_DIR}/boxes"

tvp_caddy_install() {
    if command -v caddy >/dev/null 2>&1; then return; fi
    tvp_spin "install caddy" bash -c '
        if command -v apt-get >/dev/null; then
            apt-get update && apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl
            curl -1sLf https://dl.cloudflare.steamstatic.com/caddy/stable/gpg.key | gpg --dearmor -o /usr/share/keyrings/caddy-stable.gpg 2>/dev/null || true
            echo "deb [signed-by=/usr/share/keyrings/caddy-stable.gpg] https://dl.cloudflare.steamstatic.com/caddy/stable/deb/debian all main" > /etc/apt/sources.list.d/caddy-stable.list
            apt-get update && apt-get install -y caddy
        else
            curl -fsSL https://caddyserver.com/api/download?os=linux -o /usr/local/bin/caddy && chmod +x /usr/local/bin/caddy
        fi'
}

tvp_setup_caddy() {
    local domain="$1"
    tvp_caddy_install
    mkdir -p "${TVP_CADDY_BOXES}"
    cat > "${TVP_CADDY_DIR}/Caddyfile" <<EOF
{
    auto_https off
    email nocert@${domain}
}
import ${TVP_CADDY_BOXES}/*.caddy
EOF
    # systemd / openrc / direct fallback
    if command -v systemctl >/dev/null 2>&1; then
        systemctl enable caddy 2>/dev/null; systemctl restart caddy 2>/dev/null || caddy start --config "${TVP_CADDY_DIR}/Caddyfile" &
    else
        pkill caddy 2>/dev/null || true
        nohup caddy run --config "${TVP_CADDY_DIR}/Caddyfile" >/var/log/caddy.log 2>&1 &
    fi
}

tvp_caddy_add_box() {
    local name="$1" ip="$2" services="$3"
    local file="${TVP_CADDY_BOXES}/${name}.caddy"
    cat > "${file}" <<EOF
${name}.${TVP_DOMAIN} {
    reverse_proxy ${ip}:80
}
EOF
    # per-service UI subdomains (mailpit etc.)
    for s in ${services}; do
        local f; f="$(tvp_find_plugin service "${s}")" || continue
        unset TVP_NAME TVP_UI_PORT TVP_UI_SUBDOMAIN
        # shellcheck disable=SC1090
        source "${f}"
        if [ -n "${TVP_UI_PORT}" ]; then
            local sub="${TVP_UI_SUBDOMAIN:-${s}}"
            cat >> "${file}" <<EOF
${sub}.${name}.${TVP_DOMAIN} {
    reverse_proxy ${ip}:${TVP_UI_PORT}
}
EOF
        fi
    done
    caddy reload --config "${TVP_CADDY_DIR}/Caddyfile" 2>/dev/null || true
}

tvp_caddy_remove_box() {
    rm -f "${TVP_CADDY_BOXES}/${1}.caddy"
    caddy reload --config "${TVP_CADDY_DIR}/Caddyfile" 2>/dev/null || true
}
