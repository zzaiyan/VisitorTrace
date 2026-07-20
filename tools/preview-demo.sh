#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$ROOT_DIR"
GO_BIN=${GO:-"$HOME/.local/go/bin/go"}
if [ ! -x "$GO_BIN" ]; then
  GO_BIN=go
fi

LISTEN=${VISITORTRACE_LISTEN:-127.0.0.1:8790}
SITE_ORIGIN=${VISITORTRACE_SITE_ORIGIN:-http://127.0.0.1:8088}
PASSWORD=${VISITORTRACE_PREVIEW_PASSWORD:-VisitorTrace2026}
WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/visitortrace-demo.XXXXXX")
CONFIG="$WORK_DIR/config.json"
BINARY="$WORK_DIR/visitortrace"
SERVER_PID=

cleanup() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT INT TERM

if command -v curl >/dev/null 2>&1 && curl -fsS "http://$LISTEN/health/live" >/dev/null 2>&1; then
  echo "a service is already listening on http://$LISTEN; set VISITORTRACE_LISTEN to another address" >&2
  exit 1
fi

"$GO_BIN" build -trimpath -o "$BINARY" ./cmd/visitortrace

printf '%s\n%s\n' "$PASSWORD" "$PASSWORD" | "$BINARY" init --data-dir "$WORK_DIR/data" --config "$CONFIG"

SITE_OUTPUT=$("$BINARY" site create --config "$CONFIG" --name "VisitorTrace Demo" --origin "$SITE_ORIGIN")
SITE_ID=$(printf '%s\n' "$SITE_OUTPUT" | awk '/^id:/ {print $2}')

"$GO_BIN" run ./tools/seed-demo --config "$CONFIG" --site-id "$SITE_ID"

"$BINARY" serve --config "$CONFIG" --listen "$LISTEN" &
SERVER_PID=$!

cat <<EOF

VisitorTrace demo is running.

Admin Console:    http://$LISTEN/admin/login
Admin password:   $PASSWORD
Public Analytics: http://$LISTEN/public/$SITE_ID/analytics?range=all
Public Map:       http://$LISTEN/api/v1/sites/$SITE_ID/map.svg?w=720&h=400
Site ID:          $SITE_ID

The database contains fake Pageviews with geographic coordinates for map marker styling.
Press Ctrl-C to stop and remove the temporary database.
EOF

wait "$SERVER_PID"
