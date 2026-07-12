TVP_NAME="laravel"
TVP_DESC="Laravel (PHP + nginx + composer)"
TVP_CATEGORY="stack"
TVP_INSTALL_apt='apt-get update && apt-get install -y nginx php8.3-fpm php8.3-cli php8.3-mysql php8.3-mbstring php8.3-xml php8.3-curl php8.3-zip php8.3-gd unzip curl git && curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer && service nginx start && service php8.3-fpm start'
TVP_INSTALL_apk='apk add nginx php83-fpm php83-cli php83-mysqli php83-mbstring php83-xml php83-curl php83-zip php83-gd composer git && rc-service nginx start && rc-service php83-fpm start'
TVP_INSTALL_dnf='dnf install -y nginx php-fpm php-cli php-mysqlnd php-mbstring php-xml php-curl php-zip php-gd composer git && systemctl start nginx php-fpm'
TVP_INSTALL_zypper='zypper install -y nginx php8 php-fpm composer git && systemctl start nginx php-fpm'
TVP_INSTALL_pacman='pacman -S --noconfirm nginx php-fpm composer git && systemctl start nginx php-fpm'
TVP_INSTALL_xbps='xbps-install -y nginx php-fpm composer git && ln -s /etc/sv/nginx /var/service/'
