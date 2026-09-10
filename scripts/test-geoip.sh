#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
CONFIG_PATH="${1:-$ROOT_DIR/.geoip-test.json}"

case "$CONFIG_PATH" in
  /*) ;;
  *) CONFIG_PATH="$(pwd)/$CONFIG_PATH" ;;
esac

if [[ ! -f "$CONFIG_PATH" ]]; then
  printf 'error: GeoIP integration config not found: %s\n' "$CONFIG_PATH" >&2
  printf 'copy docs/geoip-test.example.json to .geoip-test.json and fill in local paths and credentials\n' >&2
  exit 1
fi

if [[ -z "${GO:-}" ]]; then
  GO=go
fi

cd "$ROOT_DIR"
VISITORTRACE_GEOIP_TEST_CONFIG="$CONFIG_PATH" "$GO" test -tags=geoip_integration ./internal/geoipupdate
