#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
BINARY="${VISITORTRACE_BIN:-$SCRIPT_DIR/../bin/visitortrace}"

usage() {
  cat <<'EOF'
Usage: query-mmdb.sh [--binary PATH] [geoip query options] IP

Decode and print the raw MMDB record for one IP address.

Options:
  --binary PATH  VisitorTrace executable (default: ./bin/visitortrace)
  -h, --help     Show this help

The remaining options are passed to `visitortrace geoip query`, including
--config PATH and --mmdb PATH.
EOF
}

ARGS=()
while (($# > 0)); do
  case "$1" in
    --binary)
      (($# >= 2)) || { printf 'error: --binary requires a path\n' >&2; exit 2; }
      BINARY="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      ARGS+=("$@")
      break
      ;;
    *)
      ARGS+=("$1")
      shift
      ;;
  esac
done

[[ -x "$BINARY" ]] || {
  printf 'error: VisitorTrace executable is not executable: %s\n' "$BINARY" >&2
  exit 1
}

exec "$BINARY" geoip query "${ARGS[@]}"
