# ============================================================
# tavpbox — OS detection + installer dispatch
# ============================================================

# Echo one of: linux | wsl | mac | windows(unsupported directly)
tvp_detect_os() {
    local uname="$(uname -s)"
    case "${uname}" in
        Linux*)
            if [ -f /proc/version ] && grep -qi microsoft /proc/version; then
                echo wsl
            else
                echo linux
            fi
            ;;
        Darwin*) echo mac ;;
        *) echo unknown ;;
    esac
}

# Map a distro name -> LXD image alias (images: remote)
tvp_distro_image() {
    case "$1" in
        ubuntu)   echo "ubuntu:24.04" ;;
        debian)   echo "images:debian/12" ;;
        alpine)   echo "images:alpine/3.20" ;;
        fedora)  echo "images:fedora/40" ;;
        arch)    echo "images:archlinux" ;;
        centos)  echo "images:centos/9-Stream" ;;
        rocky)   echo "images:rockylinux/9" ;;
        opensuse) echo "images:opensuse/15.5" ;;
        mint)    echo "images:linuxmint/21" ;;
        manjaro) echo "images:manjaro" ;;
        void)    echo "images:void" ;;
        *)       echo "images:$1" ;;   # custom: pass through
    esac
}

# Map distro -> package-manager tag used by service plugins
tvp_distro_pkgmgr() {
    case "$1" in
        ubuntu|debian|mint) echo apt ;;
        alpine)             echo apk ;;
        fedora|centos|rocky) echo dnf ;;
        opensuse)          echo zypper ;;
        arch|manjaro)      echo pacman ;;
        void)              echo xbps ;;
        *)                 echo apt ;;   # safest fallback
    esac
}

# The 10 curated popular distros (Ubuntu first = default)
TVP_DISTROS=(
    "ubuntu|Ubuntu 24.04|Stabil, LXD resmi"
    "debian|Debian 12|Familiar, apt"
    "alpine|Alpine 3.20|Paling irit RAM"
    "fedora|Fedora 40|Modern, dnf"
    "arch|Arch Linux|Rolling, pacman"
    "centos|CentOS 9|Enterprise-like"
    "rocky|Rocky Linux 9|RHEL clone"
    "opensuse|openSUSE 15.5|zypper"
    "mint|Linux Mint 21|Desktop-friendly"
    "manjaro|Manjaro|Arch-based"
)

# Is LXD installed on this host?
tvp_lxd_present() {
    command -v lxc >/dev/null 2>&1 && command -v lxd >/dev/null 2>&1
}
