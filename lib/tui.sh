# ============================================================
# tavpbox — TUI helpers (whiptail if present, else plain menu)
# ============================================================

TVP_USE_TUI=0
if command -v whiptail >/dev/null 2>&1; then
    TVP_USE_TUI=1
fi

# Plain read-line fallback
_tui_plain_select() {
    local title="$1"; shift
    tvp_bold "${title}"
    local i=1 opts=("$@")
    for o in "${opts[@]}"; do
        printf "  %s) %s\n" "${i}" "${o}"
        i=$((i+1))
    done
    local pick
    read -r -p "Pilih [1-${#opts[@]}]: " pick
    local sel="${opts[$((pick-1))]}"
    echo "${sel%% *}"   # return only the key (first word)
}

_tui_plain_check() {
    local title="$1"; shift
    tvp_bold "${title} (ketik nomor dipisah spasi, kosong = none)"
    local i=1 opts=("$@")
    for o in "${opts[@]}"; do
        printf "  [%s] %s\n" "${i}" "${o}"
        i=$((i+1))
    done
    local picks
    read -r -p "Pilih: " picks
    local out=()
    for p in ${picks}; do
        out+=("${opts[$((p-1))]%% *}")   # return only the key
    done
    echo "${out[*]}"
}

# tvp_tui_select "Title" "a" "b" "c"  -> echoes chosen value
tvp_tui_select() {
    local title="$1"; shift
    if [ "${TVP_USE_TUI}" -eq 1 ]; then
        local i=0 opts=()
        for o in "$@"; do opts+=("${o}" ""); done
        whiptail --title "tavpbox" --menu "${title}" 20 70 10 "${opts[@]}" 3>&1 1>&2 2>&3
    else
        _tui_plain_select "${title}" "$@"
    fi
}

# tvp_tui_check "Title" "a" "b" -> echoes space-separated chosen
tvp_tui_check() {
    local title="$1"; shift
    if [ "${TVP_USE_TUI}" -eq 1 ]; then
        local opts=()
        for o in "$@"; do opts+=("${o}" "" "OFF"); done
        whiptail --title "tavpbox" --checklist "${title}" 20 70 12 "${opts[@]}" 3>&1 1>&2 2>&3 \
            | tr -d '"'
    else
        _tui_plain_check "${title}" "$@"
    fi
}

# tvp_tui_input "Prompt" "default" -> echoes value
tvp_tui_input() {
    local prompt="$1"; local def="$2"
    if [ "${TVP_USE_TUI}" -eq 1 ]; then
        whiptail --title "tavpbox" --inputbox "${prompt}" 10 70 "${def}" 3>&1 1>&2 2>&3
    else
        local v
        read -r -p "${prompt} [${def}]: " v
        echo "${v:-${def}}"
    fi
}

# tvp_tui_confirm "Prompt" -> 0 yes / 1 no
tvp_tui_confirm() {
    local prompt="$1"
    if [ "${TVP_USE_TUI}" -eq 1 ]; then
        whiptail --title "tavpbox" --yesno "${prompt}" 10 70
    else
        local v
        read -r -p "${prompt} (y/n): " v
        [[ "${v}" =~ ^[Yy] ]]
    fi
}
