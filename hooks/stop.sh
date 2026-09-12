#!/bin/bash
#
# Grimes Grind Stop Hook
#
# Intercepts exit attempts during an active grind loop and asks the engine
# whether another iteration is owed.
#
# The engine owns the loop: it derives the verdict, advances the iteration, and
# writes the run record. This hook decides nothing. It locates the repository
# and the binary, then reports what the engine said.
#
# Integration: your agent's hook system should call this script on session stop.
# - Exit 0: allow exit
# - Exit 2: block exit and continue grinding (prompt on stdout)
#
# Run record: .grimes/state.pb and .grimes/result.pb in the repository, written
# by the engine. A hand-written verdict is rejected, not believed.
#

set -euo pipefail

# The hook may be installed in a plugin cache, so its own location says nothing
# about which repository is under review. Ask the environment first and fall
# back to the script's parent only when nothing else identifies the project.
resolve_project_root() {
    if [[ -n "${GRIMES_PROJECT_DIR:-}" ]]; then
        echo "$GRIMES_PROJECT_DIR"
    elif [[ -n "${CLAUDE_PROJECT_DIR:-}" ]]; then
        echo "$CLAUDE_PROJECT_DIR"
    elif [[ -d "${PWD}/.grimes" ]]; then
        echo "$PWD"
    else
        (cd "$(dirname "$0")/.." && pwd)
    fi
}

PROJECT_ROOT="$(resolve_project_root)"
LOG_DIR="${PROJECT_ROOT}/.grimes/logs"
LOG_FILE="${LOG_DIR}/hook.log"
STATE_FILE="${PROJECT_ROOT}/.grimes/state.pb"

log() {
    mkdir -p "$LOG_DIR" 2>/dev/null || return 0
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >>"$LOG_FILE"
}

# Prefer a binary shipped beside the hook, then whatever is on PATH.
find_engine() {
    local here
    here="$(cd "$(dirname "$0")" && pwd)"
    for candidate in "${here}/../bin/grimes" "${here}/grimes"; do
        if [[ -x "$candidate" ]]; then
            echo "$candidate"
            return 0
        fi
    done
    command -v grimes 2>/dev/null || return 1
}

if ! GRIMES_BIN="$(find_engine)"; then
    # No engine and no run record is the ordinary case: no grind is running.
    if [[ -f "$STATE_FILE" ]]; then
        log "ERROR: grimes binary not found but loop state exists at $STATE_FILE"
        echo "Grimes Grind: the grimes binary was not found, so this run cannot be verified and no pass is recorded." >&2
    fi
    exit 0
fi

log "Invoking engine: $GRIMES_BIN loop --dir=$PROJECT_ROOT"

set +e
OUTPUT="$("$GRIMES_BIN" loop --dir="$PROJECT_ROOT" 2>>"$LOG_FILE")"
CODE=$?
set -e

if [[ -n "$OUTPUT" ]]; then
    echo "$OUTPUT"
fi

# Only a deliberate continue blocks the exit. Any other code, including an
# engine failure, lets the session end without recording a pass.
if [[ "$CODE" -eq 2 ]]; then
    log "Engine asked for another iteration"
    exit 2
fi

log "Engine allowed exit (code $CODE)"
exit 0
