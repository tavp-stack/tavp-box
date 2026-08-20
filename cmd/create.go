package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tavp-stack/tavpbox/internal/config"
	"github.com/tavp-stack/tavpbox/internal/podman"
	"github.com/tavp-stack/tavpbox/internal/proxy"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create and start a project container",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, cfg, err := config.FindProject()
		if err != nil {
			return fmt.Errorf(".tavpbox.yml not found. Run: tavpbox init")
		}

		globalCfg, _ := config.LoadGlobal()
		client := podman.New()

		// Ensure Podman machine is running
		if err := client.EnsureRunning(); err != nil {
			return fmt.Errorf("podman: %w", err)
		}

		cname := client.ContainerName(cfg.Name)

		// Assign LAN port
		lanMgr := proxy.NewLanPortManager()
		lanPort, err := lanMgr.GetOrAssign(cfg.Name)
		if err != nil {
			return fmt.Errorf("assign LAN port: %w", err)
		}

		fmt.Printf("Creating box '%s' (%s recipe)...\n", cfg.Name, cfg.Recipe)

		image := getImage(cfg, globalCfg)
		ports := getPorts(cfg, lanPort)

		env := make(map[string]string)
		env["APP_ENV"] = "local"
		env["TZ"] = "Asia/Jakarta"
		if cfg.TZ != "" {
			env["TZ"] = cfg.TZ
		}
		for k, v := range cfg.Env {
			env[k] = v
		}

		// Always mount the FULL project directory to /var/www/html so that
		// sibling dirs (lib/, app/, vendor/, config.ini) are visible to PHP
		// and so container creation never fails when the webroot subdir does not
		// exist yet. The webroot subdir is only used for the nginx `root`
		// directive (see installPHPServer / installLaravel). Fixes #17 and the
		// autoload class of bugs (#35).
		projectRoot, _ := os.Getwd()
		absRoot, _ := filepath.Abs(projectRoot)
		volumes := []string{
			fmt.Sprintf("%s:/var/www/html", absRoot),
		}

		// Auto-volume for database persistence
		if cfg.Services["mariadb"].Enabled || cfg.Services["mysql"].Enabled {
			dbVolumeDir := filepath.Join(config.HomeDir(), "volumes", cfg.Name, "mysql")
			os.MkdirAll(dbVolumeDir, 0755)
			volumes = append(volumes, fmt.Sprintf("%s:/var/lib/mysql", dbVolumeDir))
			fmt.Printf("  DB volume: %s\n", dbVolumeDir)
		}
		if cfg.Services["postgres"].Enabled {
			pgVolumeDir := filepath.Join(config.HomeDir(), "volumes", cfg.Name, "postgres")
			os.MkdirAll(pgVolumeDir, 0755)
			volumes = append(volumes, fmt.Sprintf("%s:/var/lib/postgresql/data", pgVolumeDir))
			fmt.Printf("  DB volume: %s\n", pgVolumeDir)
		}

		domain := cfg.Name + "." + globalCfg.DomainSuffix

		// 1. Pull image
		fmt.Printf("  [1/4] Pulling image %s...\n", image)
		if err := client.Pull(image); err != nil {
			return fmt.Errorf("pull image: %w", err)
		}

		// 2. Create container
		fmt.Printf("  [2/4] Creating container...\n")
		client.Remove(cname)
		if err := client.Create(cname, image, ports, env, volumes, map[string]string{}); err != nil {
			return fmt.Errorf("create container: %w", err)
		}

		// 3. Start container
		fmt.Printf("  [3/4] Starting container...\n")
		if err := client.Start(cname); err != nil {
			return fmt.Errorf("start container: %w", err)
		}
		time.Sleep(2 * time.Second)

		// 4. Install recipe + services
		fmt.Printf("  [4/4] Installing %s recipe...\n", cfg.Recipe)
		if err := installRecipe(client, cname, cfg); err != nil {
			fmt.Printf("  ⚠ Recipe install warning: %v\n", err)
		}

		for svcName, svcCfg := range cfg.Services {
			if !svcCfg.Enabled {
				continue
			}
			fmt.Printf("  Installing %s...\n", svcName)
			if err := installService(client, cname, svcName); err != nil {
				fmt.Printf("  ⚠ %s install warning: %v\n", svcName, err)
			}
		}

		// Create startup script for auto-restart
		fmt.Printf("  Creating startup script...\n")
		startupScript := buildStartupScript(cfg)
		client.Exec(cname, "bash", "-c", fmt.Sprintf("cat > /usr/local/bin/tavpbox-startup.sh << 'STARTEOF'\n%s\nSTARTEOF\nchmod +x /usr/local/bin/tavpbox-startup.sh", startupScript))

		// Run startup script in background (not blocking)
		client.Exec(cname, "bash", "-c", "nohup /usr/local/bin/tavpbox-startup.sh > /var/log/tavpbox-startup.log 2>&1 &")

		// Configure nginx locations for admin services (phpmyadmin, adminer)
		configureAdminServices(client, cname, cfg)

		// Execute Lando build/run commands if present
		if buildCmds, ok := cfg.Env["LANDO_BUILD_CMDS"]; ok && buildCmds != "" {
			fmt.Printf("  Running build commands...\n")
			if _, err := client.Exec(cname, "bash", "-c", buildCmds); err != nil {
				fmt.Printf("  ⚠ Build commands warning: %v\n", err)
			}
		}

		// Execute events.post-start commands
		if len(cfg.Events.PostStart) > 0 {
			fmt.Printf("  Running post-start events...\n")
			for _, eventCmd := range cfg.Events.PostStart {
				if _, err := client.Exec(cname, "bash", "-c", eventCmd); err != nil {
					fmt.Printf("  ⚠ Event command warning: %v\n", err)
				}
			}
		}

		// Get container IP and host port
		ip, _ := client.GetIP(cname)
		hostPort := client.GetHostPort(cname, "80")

		// Ensure proxy is running before adding routes
		ensureProxyRunning()

		// Add proxy route for domain access
		p := proxy.New(80)
		p.AddRoute(domain, "127.0.0.1", hostPort)

		// Add routes for services (single-level subdomains to match *.tavp.my.id cert)
		if cfg.Services["mailpit"].Enabled || cfg.Services["mailhog"].Enabled {
			mailpitPort := client.GetHostPort(cname, "8025")
			if mailpitPort > 0 {
				p.AddRoute(cfg.Name+"-mailpit."+globalCfg.DomainSuffix, "127.0.0.1", mailpitPort)
			}
		}
		if cfg.Services["adminer"].Enabled {
			adminerPort := client.GetHostPort(cname, "8081")
			if adminerPort > 0 {
				p.AddRoute(cfg.Name+"-adminer."+globalCfg.DomainSuffix, "127.0.0.1", adminerPort)
			}
		}
		if cfg.Services["phpmyadmin"].Enabled {
			phpmyadminPort := client.GetHostPort(cname, "8080")
			if phpmyadminPort > 0 {
				p.AddRoute(cfg.Name+"-phpmyadmin."+globalCfg.DomainSuffix, "127.0.0.1", phpmyadminPort)
			}
		}

		// Proxy auto-detects route changes via file watcher — no restart needed

		fmt.Printf("\n✓ Box '%s' created and running!\n", cfg.Name)
		fmt.Printf("  Direct:  http://localhost:%d\n", hostPort)
		fmt.Printf("  HTTP:    http://%s\n", domain)
		fmt.Printf("  LAN:     http://%s:%d\n", proxy.GetHostIP(), lanPort)
		if cfg.Services["mailpit"].Enabled || cfg.Services["mailhog"].Enabled {
			fmt.Printf("  Mailpit: http://%s-mailpit.%s\n", cfg.Name, globalCfg.DomainSuffix)
		}
		if cfg.Services["adminer"].Enabled {
			fmt.Printf("  Adminer: http://%s-adminer.%s\n", cfg.Name, globalCfg.DomainSuffix)
		}
		if cfg.Services["phpmyadmin"].Enabled {
			fmt.Printf("  phpMyAdmin: http://%s-phpmyadmin.%s\n", cfg.Name, globalCfg.DomainSuffix)
		}
		if ip != "" {
			fmt.Printf("  IP:      %s\n", ip)
		}
		fmt.Printf("  SSH:     tavpbox ssh\n")

		return nil
	},
}
func getImage(cfg *config.ProjectConfig, globalCfg *config.GlobalConfig) string {
	if cfg.Image != "" {
		return cfg.Image
	}

	switch cfg.Recipe {
	case "tavp", "php", "laravel":
		return "ghcr.io/tavp-stack/tavpbox-php:latest"
	case "node":
		return "ghcr.io/tavp-stack/tavpbox-node:latest"
	case "go":
		return "ghcr.io/tavp-stack/tavpbox-go:latest"
	case "python":
		return "ghcr.io/tavp-stack/tavpbox-python:latest"
	default:
		if globalCfg.DefaultImage != "" {
			return globalCfg.DefaultImage
		}
		return "docker.io/library/ubuntu:24.04"
	}
}

func getPorts(cfg *config.ProjectConfig, lanPort int) []string {
	var ports []string

	// Use fixed LAN port for web (e.g., 0.0.0.0:8081:80)
	if lanPort > 0 {
		ports = append(ports, fmt.Sprintf("0.0.0.0:%d:80", lanPort))
	} else {
		ports = append(ports, "80")
	}

	for svcName, svcCfg := range cfg.Services {
		if !svcCfg.Enabled {
			continue
		}
		switch svcName {
		case "mailpit":
			ports = append(ports, "8025", "1025")
		case "phpmyadmin":
			ports = append(ports, "8080")
		case "adminer":
			ports = append(ports, "8081")
		}
	}

	return ports
}

func installRecipe(client *podman.Client, cname string, cfg *config.ProjectConfig) error {
	switch cfg.Recipe {
	case "tavp", "php":
		return installPHPServer(client, cname, cfg)
	case "laravel":
		return installLaravel(client, cname, cfg)
	case "node":
		return installNode(client, cname, cfg)
	case "go":
		return installGo(client, cname, cfg)
	case "python":
		return installPython(client, cname, cfg)
	default:
		return nil
	}
}

func installPHPServer(client *podman.Client, cname string, cfg *config.ProjectConfig) error {
	// Full project is mounted at /var/www/html. Serve from the webroot subdir
	// when set (e.g. "public"), otherwise from the project root.
	webroot := strings.Trim(cfg.Webroot, "/")
	nginxRoot := "/var/www/html"
	if webroot != "" && webroot != "." {
		nginxRoot = "/var/www/html/" + webroot
	}

	// Nginx config content (written via podman cp to avoid shell escaping issues with $)
	nginxCfg := fmt.Sprintf(`server {
    listen 80 default_server;
    root %s;
    index index.php index.html;
    location / { try_files $uri $uri/ /index.php?$query_string; }
    location ~ \.php$ {
        fastcgi_pass 127.0.0.1:9000;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }
    location ~ /\.ht { deny all; }
}
`, nginxRoot)

	// Check if packages are already installed (pre-built image)
	_, err := client.Exec(cname, "bash", "-c", "command -v nginx && command -v php-fpm")
	if err == nil {
		// Already installed, just configure and start
		// Only install phalcon if missing (pre-built image already has it) — avoids 10-15min compile hang (#11)
		client.Exec(cname, "bash", "-c", "php -m | grep -qi phalcon || (pecl install phalcon 2>/dev/null && echo 'extension=phalcon.so' > /usr/local/etc/php/conf.d/phalcon.ini) || true")

		// Create storage symlinks for Laravel/TAVP
		client.Exec(cname, "bash", "-c", "mkdir -p /run/php /tmp/storage/framework/views /tmp/storage/framework/cache /tmp/storage/framework/sessions /tmp/bootstrap-cache && rm -rf /var/www/html/storage /var/www/html/bootstrap/cache 2>/dev/null || true && ln -sf /tmp/storage /var/www/html/storage && ln -sf /tmp/bootstrap-cache /var/www/html/bootstrap/cache")

		// Write nginx config via podman cp (avoids $ escaping issues)
		if err := writeNginxConfig(client, cname, nginxCfg); err != nil {
			return err
		}

		client.Exec(cname, "bash", "-c", "php-fpm & nginx 2>/dev/null || true")
		return nil
	}

	// Not pre-built, install from scratch
	_, err = client.Exec(cname, "bash", "-c", `
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends nginx php8.3-fpm php8.3-cli php8.3-mbstring php8.3-xml php8.3-curl php8.3-zip php8.3-bcmath php8.3-intl php8.3-mysql php8.3-gd composer curl wget git unzip
apt-get install -y -qq --no-install-recommends php-pear php8.3-dev gcc make
pecl channel-update pecl.php.net 2>/dev/null
pecl install phalcon 2>/dev/null || true
echo "extension=phalcon.so" > /etc/php/8.3/mods-available/phalcon.ini
phpenmod phalcon 2>/dev/null || true
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y -qq --no-install-recommends nodejs`)
	if err != nil {
		return err
	}

	// Write nginx config
	if err := writeNginxConfig(client, cname, nginxCfg); err != nil {
		return err
	}

	_, err = client.Exec(cname, "bash", "-c", "service php8.3-fpm start 2>/dev/null; service nginx start 2>/dev/null")
	return err
}

func installLaravel(client *podman.Client, cname string, cfg *config.ProjectConfig) error {
	// Full project mounted at /var/www/html; serve from webroot subdir (e.g. public).
	webroot := strings.Trim(cfg.Webroot, "/")
	nginxRoot := "/var/www/html"
	if webroot != "" && webroot != "." {
		nginxRoot = "/var/www/html/" + webroot
	}
	nginxCfg := fmt.Sprintf(`server {
    listen 80 default_server;
    root %s;
    index index.php;
    location / { try_files $uri $uri/ /index.php?$query_string; }
    location ~ \.php$ {
        fastcgi_pass 127.0.0.1:9000;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }
    location ~ /\.ht { deny all; }
}
`, nginxRoot)

	// Check if packages are already installed (pre-built image)
	_, err := client.Exec(cname, "bash", "-c", "command -v nginx && command -v php-fpm")
	if err == nil {
		client.Exec(cname, "bash", "-c", "mkdir -p /run/php")
		if err := writeNginxConfig(client, cname, nginxCfg); err != nil {
			return err
		}
		client.Exec(cname, "bash", "-c", "php-fpm & nginx 2>/dev/null || true")
		return nil
	}

	// Not pre-built, install from scratch
	_, err = client.Exec(cname, "bash", "-c", `
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends nginx php8.3-fpm php8.3-cli php8.3-mbstring php8.3-xml php8.3-curl php8.3-zip php8.3-bcmath php8.3-intl php8.3-mysql php8.3-gd composer curl wget git unzip
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y -qq --no-install-recommends nodejs`)
	if err != nil {
		return err
	}

	if err := writeNginxConfig(client, cname, nginxCfg); err != nil {
		return err
	}
	_, err = client.Exec(cname, "bash", "-c", "service php8.3-fpm start 2>/dev/null; service nginx start 2>/dev/null")
	return err
}

func installNode(client *podman.Client, cname string, cfg *config.ProjectConfig) error {
	nginxCfg := `server {
    listen 80;
    root /var/www/html;
    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
    }
}
`

	// Check if packages are already installed (pre-built image)
	_, err := client.Exec(cname, "bash", "-c", "command -v nginx && command -v node")
	if err == nil {
		if err := writeNginxConfig(client, cname, nginxCfg); err != nil {
			return err
		}
		client.Exec(cname, "bash", "-c", "nginx 2>/dev/null || true")
		return nil
	}

	// Not pre-built, install from scratch
	_, err = client.Exec(cname, "bash", "-c", `
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends nginx
npm install -g yarn pnpm`)
	if err != nil {
		return err
	}

	if err := writeNginxConfig(client, cname, nginxCfg); err != nil {
		return err
	}
	_, err = client.Exec(cname, "bash", "-c", "systemctl start nginx 2>/dev/null || service nginx start")
	return err
}

func installGo(client *podman.Client, cname string, cfg *config.ProjectConfig) error {
	_, err := client.Exec(cname, "bash", "-c", `apt-get update -qq && apt-get install -y -qq nginx curl`)
	return err
}

func installPython(client *podman.Client, cname string, cfg *config.ProjectConfig) error {
	nginxCfg := `server {
    listen 80;
    root /var/www/html;
    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host $host;
    }
}
`

	_, err := client.Exec(cname, "bash", "-c", `
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends nginx python3 python3-pip python3-venv curl`)
	if err != nil {
		return err
	}

	if err := writeNginxConfig(client, cname, nginxCfg); err != nil {
		return err
	}
	_, err = client.Exec(cname, "bash", "-c", "systemctl start nginx 2>/dev/null || service nginx start")
	return err
}

func installService(client *podman.Client, cname, svcName string) error {
	// Check if service is already installed (pre-built image)
	checkCmd := map[string]string{
		"mariadb": "command -v mysqld",
		"mysql":   "command -v mysqld",
		"redis":   "command -v redis-server",
		"mailpit": "test -f /usr/local/bin/mailpit",
	}
	if cmd, ok := checkCmd[svcName]; ok {
		if _, err := client.Exec(cname, "bash", "-c", cmd); err == nil {
			return nil // already installed
		}
	}

	scripts := map[string]string{
		"mariadb": `export DEBIAN_FRONTEND=noninteractive
apt-get install -y -qq mariadb-server mariadb-client 2>/dev/null
mkdir -p /run/mysqld && chown mysql:mysql /run/mysqld
if [ ! -f /var/lib/mysql/ibdata1 ]; then
    mariadb-install-db --user=mysql --datadir=/var/lib/mysql 2>/dev/null || true
    chown -R mysql:mysql /var/lib/mysql
fi
mariadbd --user=mysql --datadir=/var/lib/mysql --socket=/run/mysqld/mysqld.sock --pid-file=/run/mysqld/mysqld.pid &
sleep 3
mariadb -u root -e "CREATE DATABASE IF NOT EXISTS app; CREATE USER IF NOT EXISTS 'app'@'localhost' IDENTIFIED BY 'app'; GRANT ALL ON app.* TO 'app'@'localhost'; FLUSH PRIVILEGES;" 2>/dev/null || true`,
		"mysql": `export DEBIAN_FRONTEND=noninteractive
apt-get install -y -qq mysql-server mysql-client 2>/dev/null
mkdir -p /run/mysqld && chown mysql:mysql /run/mysqld
mysqld --user=mysql &
sleep 3`,
		"postgres": `export DEBIAN_FRONTEND=noninteractive
apt-get install -y -qq postgresql postgresql-client 2>/dev/null
su - postgres -c "pg_ctlcluster $(pg_lsclusters -h | head -1 | awk '{print $1, $2}') start" 2>/dev/null || true
su - postgres -c "psql -c \"CREATE USER app WITH PASSWORD 'app' CREATEDB;\"" 2>/dev/null || true
su - postgres -c "psql -c \"CREATE DATABASE app OWNER app;\"" 2>/dev/null || true`,
		"redis": `export DEBIAN_FRONTEND=noninteractive
apt-get install -y -qq redis-server 2>/dev/null
redis-server --daemonize yes 2>/dev/null || true`,
		"mailpit": `curl -sL https://github.com/axllent/mailpit/releases/latest/download/mailpit_linux_amd64.tar.gz | tar xz -C /usr/local/bin/
nohup /usr/local/bin/mailpit --listen 0.0.0.0:8025 --smtp 0.0.0.0:1025 > /var/log/mailpit.log 2>&1 &`,
		"adminer": `mkdir -p /var/www/html/adminer
curl -sL https://www.adminer.org/latest.php -o /var/www/html/adminer/index.php
curl -sL https://www.adminer.org/download/v5.4.4/designs/haeckel/adminer.css -o /var/www/html/adminer/adminer.css
chmod 644 /var/www/html/adminer/index.php /var/www/html/adminer/adminer.css`,
		"phpmyadmin": `export DEBIAN_FRONTEND=noninteractive
apt-get install -y -qq phpmyadmin 2>/dev/null
# Symlink ke webroot yang benar (bisa /var/www/html/public/pma untuk Laravel)
mkdir -p /var/www/html/public
ln -sf /usr/share/phpmyadmin /var/www/html/public/pma 2>/dev/null || true
ln -sf /usr/share/phpmyadmin /var/www/html/pma 2>/dev/null || true
# Prevent phpMyAdmin "should not be world writable" error
chmod 0644 /usr/share/phpmyadmin/config.inc.php 2>/dev/null || true`,
	}

	if script, ok := scripts[svcName]; ok {
		_, err := client.Exec(cname, "bash", "-c", script)
		return err
	}
	return nil
}

// writeNginxConfig writes an nginx config to a temp file and copies it into the container.
// dest is the container path, e.g. "/etc/nginx/sites-available/default"
func writeNginxConfigTo(client *podman.Client, cname, config, dest string) error {
	tmpFile := filepath.Join(os.TempDir(), "tavpbox-nginx-"+cname+".conf")
	if err := os.WriteFile(tmpFile, []byte(config), 0644); err != nil {
		return err
	}
	defer os.Remove(tmpFile)
	return client.Copy(tmpFile, dest, cname)
}

// writeNginxConfig writes the default nginx config into the container.
func writeNginxConfig(client *podman.Client, cname, config string) error {
	return writeNginxConfigTo(client, cname, config, "/etc/nginx/sites-available/default")
}

// buildStartupScript creates a script that starts all installed services
func buildStartupScript(cfg *config.ProjectConfig) string {
	script := `#!/bin/bash
# TAVPBox auto-start services

# Start MariaDB/MySQL if installed
if command -v mysqld &> /dev/null; then
    mkdir -p /run/mysqld && chown mysql:mysql /run/mysqld
    mysqld --user=mysql --datadir=/var/lib/mysql &
    sleep 2
fi

# Start Redis if installed
if command -v redis-server &> /dev/null; then
    redis-server --daemonize yes >/dev/null 2>&1
fi

# Start PHP-FPM if installed
if command -v php-fpm8.3 &> /dev/null; then
    php-fpm8.3 --daemonize >/dev/null 2>&1
elif command -v php-fpm &> /dev/null; then
    php-fpm --daemonize >/dev/null 2>&1
fi
sleep 1

# Start Nginx (retry if fails)
if command -v nginx &> /dev/null; then
    for i in 1 2 3; do
        nginx >/dev/null 2>&1 && break
        sleep 1
    done
fi

# Start Mailpit if installed
if [ -f /usr/local/bin/mailpit ]; then
    nohup /usr/local/bin/mailpit --listen 0.0.0.0:8025 --smtp 0.0.0.0:1025 > /var/log/mailpit.log 2>&1 &
fi

# Health check - restart dead services (use exec to properly handle PID 1)
exec sh -c '
while true; do
    sleep 10
    if [ -f /usr/local/bin/mailpit ] && ! pgrep -x mailpit >/dev/null 2>&1; then
        nohup /usr/local/bin/mailpit --listen 0.0.0.0:8025 --smtp 0.0.0.0:1025 > /var/log/mailpit.log 2>&1 &
    fi
    if ! pgrep nginx >/dev/null 2>&1; then
        nginx >/dev/null 2>&1
    fi
done
'
`
	return script
}

// configureAdminServices adds nginx locations for admin services
func configureAdminServices(client *podman.Client, cname string, cfg *config.ProjectConfig) {
	hasPhpmyadmin := cfg.Services["phpmyadmin"].Enabled
	hasAdminer := cfg.Services["adminer"].Enabled

	if !hasPhpmyadmin && !hasAdminer {
		return
	}

	fmt.Printf("  Configuring admin services...\n")

	// Download phpMyAdmin if needed
	if hasPhpmyadmin {
		client.Exec(cname, "bash", "-c", `
rm -f /var/www/html/pma /var/www/html/public/pma
mkdir -p /var/www/html/pma
if [ ! -f /var/www/html/pma/index.php ]; then
    curl -sL https://files.phpmyadmin.net/phpMyAdmin/5.2.1/phpMyAdmin-5.2.1-all-languages.tar.gz | tar xz -C /tmp/ 2>/dev/null
    cp -r /tmp/phpMyAdmin-5.2.1-all-languages/* /var/www/html/pma/ 2>/dev/null
    rm -rf /tmp/phpMyAdmin-5.2.1-all-languages
fi
if [ -f /var/www/html/pma/config.inc.php ]; then
    cp /var/www/html/pma/config.inc.php /etc/pma-config.inc.php 2>/dev/null || true
    chmod 0644 /etc/pma-config.inc.php 2>/dev/null || true
    rm -f /var/www/html/pma/config.inc.php
    ln -sf /etc/pma-config.inc.php /var/www/html/pma/config.inc.php 2>/dev/null || true
fi
# Fix world-writable config (phpMyAdmin refuses to run) — #12
chmod 0644 /var/www/html/pma/config.inc.php 2>/dev/null || true
chown -R www-data:www-data /var/www/html/pma 2>/dev/null || true`)

		pmaNginx := `server {
    listen 8080 default_server;
    root /var/www/html/pma;
    index index.php;
    location / { try_files $uri $uri/ /index.php?$query_string; }
    location ~ \.php$ {
        fastcgi_pass 127.0.0.1:9000;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }
}
`
		writeNginxConfigTo(client, cname, pmaNginx, "/etc/nginx/sites-available/phpmyadmin")
		client.Exec(cname, "bash", "-c", "ln -sf /etc/nginx/sites-available/phpmyadmin /etc/nginx/sites-enabled/phpmyadmin && nginx -s reload 2>/dev/null || true")
	}

	// Configure adminer
	if hasAdminer {
		client.Exec(cname, "bash", "-c", `
mkdir -p /var/www/html/adminer
curl -sL https://www.adminer.org/latest.php -o /var/www/html/adminer/index.php 2>/dev/null
curl -sL https://www.adminer.org/download/v5.5.0/designs/haeckel/adminer.css -o /var/www/html/adminer/adminer.css 2>/dev/null
if [ -f /var/www/html/adminer/index.php ]; then
    cp /var/www/html/adminer/index.php /etc/adminer-index.php 2>/dev/null || true
    chmod 0644 /etc/adminer-index.php 2>/dev/null || true
    rm -f /var/www/html/adminer/index.php
    ln -sf /etc/adminer-index.php /var/www/html/adminer/index.php 2>/dev/null || true
fi
chmod 644 /var/www/html/adminer/adminer.css 2>/dev/null || true
chown -R www-data:www-data /var/www/html/adminer 2>/dev/null || true`)

		adminerNginx := `server {
    listen 8081;
    server_name adminer;
    root /var/www/html/adminer;
    index index.php;
    location / { try_files $uri $uri/ /index.php?$query_string; }
    location ~ \.php$ {
        fastcgi_pass 127.0.0.1:9000;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }
}
`
		writeNginxConfigTo(client, cname, adminerNginx, "/etc/nginx/sites-available/adminer")
		client.Exec(cname, "bash", "-c", "ln -sf /etc/nginx/sites-available/adminer /etc/nginx/sites-enabled/adminer && nginx -s reload 2>/dev/null || true")
	}
}
