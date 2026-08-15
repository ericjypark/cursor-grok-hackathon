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

# ---- house style ---------------------------------------------------------
# The launcher wears the same violet-to-cyan ramp as the CLI it starts. Color
# is dropped entirely when stdout is not a terminal, and the per-character
# gradient is dropped when the terminal cannot do 24-bit color.
[ -t 1 ] && COLOR=1 || COLOR=0
case "${COLORTERM:-}" in truecolor|24bit) TRUECOLOR=1 ;; *) TRUECOLOR=0 ;; esac

off=''; bold=''; dim=''; body=''; violet=''; cyan=''; red=''; yellow=''
if [ "$COLOR" -eq 1 ]; then
  off=$'\033[0m'; bold=$'\033[1m'
  dim=$'\033[38;2;113;113;121m'; body=$'\033[38;2;211;211;216m'
  violet=$'\033[38;2;124;92;255m'; cyan=$'\033[38;2;34;211;238m'
  red=$'\033[38;2;251;113;133m'; yellow=$'\033[38;2;251;191;36m'
fi

# ramp emits the escape for step $1 of $2 along violet -> cyan.
ramp() {
  local t=$(( $2 > 1 ? $1 * 1000 / ($2 - 1) : 0 ))
  printf '\033[38;2;%d;%d;%dm' \
    $((167 + (34 - 167) * t / 1000)) \
    $((139 + (211 - 139) * t / 1000)) \
    $((250 + (238 - 250) * t / 1000))
}

# paint spreads the ramp across an ASCII string, one step per character.
paint() {
  local s=$1 n=${#1} i out=''
  if [ "$COLOR" -eq 0 ]; then printf '%s' "$s"; return; fi
  if [ "$TRUECOLOR" -eq 0 ] || [ "$n" -le 1 ]; then printf '%s%s%s' "$violet" "$s" "$off"; return; fi
  for ((i = 0; i < n; i++)); do out+="$(ramp "$i" "$n")${s:i:1}"; done
  printf '%s%s' "$out" "$off"
}

# rule draws the gradient divider, matching the CLI's 60-column frame.
rule() {
  local i out=''
  # Literal box-drawing characters, not \u escapes: bash 3.2 (the macOS
  # default) prints those verbatim rather than expanding them.
  if [ "$COLOR" -eq 0 ]; then printf '%60s' '' | tr ' ' '-'; return; fi
  if [ "$TRUECOLOR" -eq 0 ]; then printf '%s' "$violet"; for ((i = 0; i < 60; i++)); do out+='─'; done
  else for ((i = 0; i < 60; i++)); do out+="$(ramp "$i" 60)─"; done; fi
  printf '%s%s' "$out" "$off"
}

banner() {
  printf '\n  %s%s%s  %s%s%s  %s· booting the stack%s\n' \
    "$violet" '◆' "$off" "$bold" "$(paint 'fieldnote')" "$off" "$dim" "$off"
  printf '  %s\n\n' "$(rule)"
}

step() { printf '  %s%s%s %s%s%s\n' "$violet" '▸' "$off" "$body" "$1" "$off"; }
info() { printf '    %s%s%s\n' "$dim" "$1" "$off"; }
die()  { printf '\n  %s%s %s%s\n\n' "$red" '✗' "$1" "$off" >&2; exit 1; }

DEMO=0
CLI_ARGS=()
for arg in "$@"; do
  if [ "$arg" = "--demo" ]; then DEMO=1; else CLI_ARGS+=("$arg"); fi
done

banner

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
  for key in FIRECRAWL_API_KEY EXA_API_KEY XAI_API_KEY; do
    have_key "$key" || missing+=("$key")
  done
  if [ ${#missing[@]} -gt 0 ]; then
    printf '\n  %s%s missing %s%s\n\n' "$red" '✗' "${missing[*]}" "$off" >&2
    printf '    %sAdd them to%s %s/.env  %s(cp .env.example .env)%s\n' "$dim" "$off" "$BACKEND_DIR" "$dim" "$off" >&2
    printf '    %sOr run without keys:%s %s./run.sh --demo%s\n\n' "$dim" "$off" "$bold" "$off" >&2
    exit 1
  fi
fi

# ---- backend server ------------------------------------------------------
APP="main:app"
if [ "$DEMO" -eq 1 ]; then
  APP="demo_server:app"
  printf '  %s▲ demo mode%s %s· canned dossier, no APIs called%s\n' \
    "$yellow" "$off" "$dim" "$off"
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
    ( cd "$BACKEND_DIR" && exec env FIRECRAWL_API_KEY=x EXA_API_KEY=x XAI_API_KEY=x \
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

printf '  %s\n' "$(rule)"
set +e
# bash 3.2 (the macOS default) treats an empty array under `set -u` as an
# unbound variable, so the expansion is guarded rather than written plainly.
"$HERE/fieldnote" --backend "$BASE" ${CLI_ARGS[@]+"${CLI_ARGS[@]}"}
STATUS=$?
set -e
exit "$STATUS"
