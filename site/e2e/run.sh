#!/usr/bin/env bash
# Browser test of site/index.html against a real dataset.
#
#   site/e2e/run.sh [path/to/dist]        # default: ./dist, from `toi build`
#   E2E_SHOTS=/tmp/shots site/e2e/run.sh  # also save screenshots there
#
# Needs Node 22+ (for the built-in WebSocket), python3 and a Chrome or Chromium
# (set CHROME to choose one). It serves a copy of the page next to the data on
# localhost, starts a headless browser, drives it over the DevTools protocol,
# and cleans everything up. Not part of CI; run it after changing the page.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
DIST=$(cd "${1:-$ROOT/dist}" && pwd)
CHROME=${CHROME:-$(command -v chromium || command -v chromium-browser || command -v google-chrome || command -v google-chrome-stable || true)}
[ -n "$CHROME" ] || { echo "no Chrome or Chromium found; set CHROME=/path/to/it" >&2; exit 2; }

export E2E_PORT=${E2E_PORT:-8765} E2E_CDP=${E2E_CDP:-9333} E2E_DIST=$DIST
WORK=$(mktemp -d)
mkdir -p "$WORK/site/v2" "$WORK/home"
cp "$ROOT/site/index.html" "$WORK/site/"
cp -r "$DIST/index" "$DIST/simplified" "$DIST/manifest.json" "$DIST/report.json" "$WORK/site/v2/"

( cd "$WORK/site" && exec python3 -m http.server "$E2E_PORT" --bind 127.0.0.1 >/dev/null 2>&1 ) &
SRV=$!
# --no-sandbox because Chrome's own sandbox cannot start in many containers and
# CI runners; the browser only ever loads the localhost copy above.
HOME="$WORK/home" "$CHROME" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --remote-debugging-port="$E2E_CDP" --user-data-dir="$WORK/profile" about:blank >"$WORK/chrome.log" 2>&1 &
CH=$!
cleanup() {
  kill "$SRV" "$CH" 2>/dev/null || true
  wait 2>/dev/null || true
  sleep 1 # Chrome's helper processes outlive it briefly and still write to the profile
  rm -rf "$WORK" 2>/dev/null || { sleep 2; rm -rf "$WORK" 2>/dev/null || true; }
}
trap cleanup EXIT

node -e '
const [cdp, port] = [process.env.E2E_CDP, process.env.E2E_PORT], t = Date.now();
(async () => {
  while (Date.now() - t < 30000) {
    try { await fetch(`http://127.0.0.1:${cdp}/json/version`); await fetch(`http://127.0.0.1:${port}/`); process.exit(0); }
    catch { await new Promise((r) => setTimeout(r, 300)); }
  }
  console.error("the server or the browser did not start"); process.exit(1);
})();
' || { tail -5 "$WORK/chrome.log" >&2; exit 1; }

node "$HERE/drive.mjs"
