TVP_NAME="mysql"
TVP_DESC="MySQL database server"
TVP_CATEGORY="database"
TVP_PORTS=(3306)
TVP_UI_PORT=""
TVP_UI_SUBDOMAIN=""
TVP_INSTALL_apt='apt-get update && apt-get install -y mysql-server && service mysql start'
TVP_INSTALL_apk='apk add mysql && rc-service mysql start'
TVP_INSTALL_dnf='dnf install -y mysql-server && systemctl start mysqld'
TVP_INSTALL_zypper='zypper install -y mysql && systemctl start mysql'
TVP_INSTALL_pacman='pacman -S --noconfirm mysql && mysql_install_db --user=mysql && systemctl start mysql'
TVP_INSTALL_xbps='xbps-install -y mysql && mysql_install_db && ln -s /etc/sv/mysql /var/service/'
