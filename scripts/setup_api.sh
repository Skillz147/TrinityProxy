#!/bin/bash

set -e

API_DOMAIN="api.sauronstore.com"

echo "[+] Installing NGINX, Certbot, and dependencies..."
apt update -y
apt install -y nginx certbot python3-certbot-nginx ufw

echo "[+] Enabling firewall for HTTP and HTTPS..."
ufw allow 80
ufw allow 443

echo "[+] Configuring NGINX reverse proxy for $API_DOMAIN..."

cat >/etc/nginx/sites-available/trinityproxy-api <<NGINXEOF
server {
    listen 80;
    server_name $API_DOMAIN;

    location / {
        proxy_pass http://localhost:3100/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
    }
}
NGINXEOF

ln -s /etc/nginx/sites-available/trinityproxy-api /etc/nginx/sites-enabled/

echo "[+] Testing and reloading NGINX..."
nginx -t && systemctl reload nginx

echo "[+] Requesting Let's Encrypt cert for $API_DOMAIN..."
certbot --nginx --non-interactive --agree-tos -m admin@$API_DOMAIN -d $API_DOMAIN

echo "[+] Enabling auto-renewal..."
systemctl enable certbot.timer

echo ""
echo "[✔] TrinityProxy API is now live at: https://$API_DOMAIN/"
echo "[✔] Ready to receive secure heartbeats from agents."
