#!/usr/bin/env bash
set -Eeuo pipefail

umask 077

SERVICE_NAME="visitortrace"
SERVICE_USER="visitortrace"
DATA_DIR="/var/lib/visitortrace"
CONFIG_PATH="/etc/visitortrace/config.json"
BINARY=""
CHECKSUM_FILE=""
SHA256=""

usage() {
  cat <<'EOF'
Usage: update-systemd-binary.sh --binary PATH [options]

Install an already-downloaded VisitorTrace binary into the existing systemd
release layout. The script does not download files or configure a proxy.

Options:
  --binary PATH        Local downloaded VisitorTrace binary (required)
  --checksum-file PATH sha256sum file containing the binary checksum
  --sha256 HASH        Expected SHA-256 checksum
  --user NAME          Service user (default: visitortrace)
  --data-dir PATH      Persistent data directory (default: /var/lib/visitortrace)
  --config PATH        Protected config path (default: /etc/visitortrace/config.json)
  --service-name NAME  systemd service name (default: visitortrace)
  -h, --help           Show this help
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
    --checksum-file)
      (($# >= 2)) || die "--checksum-file requires a path"
      CHECKSUM_FILE="$2"
      shift 2
      ;;
    --sha256)
      (($# >= 2)) || die "--sha256 requires a hash"
      SHA256="$2"
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

[[ -n "$BINARY" ]] || die "--binary is required"
[[ "$(id -u)" == "0" ]] || die "run this script as root"
[[ -f "$BINARY" ]] || die "binary does not exist: $BINARY"
[[ -z "$CHECKSUM_FILE" || -f "$CHECKSUM_FILE" ]] || die "checksum file does not exist: $CHECKSUM_FILE"
require_command awk
require_command basename
require_command chmod
require_command chown
require_command id
require_command install
require_command ln
require_command mktemp
require_command mv
require_command readlink
require_command runuser
require_command sed
require_command sha256sum
require_command systemctl
require_absolute_path "--data-dir" "$DATA_DIR"
require_absolute_path "--config" "$CONFIG_PATH"

case "$SERVICE_USER" in
  ''|*[!a-zA-Z0-9_.-]*) die "invalid service user name: $SERVICE_USER" ;;
esac
case "$SERVICE_NAME" in
  ''|*[!a-zA-Z0-9_.@-]*) die "invalid systemd service name: $SERVICE_NAME" ;;
esac
[[ -z "$SHA256" || "$SHA256" =~ ^[[:xdigit:]]{64}$ ]] || die "--sha256 must contain 64 hexadecimal characters"

if [[ "$BINARY" != /* ]]; then
  BINARY="$PWD/$BINARY"
fi
BINARY_NAME="$(basename -- "$BINARY")"
[[ "$BINARY_NAME" != *[[:space:]]* ]] || die "binary filename must not contain whitespace"

id "$SERVICE_USER" >/dev/null 2>&1 || die "service user does not exist: $SERVICE_USER"
SERVICE_GROUP="$(id -gn "$SERVICE_USER")"
RELEASES_ROOT="$DATA_DIR/releases"
CURRENT_LINK="$RELEASES_ROOT/current"
PENDING_PATH="$DATA_DIR/.update-pending.json"
UNIT="$SERVICE_NAME.service"

[[ -d "$RELEASES_ROOT" ]] || die "release layout does not exist: $RELEASES_ROOT (run update bootstrap first)"
[[ -L "$CURRENT_LINK" ]] || die "current release link is missing: $CURRENT_LINK"
[[ ! -e "$PENDING_PATH" ]] || die "a signed self-update is already pending: $PENDING_PATH"
systemctl cat "$UNIT" >/dev/null 2>&1 || die "systemd unit does not exist: $UNIT"

CURRENT_TARGET="$(readlink -- "$CURRENT_LINK")"
case "$CURRENT_TARGET" in
  ''|.|..|*/*) die "current release link has an unsafe target: $CURRENT_TARGET" ;;
esac
CURRENT_BINARY="$RELEASES_ROOT/$CURRENT_TARGET/visitortrace"
[[ -x "$CURRENT_BINARY" ]] || die "current release executable is unavailable: $CURRENT_BINARY"
current_version_json="$(runuser -u "$SERVICE_USER" -- "$CURRENT_BINARY" version --json)" || die "current version command failed"
current_schema="$(printf '%s\n' "$current_version_json" | sed -n 's/.*"schema_version":\([0-9][0-9]*\).*/\1/p')"
[[ "$current_schema" =~ ^[0-9]+$ ]] || die "current binary returned an invalid schema version"

actual_sha256="$(sha256sum "$BINARY" | awk '{print $1}')"
if [[ -n "$SHA256" && "${actual_sha256,,}" != "${SHA256,,}" ]]; then
  die "binary SHA-256 mismatch: expected $SHA256, got $actual_sha256"
fi
if [[ -n "$CHECKSUM_FILE" ]]; then
  expected_sha256="$(awk -v wanted="$BINARY_NAME" 'function clean(value) { sub(/^\*/, "", value); return value } clean($2) == wanted { print $1; exit }' "$CHECKSUM_FILE")"
  [[ "$expected_sha256" =~ ^[[:xdigit:]]{64}$ ]] || die "no valid checksum found for $BINARY_NAME in $CHECKSUM_FILE"
  [[ "${actual_sha256,,}" == "${expected_sha256,,}" ]] || die "binary does not match $CHECKSUM_FILE"
else
  printf 'warning: no checksum supplied; use --checksum-file or --sha256 for release verification\n' >&2
fi

RELEASES_ROOT_TMP="$(mktemp "$RELEASES_ROOT/.manual-candidate.XXXXXX")"
cleanup_candidate() {
  rm -f -- "$RELEASES_ROOT_TMP"
}
trap cleanup_candidate EXIT
install -m 700 -o "$SERVICE_USER" -g "$SERVICE_GROUP" "$BINARY" "$RELEASES_ROOT_TMP"

version_json="$(runuser -u "$SERVICE_USER" -- "$RELEASES_ROOT_TMP" version --json)" || die "candidate version command failed"
new_version="$(printf '%s\n' "$version_json" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')"
new_schema="$(printf '%s\n' "$version_json" | sed -n 's/.*"schema_version":\([0-9][0-9]*\).*/\1/p')"
[[ "$new_version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]] || die "candidate returned an invalid semantic version: $new_version"
[[ "$new_schema" =~ ^[0-9]+$ ]] || die "candidate returned an invalid schema version"
[[ "$new_schema" == "$current_schema" ]] || die "manual update requires the same schema version ($current_schema); use the signed updater for schema version $new_schema"
release_name="v${new_version#v}"
case "$release_name" in
  *[!a-zA-Z0-9.+-]*) die "candidate version is unsafe for a release directory: $new_version" ;;
esac

target_directory="$RELEASES_ROOT/$release_name"
target_binary="$target_directory/visitortrace"
install -d -m 700 -o "$SERVICE_USER" -g "$SERVICE_GROUP" "$target_directory"
if [[ -e "$target_binary" ]]; then
  existing_sha256="$(sha256sum "$target_binary" | awk '{print $1}')"
  [[ "$existing_sha256" == "$actual_sha256" ]] || die "release directory already contains a different binary: $target_binary"
else
  install -m 700 -o "$SERVICE_USER" -g "$SERVICE_GROUP" "$BINARY" "$target_binary"
fi
chown "$SERVICE_USER:$SERVICE_GROUP" "$target_binary"
chmod 700 "$target_binary"
rm -f -- "$RELEASES_ROOT_TMP"
trap - EXIT

printf 'candidate: %s (schema %s)\n' "$target_binary" "$new_schema"
runuser -u "$SERVICE_USER" -- "$target_binary" doctor --config "$CONFIG_PATH" --upgrade-check

backup_output=""
if ! backup_output="$(runuser -u "$SERVICE_USER" -- "$CURRENT_BINARY" backup --config "$CONFIG_PATH")"; then
  printf '%s\n' "$backup_output" >&2
  die "pre-update backup failed"
fi
printf '%s\n' "$backup_output"

service_was_active=0
if systemctl is-active --quiet "$UNIT"; then
  service_was_active=1
fi

switch_current() {
  local target="$1"
  local temporary="$RELEASES_ROOT/.current-manual-$RANDOM-$$"
  [[ ! -e "$temporary" && ! -L "$temporary" ]] || return 1
  ln -s -- "$target" "$temporary"
  mv -Tf -- "$temporary" "$CURRENT_LINK"
}

restart_service() {
  systemctl restart "$UNIT" || return 1
  for _ in 1 2 3 4 5; do
    sleep 1
    systemctl is-active --quiet "$UNIT" || return 1
  done
  return 0
}

switched=0
rollback() {
  printf 'update failed; restoring release %s\n' "$CURRENT_TARGET" >&2
  if ! switch_current "$CURRENT_TARGET"; then
    printf 'error: failed to restore current release link\n' >&2
    return 1
  fi
  switched=0
  if ((service_was_active)); then
    if ! restart_service; then
      printf 'error: previous release also failed to start\n' >&2
      return 1
    fi
  fi
  return 0
}

on_exit() {
  local exit_code=$?
  if ((exit_code != 0 && switched)); then
    set +e
    rollback
    set -e
  fi
  exit "$exit_code"
}
trap on_exit EXIT

if ! switch_current "$release_name"; then
  die "could not switch current release link"
fi
switched=1

if ((service_was_active)); then
  if ! restart_service; then
    die "new release did not remain active"
  fi
  printf 'service restarted: %s\n' "$UNIT"
else
  printf 'service was inactive; release installed without starting it\n'
fi

switched=0
printf 'updated VisitorTrace to %s\n' "$new_version"
printf 'stable executable: %s\n' "$CURRENT_LINK/visitortrace"
