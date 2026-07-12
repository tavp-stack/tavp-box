# TAVP Box

**Dev environment all-in-one ala Lando, tapi tanpa Docker** — berbasis **LXC**,
jadi irit RAM dan **Phalcon tidak perlu di-install ulang tiap restart**.

```
1 laptop = banyak "VPS mini". Tiap project = 1 box terisolasi.
```

| | Lando (Docker) | TAVP Box (LXC) |
|---|---|---|
| RAM / 20 project | ~40 GB | ~700 MB (Windows) · ~600 MB (Linux) |
| Phalcon reinstall? | Sering | Sekali (di-bake ke box) |
| Auto domain | `*.lndo.site` | `*.tavp.local` |
| Mail per-project | `mail.*.lndo.site` | `mail.*.tavp.local` |
| Multi-distro | ✗ | ✓ (Ubuntu/Alpine/Debian/...) |
| Multi-stack | ✓ | ✓ (TAVP/Laravel/Python/Node/Go/...) |
| Production | ✗ (dev doang) | ✓ (banyak site di 1 VPS) |

---

## 0. Prasyarat

- **Linux**: native distro (Ubuntu/Debian/Fedora/Arch/...).
- **Windows**: Windows 10/11 64-bit, bisa enable WSL2.
- **macOS**: Homebrew terpasang.
- Lo **tidak perlu** paham nginx/docker/LXC — cukup jalankan perintah.

---

## 1. Install

### Linux (native)
```bash
sudo bash install/install-linux.sh
```

### Windows (WSL2)
Buka **PowerShell sebagai Administrator**:
```powershell
powershell -ExecutionPolicy Bypass -File install/install-windows.ps1
```
Setelah reboot (kalau WSL baru di-install), jalankan installer itu lagi.

> **Tips:** Setelah terpasang, lo bisa gunakan GUI desktop **TAVP Box Desktop**  
> (unduh di [release tavpbox-desktop](https://github.com/tavp-stack/tavpbox-desktop/releases)).

### macOS (Lima)
```bash
bash install/install-mac.sh
```

Installer akan: pasang LXD, whiptail (TUI), jq, dan salin CLI `tavpbox` ke PATH.

---

## 2. Inisialisasi Pertama (`tavpbox init`)

Jalankan **sekali**. Akan muncul TUI:

```
┌─ tavpbox init ──────────────────────────────┐
│ Pilih base distro:                          │
│   ⮞ Ubuntu 24.04      (default, stabil)     │
│     Alpine 3.20      (paling irit RAM)      │
│     Debian 12 / Fedora / Arch / ...         │
│     Lainnya...      (ketik nama distro)     │
│ Domain suffix: tavp.local                   │
│ RAM max/box: 512MB   CPU: 1                 │
└──────────────────────────────────────────────┘
```

- **Distro**: 10 populer + "lainnya". Default Ubuntu.
- **Domain**: semua box dapat subdomain otomatis (`namabox.tavp.local`).
- **RAM/CPU**: limit per box (bisa diubah di TUI). Cegah 1 box makan semua.

Setelah ini, Caddy + DNS wildcard sudah jalan.

---

## 3. Buat Box Pertama (`tavpbox create`)

Jalankan tanpa argumen → TUI:

```
Nama box     : project1
Stack        : tavp
Phalcon      : 5.16        (kosong = tidak pakai Phalcon)
Folder       : /path/ke/project  (jadi webroot box)
Services     : [✓] mariadb [✓] redis [✓] mailpit [✓] phpmyadmin
```

Atau dari file config (alak `.lando.yml`) — lihat `examples/tavpbox.yml`:
```bash
tavpbox create --from tavpbox.yml
```

`tavpbox` akan:
1. `lxc launch` image distro pilihan
2. pasang limit RAM/CPU
3. **map folder lo** → `/var/www/html` di box (Lando-style)
4. pasang stack (PHP+nginx+composer, atau Python/Node/Go/...)
5. **bake Phalcon** kalau dipilih (sekali, persist)
6. pasang services yang dipilih
7. daftarkan domain + mail ke Caddy/DNS

---

## 4. Jalankan & Akses

```bash
tavpbox start project1
```

Buka di browser:
- **App** : `http://project1.tavp.local`
- **Mail**: `http://mail.project1.tavp.local`  ← OTP/email masuk ke sini, per-project
- **DB UI**: `http://pma.project1.tavp.local` (kalau pilih phpmyadmin)

Perintah harian:
```bash
tavpbox list              # lihat semua box + status
tavpbox stop project1     # matikan → RAM balik 0
tavpbox start project1    # nyalakan lagi (detik, Phalcon tetap ada)
tavpbox rebuild project1  # recreate container, data tetap
tavpbox ssh project1      # masuk terminal box
tavpbox mail project1     # buka mailpit
tavpbox destroy project1  # hapus box
tavpbox snapshot project1 # backup (untuk production)
```

---

## 5. Config File (`tavpbox.yml`)

Simpan di repo project, commit ke Git, tim lo tinggal `tavpbox create --from`:

```yaml
name: project1
stack: tavp            # tavp | laravel | symfony | wordpress | python | node | go | ruby | blank
phalcon: "5.16"       # kosong = tidak install Phalcon
path: /home/user/projects/cms   # folder jadi webroot (/var/www/html di box)

services:
  - mariadb
  - redis
  - mailpit
  - phpmyadmin
```

---

## 6. Service Plugin (tambah sendiri, ala Lando)

Tiap service = 1 file `*.tavp.sh` di `~/.tavpbox/services/`. Contoh
`solr.tavp.sh`:

```bash
TVP_NAME="solr"
TVP_DESC="Apache Solr search"
TVP_CATEGORY="search"
TVP_PORTS=(8983)
TVP_UI_PORT="8983"
TVP_UI_SUBDOMAIN="solr"
TVP_INSTALL_apt='apt-get install -y solr-tomcat && service solr start'
TVP_INSTALL_apk='apk add solr && rc-service solr start'
# ... per distro (dnf / zypper / pacman / xbps)
```

Taruh file → langsung muncul di TUI `create`. **Lando punya ~30 plugin;
tavpbox bisa unlimited** karena definisinya terbuka (bisa Solr, Varnish,
apa aja).

Stack (recipe) sama konsepnya, ada di `~/.tavpbox/stacks/`.

Service bawaan: `mariadb mysql postgres mongodb redis memcached
elasticsearch solr varnish mailpit mailhog phpmyadmin adminer nginx apache`.

Stack bawaan: `tavp laravel symfony wordpress python node go ruby blank`.

---

## 7. Production & TAVP Cloud

`tavpbox` juga dipakai di **VPS production**: banyak website, 1 VPS, tiap box
terisolasi + resource limit. Untuk **jual hosting ke orang asing** (untrusted
tenant) pakai mode **VM** (LXD `--vm`) — itu evolusi ke **tavp-cloud**.

---

## 8. Struktur Folder

```
tavpbox/
├── bin/tavpbox          CLI utama
├── lib/
│   ├── common.sh        logging + spinner
│   ├── os.sh            deteksi OS + 10 distro + pkg-manager map
│   ├── tui.sh           menu (whiptail / fallback)
│   ├── lxd.sh           lifecycle box (init/create/start/stop/...)
│   ├── services.sh      loader plugin + install per-distro
│   ├── caddy.sh         reverse proxy + wildcard domain
│   └── dns.sh           *.tavp.local -> 127.0.0.1
├── services/            plugin katalog
├── stacks/              recipe
├── install/             installer per OS
├── examples/tavpbox.yml contoh config
├── composer.json        paket tavp/box (Packagist)
└── README.md
```

---

## 9. Troubleshooting

- **Domain tidak resolve di Windows**: IP WSL2 bisa berubah tiap reboot.
  Jalankan lagi `install/install-wsl.ps1` untuk perbarui DNS (NRPT).
- **Caddy gagal start**: pastikan port 80/443 bebas. Cek `journalctl -u caddy`
  atau `/var/log/caddy.log`.
- **dnsmasq bentrok dengan systemd-resolved**: nonaktifkan systemd-resolved
  atau arahkan `/etc/resolv.conf` ke dnsmasq.
- **Folder Windows tidak kelihatan di box**: pastikan path di WSL
  (`/mnt/c/Users/...`), bukan path Windows (`C:\...`).

## Lisensi
MIT
