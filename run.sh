#!/usr/bin/env bash
#
# One command to get T0 running: starts the backend, builds the CLI, runs it.
#
#   ./run.sh                                  # prompts for the inputs
#   ./run.sh --url https://cursor.com         # or pass them straight through
#   ./run.sh --demo                           # canned dossier, no API keys needed
#
# Any flag other than --demo is forwarded to the CLI verbatim.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="${FIELDNOTE_BACKEND_DIR:-$(dirname "$HERE")/field-note-backend}"
BACKEND_REPO="${FIELDNOTE_BACKEND_REPO:-in-sol-ence/field-note-backend}"
PORT="${FIELDNOTE_PORT:-8000}"
BASE="http://127.0.0.1:${PORT}"

bold=$'\033[1m'; dim=$'\033[2m'; red=$'\033[31m'; green=$'\033[32m'; yellow=$'\033[33m'; off=$'\033[0m'
step() { printf '%s==>%s %s\n' "$bold" "$off" "$1"; }
info() { printf '    %s%s%s\n' "$dim" "$1" "$off"; }
die()  { printf '%s error:%s %s\n' "$red" "$off" "$1" >&2; exit 1; }

DEMO=0
CLI_ARGS=()
for arg in "$@"; do
  if [ "$arg" = "--demo" ]; then DEMO=1; else CLI_ARGS+=("$arg"); fi
done

# ---- prerequisites -------------------------------------------------------
for tool in uv go curl; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is not installed"
done

# ---- backend checkout ----------------------------------------------------
if [ ! -d "$BACKEND_DIR" ]; then
  step "Cloning the backend into $BACKEND_DIR"
  command -v gh >/dev/null 2>&1 || die "backend missing and gh not installed; clone $BACKEND_REPO to $BACKEND_DIR"
  gh repo clone "$BACKEND_REPO" "$BACKEND_DIR" >/dev/null 2>&1 || die "could not clone $BACKEND_REPO"
fi

step "Syncing backend dependencies"
(cd "$BACKEND_DIR" && uv sync --quiet) || die "uv sync failed"

# ---- credentials ---------------------------------------------------------
have_key() {
  local key="$1" env_file="$BACKEND_DIR/.env"
  [ -n "${!key:-}" ] && return 0
  [ -f "$env_file" ] || return 1
  # A value copied unedited from .env.example still ends in "..."
  grep -qE "^${key}=[^[:space:]]+" "$env_file" && ! grep -qE "^${key}=.*\.\.\." "$env_file"
}

if [ "$DEMO" -eq 0 ]; then
  missing=()
  for key in FIRECRAWL_API_KEY EXA_API_KEY LLM_API_KEY; do
    have_key "$key" || missing+=("$key")
  done
  if [ ${#missing[@]} -gt 0 ]; then
    printf '%s error:%s missing %s\n\n' "$red" "$off" "${missing[*]}" >&2
    printf '  Add them to %s/.env  %s(cp .env.example .env)%s\n' "$BACKEND_DIR" "$dim" "$off" >&2
    printf '  Or run without keys:  %s./run.sh --demo%s\n\n' "$bold" "$off" >&2
    exit 1
  fi
fi

# ---- backend server ------------------------------------------------------
APP="main:app"
if [ "$DEMO" -eq 1 ]; then
  APP="demo_server:app"
  printf '%s  demo mode: canned dossier, no APIs called%s\n' "$yellow" "$off"
fi

STARTED_SERVER=0
cleanup() {
  if [ "$STARTED_SERVER" -eq 1 ] && [ -n "${SERVER_PID:-}" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

if curl -sf "$BASE/health" >/dev/null 2>&1; then
  step "Reusing the backend already on port $PORT"
else
  step "Starting the backend on port $PORT"
  LOG="${TMPDIR:-/tmp}/fieldnote-backend.log"
  # Demo mode needs placeholder keys so preflight passes. Real mode must NOT
  # set them: an environment variable outranks .env in pydantic-settings, so
  # injecting dummies here silently overrides the user's real credentials.
  if [ "$DEMO" -eq 1 ]; then
    ( cd "$BACKEND_DIR" && exec env FIRECRAWL_API_KEY=x EXA_API_KEY=x LLM_API_KEY=x \
        uv run uvicorn "$APP" --port "$PORT" ) > "$LOG" 2>&1 &
  else
    ( cd "$BACKEND_DIR" && exec uv run uvicorn "$APP" --port "$PORT" ) > "$LOG" 2>&1 &
  fi
  SERVER_PID=$!
  STARTED_SERVER=1
  info "logs: $LOG"

  for _ in $(seq 1 60); do
    curl -sf "$BASE/health" >/dev/null 2>&1 && break
    kill -0 "$SERVER_PID" 2>/dev/null || { tail -20 "$LOG" >&2; die "backend exited on startup"; }
    sleep 0.5
  done
  curl -sf "$BASE/health" >/dev/null 2>&1 || { tail -20 "$LOG" >&2; die "backend never became healthy"; }
fi

# ---- CLI -----------------------------------------------------------------
step "Building the CLI"
(cd "$HERE" && go build -o "$HERE/fieldnote" ./cmd/fieldnote) || die "go build failed"

step "Running T0"
echo
set +e
"$HERE/fieldnote" --backend "$BASE" "${CLI_ARGS[@]}"
STATUS=$?
set -e
exit "$STATUS"
