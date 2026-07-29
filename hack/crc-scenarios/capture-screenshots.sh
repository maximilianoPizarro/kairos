#!/usr/bin/env bash
# Capture product screenshots from a live console (port-forward recommended).
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$DIR/../.." && pwd)"
BASE="${KAIROS_CONSOLE_URL:-http://127.0.0.1:8181}"
OUT="${SCREENSHOT_OUT:-$ROOT/docs/images/screenshots}"
PAGES_OUT="${SCREENSHOT_PAGES_OUT:-$ROOT/docs/pages/images/screenshots}"

curl -sf --max-time 5 "$BASE/healthz" >/dev/null || {
  echo "Console not reachable at $BASE"
  echo "Run: oc port-forward -n kairos-system svc/kairos-console 8181:8080"
  exit 1
}

if [[ ! -d "$DIR/node_modules/playwright" ]]; then
  echo "Installing playwright (local to hack/crc-scenarios, gitignored)..."
  (cd "$DIR" && npm install playwright@1.49.1 --no-save --no-package-lock)
  (cd "$DIR" && npx playwright install chromium)
fi

mkdir -p "$OUT" "$PAGES_OUT"
node "$DIR/capture-screenshots.mjs" "$BASE" "$OUT"

# Mirror into GitHub Pages tree
cp -f "$OUT"/*.png "$PAGES_OUT"/
echo "Mirrored to $PAGES_OUT"
ls -la "$OUT"
