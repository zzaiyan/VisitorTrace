#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

SERVICE_NAME="visitortrace"
SERVICE_USER="visitortrace"
BINARY="/usr/local/bin/visitortrace"
DATA_DIR="/var/lib/visitortrace"
CONFIG_PATH="/etc/visitortrace/config.json"

usage() {
  cat <<'EOF'
Usage: install-systemd.sh [options]

Configure an already-installed VisitorTrace binary as a systemd service.
This script does not download files, create backups, configure a reverse proxy,
or configure BT Panel.

Options:
  --binary PATH       VisitorTrace executable (default: /usr/local/bin/visitortrace)
  --user NAME         Dedicated service user (default: visitortrace)
  --data-dir PATH     Persistent data directory (default: /var/lib/visitortrace)
  --config PATH       Protected config path (default: /etc/visitortrace/config.json)
  --service-name NAME systemd service name (default: visitortrace)
  -h, --help          Show this help
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_absolute_path() {
  case "$2" in
    /*) ;;
    *) die "$1 must be an absolute path: $2" ;;
  esac
  case "$2" in
    *[[:space:]]*) die "$1 must not contain whitespace: $2" ;;
  esac
}

while (($# > 0)); do
  case "$1" in
    --binary)
      (($# >= 2)) || die "--binary requires a path"
      BINARY="$2"
      shift 2
      ;;
    --user)
      (($# >= 2)) || die "--user requires a name"
      SERVICE_USER="$2"
      shift 2
      ;;
    --data-dir)
      (($# >= 2)) || die "--data-dir requires a path"
      DATA_DIR="$2"
      shift 2
      ;;
    --config)
      (($# >= 2)) || die "--config requires a path"
      CONFIG_PATH="$2"
      shift 2
      ;;
    --service-name)
      (($# >= 2)) || die "--service-name requires a name"
      SERVICE_NAME="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1 (use --help for usage)"
      ;;
  esac
done

[[ "$(id -u)" == "0" ]] || die "run this script as root"
[[ -x "$BINARY" ]] || die "VisitorTrace executable is not executable: $BINARY"
require_command id
require_command install
require_command useradd
require_command runuser
require_command systemctl
require_command chown
require_command chmod
require_command dirname
require_absolute_path "--binary" "$BINARY"
require_absolute_path "--data-dir" "$DATA_DIR"
require_absolute_path "--config" "$CONFIG_PATH"

case "$SERVICE_USER" in
  ''|*[!a-zA-Z0-9_.-]*) die "invalid service user name: $SERVICE_USER" ;;
esac
case "$SERVICE_NAME" in
  ''|*[!a-zA-Z0-9_.@-]*) die "invalid systemd service name: $SERVICE_NAME" ;;
esac

CONFIG_DIR="$(dirname -- "$CONFIG_PATH")"
UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

if id "$SERVICE_USER" >/dev/null 2>&1; then
  printf 'using existing service user: %s\n' "$SERVICE_USER"
else
  useradd --system \
    --home-dir "$DATA_DIR" \
    --create-home \
    --shell /usr/sbin/nologin \
    "$SERVICE_USER"
  printf 'created service user: %s\n' "$SERVICE_USER"
fi

SERVICE_GROUP="$(id -gn "$SERVICE_USER")"
install -d -m 700 -o "$SERVICE_USER" -g "$SERVICE_GROUP" "$DATA_DIR" "$CONFIG_DIR"

if [[ -e "$CONFIG_PATH" ]]; then
  printf 'using existing config: %s\n' "$CONFIG_PATH"
else
  printf 'initializing VisitorTrace; enter the Administrator password when prompted\n'
  runuser -u "$SERVICE_USER" -- "$BINARY" init \
    --data-dir "$DATA_DIR" \
    --config "$CONFIG_PATH"
fi

[[ -f "$CONFIG_PATH" ]] || die "config was not created: $CONFIG_PATH"
chown "$SERVICE_USER:$SERVICE_GROUP" "$CONFIG_PATH"
chmod 600 "$CONFIG_PATH"

printf 'initializing the stable executable path for self-updates\n'
runuser -u "$SERVICE_USER" -- "$BINARY" update bootstrap --config "$CONFIG_PATH"

install -d -m 755 /etc/systemd/system
cat > "$UNIT_PATH" <<EOF
[Unit]
Description=VisitorTrace visitor analytics
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory=$DATA_DIR
ExecStart=$DATA_DIR/releases/current/visitortrace serve --config $CONFIG_PATH
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
ReadWritePaths=$DATA_DIR $CONFIG_DIR

[Install]
WantedBy=multi-user.target
EOF
chmod 644 "$UNIT_PATH"

systemctl daemon-reload
systemctl enable "$SERVICE_NAME.service"
systemctl restart "$SERVICE_NAME.service"
systemctl is-active --quiet "$SERVICE_NAME.service" || {
  systemctl --no-pager --full status "$SERVICE_NAME.service" || true
  die "systemd service did not become active"
}

printf '\nVisitorTrace is running under systemd.\n'
printf 'service: %s.service\n' "$SERVICE_NAME"
printf 'config:  %s\n' "$CONFIG_PATH"
printf 'data:    %s\n' "$DATA_DIR"
printf 'check:   systemctl status %s.service\n' "$SERVICE_NAME"
printf 'logs:    journalctl -u %s.service -f\n' "$SERVICE_NAME"
