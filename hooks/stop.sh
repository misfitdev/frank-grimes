#!/bin/bash
#
# Grimes Grind Stop Hook
#
# This hook intercepts exit attempts during an active grind loop.
# If a grind is in progress and verdict is not GREEN, it re-injects the prompt
# to continue iteration.
#
# Based on the Ralph Wiggum loop technique by Geoffrey Huntley.
#
# Integration: Your agent's hook system should call this script on session stop.
# - Exit 0: Allow exit (grind complete, max iterations reached, or auto-loop disabled)
# - Exit 2: Block exit and continue grinding (re-injected prompt on stdout)
#
# State file: .grimes-state.json in the project root (gitignored by default).
# The hook reads/writes this file to track iteration, verdict, and auto-loop status.
#

set -euo pipefail

# Project root: the directory containing this script's parent
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE_FILE="${PROJECT_ROOT}/.grimes-state.json"

LOG_DIR="${PROJECT_ROOT}/.grimes/logs"
LOG_FILE="${LOG_DIR}/hook.log"

# Initialize directories
mkdir -p "$LOG_DIR"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" >> "$LOG_FILE"
}

# Validate jq is available
if ! command -v jq &> /dev/null; then
    log "ERROR: jq not found. Auto-loop feature requires jq."
    echo "ERROR: jq is required for auto-loop feature. Install with: brew install jq (macOS) or apt-get install jq (Linux)" >&2
    exit 0
fi

# Check if state file exists (indicates active grind)
if [[ ! -f "$STATE_FILE" ]]; then
    log "No active grind state found at $STATE_FILE"
    exit 0
fi

# Validate state file is valid JSON
if ! jq empty "$STATE_FILE" 2>/dev/null; then
    log "ERROR: Corrupted state file at $STATE_FILE. Cannot parse JSON."
    echo "WARNING: State file appears corrupted. Please check $STATE_FILE" >&2
    rm -f "$STATE_FILE"
    exit 0
fi

# Validate required fields exist
for field in iteration max_iterations last_verdict target auto_loop; do
    if ! jq -e ".$field" "$STATE_FILE" &>/dev/null; then
        log "ERROR: State file missing required field: $field"
        echo "WARNING: State file is incomplete. Removing corrupted state." >&2
        rm -f "$STATE_FILE"
        exit 0
    fi
done

# Read state
STATE=$(cat "$STATE_FILE")
ITERATION=$(echo "$STATE" | jq -r '.iteration // 0')
MAX_ITERATIONS=$(echo "$STATE" | jq -r '.max_iterations // 5')
LAST_VERDICT=$(echo "$STATE" | jq -r '.last_verdict // "RED"')
TARGET=$(echo "$STATE" | jq -r '.target // ""')
AUTO_LOOP=$(echo "$STATE" | jq -r '.auto_loop // false')

# Validate iteration and max_iterations are positive integers
if ! [[ "$ITERATION" =~ ^[0-9]+$ ]] || [[ "$ITERATION" -eq 0 ]]; then
    log "ERROR: iteration is not a valid positive integer: $ITERATION"
    rm -f "$STATE_FILE"
    exit 0
fi

if ! [[ "$MAX_ITERATIONS" =~ ^[0-9]+$ ]] || [[ "$MAX_ITERATIONS" -eq 0 ]]; then
    log "ERROR: max_iterations is not a valid positive integer: $MAX_ITERATIONS"
    rm -f "$STATE_FILE"
    exit 0
fi

# Validate verdict is one of GREEN, YELLOW, or RED
if [[ ! "$LAST_VERDICT" =~ ^(GREEN|YELLOW|RED)$ ]]; then
    log "ERROR: Invalid verdict '$LAST_VERDICT' in state file. Must be GREEN, YELLOW, or RED. Treating as RED."
    LAST_VERDICT="RED"
fi

log "Hook invoked: iteration=$ITERATION/$MAX_ITERATIONS, verdict=$LAST_VERDICT, auto_loop=$AUTO_LOOP"

# If auto_loop is not enabled, allow exit
if [[ "$AUTO_LOOP" != "true" ]]; then
    log "Auto-loop disabled, allowing exit"
    exit 0
fi

# If verdict is GREEN, allow exit
if [[ "$LAST_VERDICT" == "GREEN" ]]; then
    log "Verdict is GREEN, allowing exit and cleaning up state file"
    echo "Grimes Grind complete. Verdict: GREEN. Exiting."
    rm -f "$STATE_FILE"
    exit 0
fi

# If max iterations reached, allow exit
if [[ "$ITERATION" -ge "$MAX_ITERATIONS" ]]; then
    log "Max iterations ($MAX_ITERATIONS) reached with verdict $LAST_VERDICT, allowing exit and cleaning up"
    echo "Grimes Grind: Max iterations ($MAX_ITERATIONS) reached. Final verdict: $LAST_VERDICT"
    rm -f "$STATE_FILE"
    exit 0
fi

# Otherwise, block exit and continue grinding
log "Loop continuation conditions met:"
log "  - auto_loop is $AUTO_LOOP (not disabled)"
log "  - verdict is $LAST_VERDICT (not GREEN)"
log "  - iteration is $ITERATION (< max of $MAX_ITERATIONS)"
log "BLOCKING EXIT and continuing to iteration $((ITERATION + 1))"

NEXT_ITERATION=$((ITERATION + 1))

# Update state
if ! echo "$STATE" | jq ".iteration = $NEXT_ITERATION" > "$STATE_FILE.tmp"; then
    log "ERROR: Failed to update state file"
    echo "ERROR: Failed to update grind state. Allowing exit." >&2
    rm -f "$STATE_FILE.tmp"
    exit 0
fi
mv "$STATE_FILE.tmp" "$STATE_FILE"

echo "Grimes Grind: Iteration $NEXT_ITERATION of $MAX_ITERATIONS. Last verdict: $LAST_VERDICT. Continuing..."

# Re-inject the grind prompt
log "Re-injecting grind prompt for iteration $NEXT_ITERATION"
cat << EOF
Grimes Grind: Continue Disciplined Falsification Review

Target: $TARGET
Iteration: $NEXT_ITERATION of $MAX_ITERATIONS
Previous Verdict: $LAST_VERDICT

The previous iteration failed to achieve a GREEN verdict. You are required to:

1. Conduct a clinical review of any fixes proposed in the previous iteration.
2. Execute a Phase 5 Scoped Re-Grind. Actively seek evidence to prove the fixes are broken, insecure, or introduce new failure modes.
3. Re-evaluate Phase 6 Stop Conditions.
4. Deliver an updated Grimes Report using the mandatory Evidence-First format and clinical persona defined in SKILL.md.

**CRITICAL STATE FILE REQUIREMENT:**
You MUST update .grimes-state.json after determining your verdict:
- Set last_verdict to your new verdict (GREEN, YELLOW, or RED)
- Set issues_found and issues_fixed to your iteration counts
- DO NOT change the iteration number (the hook manages that)
- Use the Write tool to persist changes

Without updating the state file, the loop will not continue even if your verdict is YELLOW.

Ensure every risk is preceded by technical evidence. Do not stop until terminal flaws are mitigated or maximum iterations are reached.
EOF

# Exit with code 2 to signal the agent should continue
exit 2
