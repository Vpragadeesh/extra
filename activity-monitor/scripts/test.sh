#!/usr/bin/env bash
# Repeatable local checks: backend tests, frontend production build and
# tests, fish syntax, extension JS syntax, and JSON validity.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ ! -x "$ROOT/backend/.venv/bin/python" ]]; then
  echo "error: backend/.venv is missing; see README.md" >&2
  exit 1
fi

# Frontend deps are pnpm-managed; fall back to npm when pnpm is missing.
if [[ ! -d "$ROOT/frontend/node_modules" ]]; then
  echo "error: frontend/node_modules is missing; run pnpm install in frontend/" >&2
  exit 1
fi
PM=()
if command -v pnpm >/dev/null 2>&1; then PM=("pnpm"); else PM=("npm" "run"); fi

echo "== backend tests =="
(cd "$ROOT/backend" && .venv/bin/python -m pytest -q)

echo "== frontend tests =="
(cd "$ROOT/frontend" && "${PM[@]}" test)

echo "== frontend build =="
(cd "$ROOT/frontend" && "${PM[@]}" build)

echo "== fish syntax =="
fish -n "$ROOT/fish/activity-monitor.fish"

echo "== extension syntax =="
node --check "$ROOT/firefox-extension/background.js"
node --check "$ROOT/firefox-extension/options.js"
python3 -m json.tool "$ROOT/firefox-extension/manifest.json" >/dev/null

echo "all checks passed"
