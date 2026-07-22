# VisitorTrace Deployment Guide

[Chinese](./deployment.zh-CN.md)

This guide runs VisitorTrace exclusively through systemd under a dedicated service account. BT Panel is used only to manage the domain, TLS certificate, and Nginx reverse proxy. The application listens on loopback, and Nginx is its only public entry point.

## Prerequisites

- A 64-bit Linux server using AMD64 or ARM64.
- A domain such as `stats.example.com` pointing to the server.
- BT Panel with Nginx installed.
- Root access for the initial installation.

Only ports 80 and 443 need to be public. Do not expose the VisitorTrace port to the Internet.

## Install and Initialize

Set the release version and architecture, then download the matching executable and checksum file:

```sh
VERSION=0.1.0
ARCH=amd64
curl -fLO "https://github.com/zzaiyan/VisitorTrace/releases/download/v${VERSION}/visitortrace-${VERSION}-linux-${ARCH}"
curl -fLO "https://github.com/zzaiyan/VisitorTrace/releases/download/v${VERSION}/checksums.txt"
grep " visitortrace-${VERSION}-linux-${ARCH}$" checksums.txt | sha256sum -c -
```

Create a dedicated service account and protected directories:

```sh
sudo useradd --system \
  --home-dir /var/lib/visitortrace \
  --create-home \
  --shell /usr/sbin/nologin \
  visitortrace
sudo install -Dm755 "visitortrace-${VERSION}-linux-${ARCH}" /usr/local/bin/visitortrace
sudo install -d -m700 -o visitortrace -g visitortrace /etc/visitortrace /var/lib/visitortrace
```

Initialize the database and enter the Administrator password when prompted:

```sh
sudo -u visitortrace /usr/local/bin/visitortrace init \
  --data-dir /var/lib/visitortrace \
  --config /etc/visitortrace/config.json
```

The default configuration listens on `127.0.0.1:8790`, stores SQLite and GeoIP data under `/var/lib/visitortrace`, and downloads the current DB-IP City Lite database on startup. Keep the configuration at mode `0600`.

Before placing a reverse proxy in front of the service, add its loopback addresses to `trusted_proxies` in `/etc/visitortrace/config.json`:

```json
"trusted_proxies": ["127.0.0.1/32", "::1/128"]
```

Only add addresses that are actual trusted proxies. This setting controls whether forwarded client IP and HTTPS headers are accepted.

Initialize the stable executable path used by one-click updates:

```sh
sudo -u visitortrace /usr/local/bin/visitortrace update bootstrap \
  --config /etc/visitortrace/config.json
```

The process supervisor must run `/var/lib/visitortrace/releases/current/visitortrace` and restart it even after a clean exit, because a verified self-update exits normally after switching the stable link.

## systemd

Create `/etc/systemd/system/visitortrace.service`:

```ini
[Unit]
Description=VisitorTrace visitor analytics
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=visitortrace
Group=visitortrace
WorkingDirectory=/var/lib/visitortrace
ExecStart=/var/lib/visitortrace/releases/current/visitortrace serve --config /etc/visitortrace/config.json
Restart=always
RestartSec=3s
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
RestrictSUIDSGID=true
LockPersonality=true
ReadWritePaths=/var/lib/visitortrace

[Install]
WantedBy=multi-user.target
```

Load and start the service:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now visitortrace
sudo systemctl status visitortrace
sudo journalctl -u visitortrace -f
```

`Restart=always` is intentional: an Admin-initiated update exits with status 0 and still needs systemd to start the new executable. An explicit `systemctl stop visitortrace` remains stopped.

### Daily Backups

VisitorTrace creates verified local backups on demand. To schedule one daily, create `/etc/systemd/system/visitortrace-backup.service`:

```ini
[Unit]
Description=Back up VisitorTrace

[Service]
Type=oneshot
User=visitortrace
Group=visitortrace
UMask=0077
ExecStart=/var/lib/visitortrace/releases/current/visitortrace backup --config /etc/visitortrace/config.json
```

Create `/etc/systemd/system/visitortrace-backup.timer`:

```ini
[Unit]
Description=Daily VisitorTrace backup

[Timer]
OnCalendar=daily
Persistent=true
RandomizedDelaySec=30m

[Install]
WantedBy=timers.target
```

Enable the timer:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now visitortrace-backup.timer
```

## BT Panel: HTTPS and Reverse Proxy

Do not create a VisitorTrace project in BT Panel's Go Project Manager. The panel may restrict project users to accounts such as `www` or `root`; neither account should run VisitorTrace. The `visitortrace` account does not need to appear in a BT Panel user selector because systemd is the only application supervisor. Do not run the same service under both systemd and BT Panel.

### Website and SSL

1. Point `stats.example.com` to the server and create that website in BT Panel.
2. No PHP or Go runtime is required for the website. Do not place VisitorTrace data in its document root.
3. Open the website's **SSL** settings and issue a Let's Encrypt certificate or install an existing certificate.
4. Enable HTTPS and, if appropriate, redirect HTTP to HTTPS.

### Reverse Proxy

Under the website settings, open **Reverse Proxy** and add one rule with values equivalent to:

| Setting | Value |
|---|---|
| Proxy path | `/` |
| Target URL | `http://127.0.0.1:8790` |
| Host | Preserve the original host |
| Cache | Disabled |
| Content replacement | Empty |

BT Panel versions use different field labels. Check the generated Nginx configuration and ensure the effective location contains these headers:

```nginx
location / {
    proxy_pass http://127.0.0.1:8790;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

`X-Forwarded-For` supplies the original visitor IP, while `X-Forwarded-Proto` lets secure Admin cookies work behind HTTPS. VisitorTrace accepts them only from the loopback CIDRs configured in `trusted_proxies`. Do not enable proxy caching for ingestion, Admin, health, or analytics routes; static assets and SVG responses already send their own cache headers.

The current BT Panel navigation and reverse-proxy fields are documented in the [official reverse-proxy guide](https://docs.bt.cn/user-guide/site/php/site-config/reverse-proxy).

## Verify and Troubleshoot

Verify the service directly and through the public HTTPS endpoint:

```sh
curl -fsS http://127.0.0.1:8790/health/live
curl -fsS https://stats.example.com/health/live
curl -sS https://stats.example.com/health/ready
```

The two live checks isolate systemd from the BT Panel proxy: if both return `{"status":"ok"}`, process supervision, Nginx, DNS, and TLS are working. A fully ready response is:

```json
{"checks":{"geoip":true,"schema":true,"sqlite":true},"status":"ready"}
```

The first GeoIP download may fail or remain unavailable on some networks. In that case, readiness returns HTTP 503. Use `curl` without `-f` to retain its diagnostic JSON, then inspect and retry the GeoIP operation:

```sh
sudo journalctl -u visitortrace -n 100 --no-pager
sudo -u visitortrace /var/lib/visitortrace/releases/current/visitortrace doctor \
  --config /etc/visitortrace/config.json
sudo -u visitortrace /var/lib/visitortrace/releases/current/visitortrace geoip update \
  --config /etc/visitortrace/config.json \
  --force
sudo systemctl restart visitortrace
```

A command-line GeoIP update runs outside the serving process, so restart the service after a successful manual update. If the server cannot reach DB-IP, download a valid DB-IP City Lite MMDB through another trusted network or mirror, place it at `/var/lib/visitortrace/geoip.mmdb` with owner `visitortrace`, mode `0600`, and restart the service. Disabling automatic updates does not remove the requirement for a valid local MMDB.

VisitorTrace intentionally has no route at `/`, so `https://stats.example.com/` returns `404 page not found`. The Administrator entry point is `https://stats.example.com/admin/login`; a public Site uses `/public/<SITE-ID>/analytics`. To redirect the bare domain to the login page, add an exact Nginx location alongside the proxy rule:

```nginx
location = / {
    return 302 /admin/login;
}
```

## Post-Deployment

1. Sign in at `https://stats.example.com/admin/login`.
2. Create a Site with the exact public website Origin.
3. Configure its timezone, retention period, deduplication window, and Map Preset.
4. Install the Integrated Widget or separated Tracker from the Site page.
5. Create a manual backup and confirm it appears on the operational dashboard.

Keep `/etc/visitortrace`, `/var/lib/visitortrace`, and backup storage access-controlled. Back up the release signing private key separately; it must never be placed on the application server.
