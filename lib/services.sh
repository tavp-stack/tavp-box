# ============================================================
# tavpbox — service & stack plugin loader
# ============================================================
# Service/stack plugins are bash files that export:
#   TVP_NAME, TVP_DESC, TVP_CATEGORY, TVP_PORTS, TVP_UI_PORT
#   TVP_INSTALL_apt / _apk / _dnf / _zypper / _pacman / _xbps
# (install commands as a string for the matching pkg manager)

TVP_SERVICES_DIR="${TVP_STATE}/services"
TVP_STACKS_DIR="${TVP_STATE}/stacks"

# Find a plugin file by name across bundled + user dirs
tvp_find_plugin() {
    local kind="$1" name="$2"
    local dir="$2"
    case "${kind}" in
        service) dir="${TVP_SERVICES_DIR}/${name}.tavp.sh" ;;
        stack)   dir="${TVP_STACKS_DIR}/${name}.tavp.sh" ;;
    esac
    if [ -f "${dir}" ]; then echo "${dir}"; return; fi
    # bundled fallback (relative to lib)
    local bundled="$(cd "${TVP_LIB}/.." && pwd)/${kind}s/${name}.tavp.sh"
    if [ -f "${bundled}" ]; then echo "${bundled}"; return; fi
    return 1
}

# Apply a service inside a box
tvp_apply_service() {
    local box="$1" svc="$2" distro="$3"
    local f; f="$(tvp_find_plugin service "${svc}")" || { tvp_warn "service '${svc}' tidak ada, lewati"; return; }
    unset TVP_NAME TVP_DESC TVP_CATEGORY TVP_PORTS TVP_UI_PORT TVP_INSTALL_apt TVP_INSTALL_apk TVP_INSTALL_dnf TVP_INSTALL_zypper TVP_INSTALL_pacman TVP_INSTALL_xbps
    # shellcheck disable=SC1090
    source "${f}"
    local pkg; pkg="$(tvp_distro_pkgmgr "${distro}")"
    local var="TVP_INSTALL_${pkg}"
    local script="${!var:-}"
    [ -z "${script}" ] && { tvp_warn "service '${svc}' belum dukung ${distro}"; return; }
    tvp_spin "install ${svc} (${distro})" tvp_box_exec "${box}" "${script}"
    tvp_ok "${svc} aktif di ${box}"
}

# Apply a stack (php/webserver + optional phalcon)
tvp_apply_stack() {
    local box="$1" stack="$2" phalcon="$3" distro="$4"
    local f; f="$(tvp_find_plugin stack "${stack}")" || { tvp_warn "stack '${stack}' tidak ada, pakai blank"; stack="blank"; f="$(tvp_find_plugin stack blank)"; }
    unset TVP_NAME TVP_DESC TVP_INSTALL_apt TVP_INSTALL_apk TVP_INSTALL_dnf TVP_INSTALL_zypper TVP_INSTALL_pacman TVP_INSTALL_xbps
    # shellcheck disable=SC1090
    source "${f}"
    local pkg; pkg="$(tvp_distro_pkgmgr "${distro}")"
    local var="TVP_INSTALL_${pkg}"
    local script="${!var:-}"
    [ -n "${script}" ] && tvp_spin "install stack ${stack}" tvp_box_exec "${box}" "${script}"

    # Phalcon (optional, TAVP stack)
    if [ -n "${phalcon}" ] && [ "${stack}" = "tavp" ]; then
        tvp_spin "bake Phalcon ${phalcon}" tvp_box_exec "${box}" "$(tvp_phalcon_script "${phalcon}" "${pkg}")"
        tvp_ok "Phalcon ${phalcon} di-bake ke ${box}"
    fi
}

# Phalcon install script (pecl preferred, git build fallback) per pkgmgr
tvp_phalcon_script() {
    local ver="$1" pkg="$2"
    case "${pkg}" in
        apt) echo "apt-get update && apt-get install -y php-dev php-pear build-essential && pecl install phalcon-${ver} && echo 'extension=phalcon.so' > /etc/php/*/cli/conf.d/30-phalcon.ini" ;;
        apk) echo "apk add --no-cache php83-dev pcre-dev re2c linux-headers && pecl install phalcon-${ver} && echo 'extension=phalcon.so' > /etc/php83/conf.d/30-phalcon.ini" ;;
        dnf) echo "dnf install -y php-devel php-pear gcc re2c pcre-devel && pecl install phalcon-${ver} && echo 'extension=phalcon.so' > /etc/php.d/30-phalcon.ini" ;;
        *)   echo "echo 'Phalcon manual install needed for ${pkg}'" ;;
    esac
}

# Minimal parser for tavpbox.yml (flat keys + services list)
tvp_load_yaml() {
    local file="$1"
    TVP_YML_name=""; TVP_YML_stack=""; TVP_YML_phalcon=""; TVP_YML_path=""; TVP_YML_services=""
    local in_services=0
    while IFS= read -r line; do
        if [ "${in_services}" -eq 1 ]; then
            [[ "${line}" =~ ^[[:space:]]*-[[:space:]]*(.+)$ ]] && TVP_YML_services+=" ${BASH_REMATCH[1]}"
            [[ "${line}" =~ ^[a-zA-Z] ]] && in_services=0
        fi
        case "${line}" in
            name:*)   TVP_YML_name="${line#name:}"; TVP_YML_name="${TVP_YML_name# }" ;;
            stack:*)  TVP_YML_stack="${line#stack:}"; TVP_YML_stack="${TVP_YML_stack# }" ;;
            phalcon:*)TVP_YML_phalcon="${line#phalcon:}"; TVP_YML_phalcon="${TVP_YML_phalcon# }" ;;
            path:*)   TVP_YML_path="${line#path:}"; TVP_YML_path="${TVP_YML_path# }" ;;
            services:) in_services=1 ;;
        esac
    done < "${file}"
}
