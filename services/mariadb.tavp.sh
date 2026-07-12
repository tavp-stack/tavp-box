TVP_NAME="mariadb"
TVP_DESC="MariaDB database server"
TVP_CATEGORY="database"
TVP_PORTS=(3306)
TVP_UI_PORT=""
TVP_UI_SUBDOMAIN=""
TVP_INSTALL_apt='apt-get update && apt-get install -y mariadb-server && service mariadb start 2>/dev/null || (mysqld_safe --user=mysql &)'
TVP_INSTALL_apk='apk add mariadb && rc-service mariadb start'
TVP_INSTALL_dnf='dnf install -y mariadb-server && systemctl start mariadb'
TVP_INSTALL_zypper='zypper install -y mariadb && systemctl start mariadb'
TVP_INSTALL_pacman='pacman -S --noconfirm mariadb && mysqld_install_db --user=mysql && systemctl start mariadb'
TVP_INSTALL_xbps='xbps-install -y mariadb && mysql_install_db && ln -s /etc/sv/mariadb /var/service/'
