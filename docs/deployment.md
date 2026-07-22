# VisitorTrace Deployment Guide

[Chinese](./deployment.zh-CN.md)

This guide deploys one VisitorTrace process behind an HTTPS reverse proxy. The application listens only on loopback; the reverse proxy is the only public entry point.

## Prerequisites

- A 64-bit Linux server using AMD64 or ARM64.
- A domain such as `stats.example.com` pointing to the server.
- Nginx or another reverse proxy with a valid HTTPS certificate.
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

## Nginx Reverse Proxy

Terminate HTTPS at Nginx and proxy the complete domain to VisitorTrace:

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

Do not enable proxy caching for ingestion, Admin, health, or analytics routes. Static browser assets and SVG responses already send their own cache headers.

After reloading Nginx, verify:

```sh
curl -fsS http://127.0.0.1:8790/health/live
curl -fsS https://stats.example.com/health/live
curl -fsS https://stats.example.com/health/ready
```

The readiness check can remain unavailable until the first GeoIP download finishes. Inspect the service logs if it does not become ready.

## BT Panel

BT Panel is an optional process-management and reverse-proxy interface. VisitorTrace does not call a BT Panel API and uses the same executable, configuration, data, health checks, and update contract as the systemd deployment.

### Process Management

Install and initialize VisitorTrace using the common steps above. In the Go Project Manager, create a project with values equivalent to:

| Setting | Value |
|---|---|
| Project directory | `/var/lib/visitortrace` |
| Executable | `/var/lib/visitortrace/releases/current/visitortrace` |
| Arguments or startup command | `serve --config /etc/visitortrace/config.json` |
| Listening port | `8790` |
| Run user | `visitortrace` |
| Restart policy | Restart after every process exit |

Panel versions use different labels for executable and argument fields. The resulting operating-system command must be:

```sh
/var/lib/visitortrace/releases/current/visitortrace serve --config /etc/visitortrace/config.json
```

If the manager cannot run an existing executable or cannot restart after a clean exit, use the systemd unit above and use BT Panel only for Nginx, TLS, and log viewing. Do not run both supervisors for the same process.

If the panel forces its own process account, grant that account ownership of `/var/lib/visitortrace` and read access to `/etc/visitortrace/config.json`; do not make the configuration world-readable.

### Website and Reverse Proxy

1. Create a website for `stats.example.com` and issue its SSL certificate.
2. Under the website settings, open **Reverse Proxy** and add a rule for `/`.
3. Set the target URL to `http://127.0.0.1:8790`.
4. Preserve the original host, disable proxy caching, and leave content replacement empty.
5. Confirm that `X-Forwarded-For` and `X-Forwarded-Proto` are passed by the generated Nginx configuration.

The current BT Panel navigation and reverse-proxy fields are documented in the [official reverse-proxy guide](https://docs.bt.cn/user-guide/site/php/site-config/reverse-proxy).

Create a daily Shell task in **Scheduled Tasks** for:

```sh
sudo -u visitortrace /var/lib/visitortrace/releases/current/visitortrace backup \
  --config /etc/visitortrace/config.json
```

Use either this task or the systemd timer, not both.

## Post-Deployment

1. Sign in at `https://stats.example.com/admin/login`.
2. Create a Site with the exact public website Origin.
3. Configure its timezone, retention period, deduplication window, and Map Preset.
4. Install the Integrated Widget or separated Tracker from the Site page.
5. Create a manual backup and confirm it appears on the operational dashboard.

Keep `/etc/visitortrace`, `/var/lib/visitortrace`, and backup storage access-controlled. Back up the release signing private key separately; it must never be placed on the application server.
