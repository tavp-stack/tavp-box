# ============================================================
# tavpbox — common helpers (logging, paths, spinners)
# ============================================================

TVP_RESET='\033[0m'; TVP_RED='\033[31m'; TVP_GREEN='\033[32m'
TVP_YELLOW='\033[33m'; TVP_BLUE='\033[34m'; TVP_BOLD='\033[1m'

tvp_log()  { printf "${TVP_BLUE}==>${TVP_RESET} %s\n" "$*"; }
tvp_ok()   { printf "${TVP_GREEN}✓${TVP_RESET} %s\n" "$*"; }
tvp_warn() { printf "${TVP_YELLOW}!${TVP_RESET} %s\n" "$*"; }
tvp_error(){ printf "${TVP_RED}✗${TVP_RESET} %s\n" "$*" >&2; }

tvp_bold() { printf "${TVP_BOLD}%s${TVP_RESET}\n" "$*"; }

# Numbered step header, like install-phalcon.sh
TVP_STEP=0
tvp_step() {
    TVP_STEP=$(( TVP_STEP + 1 ))
    printf "\n${TVP_BLUE}==>${TVP_RESET} [%s] %s\n" "${TVP_STEP}" "$*"
}

# Spinner wrapper: tvp_spin "Installing..." cmd args...
tvp_spin() {
    local msg="$1"; shift
    local logf start pid rc spin='|/-\' i=0
    logf="$(mktemp)"
    start="$(date +%s)"
    ("$@") >"${logf}" 2>&1 &
    pid=$!
    while kill -0 "${pid}" 2>/dev/null; do
        i=$(( (i+1) % 4 ))
        printf "\r    ${spin:$i:1} %s  [%ss]" "${msg}" "$(( $(date +%s) - start ))"
        sleep 0.15
    done
    if wait "${pid}"; then rc=0; else rc=$?; fi
    if [ "${rc}" -eq 0 ]; then
        printf "\r    ${TVP_GREEN}✓${TVP_RESET} %s  [%ss]    \n" "${msg}" "$(( $(date +%s) - start ))"
        rm -f "${logf}"
    else
        printf "\r    ${TVP_RED}✗${TVP_RESET} %s  [%ss]    \n" "${msg}" "$(( $(date +%s) - start ))"
        tail -25 "${logf}" >&2
        rm -f "${logf}"
        return "${rc}"
    fi
}

tvp_usage() {
    cat <<'EOF'

TAVP Box — dev environment ala Lando, tanpa Docker (LXC-based, irit RAM)

Penggunaan:
  tavpbox init                 Setup pertama (TUI): pilih distro, domain, limit
  tavpbox create               Buat box baru (TUI) atau:
  tavpbox create --from f.yml  Buat box dari file config
  tavpbox start <nama>         Nyalakan box
  tavpbox stop <nama>          Matikan box (RAM balik 0)
  tavpbox rebuild <nama>       Recreate container, data tetap
  tavpbox list                 Lihat semua box
  tavpbox mail <nama>          Buka mail.<nama>.tavp.local
  tavpbox ssh <nama>           Masuk terminal box
  tavpbox destroy <nama>       Hapus box
  tavpbox snapshot <nama>      Snapshot box (backup)
  tavpbox help                 Tampilkan bantuan ini

Contoh:
  tavpbox init
  tavpbox create               # pilih: project1, stack=tavp, phalcon=5.16
  tavpbox start project1
  # buka: http://project1.tavp.local
  # mail: http://mail.project1.tavp.local
EOF
}
