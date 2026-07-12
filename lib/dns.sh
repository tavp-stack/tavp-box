# ============================================================
# tavpbox — DNS (wildcard *.domain -> 127.0.0.1 via dnsmasq)
# ============================================================
TVP_DNS_CONF="/etc/dnsmasq.d/tavpbox.conf"

tvp_dns_install() {
    if command -v dnsmasq >/dev/null 2>&1; then return; fi
    if command -v apt-get >/dev/null; then apt-get update && apt-get install -y dnsmasq
    elif command -v apk >/dev/null; then apk add dnsmasq
    elif command -v dnf >/dev/null; then dnf install -y dnsmasq
    elif command -v pacman >/dev/null; then pacman -S --noconfirm dnsmasq
    fi
}

tvp_setup_dns() {
    local domain="$1"
    tvp_dns_install
    echo "address=/.${domain}/127.0.0.1" > "${TVP_DNS_CONF}"
    echo "listen-address=127.0.0.1" >> "${TVP_DNS_CONF}"
    if command -v systemctl >/dev/null 2>&1; then
        systemctl enable dnsmasq 2>/dev/null; systemctl restart dnsmasq 2>/dev/null || true
    elif command -v rc-service >/dev/null 2>&1; then
        rc-service dnsmasq restart 2>/dev/null || true
    else
        pkill dnsmasq 2>/dev/null || true
        nohup dnsmasq -C "${TVP_DNS_CONF}" >/dev/null 2>&1 &
    fi
    tvp_ok "DNS wildcard *.${domain} -> 127.0.0.1"
}

tvp_dns_reload() {
    if command -v systemctl >/dev/null 2>&1; then
        systemctl restart dnsmasq 2>/dev/null || true
    else
        pkill dnsmasq 2>/dev/null || true
        nohup dnsmasq -C "${TVP_DNS_CONF}" >/dev/null 2>&1 &
    fi
}
