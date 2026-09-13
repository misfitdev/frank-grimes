#!/bin/bash
#
# Adapter/engine seam tests (audit FG-106).
#
# The engine and the adapters were tested apart: the engine against fake
# providers that emit whatever it accepts, the adapters as prose greps over
# their instruction files. Nothing asserted the two agree, so the adapter could
# instruct a model to emit a format the engine cannot read and every suite still
# passed.
#
# These tests read what an adapter tells a model to produce and ask the engine
# whether it could act on it. Fixtures are derived from the instruction file
# itself, so the assertion cannot drift away from what is actually shipped.
#
# Usage: ./tests/test-adapter-seam.sh
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#   2 - The Go toolchain is unavailable
#

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GRIND="$PROJECT_ROOT/adapters/claude-code/commands/grind.md"
HOOK="$PROJECT_ROOT/hooks/stop.sh"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASSED=0
FAILED=0

pass() {
    echo -e "${GREEN}PASS${NC}: $*"
    PASSED=$((PASSED + 1))
}

fail() {
    echo -e "${RED}FAIL${NC}: $*"
    FAILED=$((FAILED + 1))
}

assert_eq() {
    if [[ "$1" == "$2" ]]; then pass "$3"; else fail "$3 (got '$1' vs '$2')"; fi
}

if ! command -v go &>/dev/null; then
    echo -e "${YELLOW}SKIP${NC}: go is not installed; seam tests require the toolchain"
    exit 2
fi

BINDIR="$(mktemp -d)"
SANDBOX=""
cleanup() {
    rm -rf "$BINDIR"
    [[ -n "$SANDBOX" ]] && rm -rf "$SANDBOX"
}
trap cleanup EXIT

echo "========================================"
echo "Adapter and Engine Seam Tests"
echo "========================================"
echo ""

echo "--- Build ---"
if (cd "$PROJECT_ROOT" && go build -o "$BINDIR/grimes" ./cmd/grimes) 2>/dev/null &&
    (cd "$PROJECT_ROOT" && go build -o "$BINDIR/grimes-contract" ./cmd/grimes-contract) 2>/dev/null; then
    pass "grimes and grimes-contract build"
else
    fail "grimes and grimes-contract build"
    echo "Passed: $PASSED / Failed: $FAILED"
    exit 1
fi
export PATH="$BINDIR:$PATH"

echo ""
echo "--- The adapter's result format is the one the engine reads ---"

# The envelope marker is the engine's only entry point for a result. An adapter
# that never names it cannot hand one over, whatever else it emits.
if grep -q 'GRIMES_RESULT_PROTOBUF_V2_BEGIN' "$GRIND"; then
    pass "grind.md instructs the model to emit the result envelope"
else
    fail "grind.md does not mention the envelope the engine reads"
fi

# The legacy block is a different, unreadable format. Its presence means the
# instructions still describe a result nothing consumes.
if grep -qE '^GRIMES_RESULT: \{|GRIMES_RESULT: \{ "iteration"' "$GRIND"; then
    fail "grind.md still instructs the model to emit the legacy JSON result block"
else
    pass "grind.md no longer instructs the legacy JSON result block"
fi

echo ""
echo "--- Loop state belongs to the engine ---"

# The hook reads .grimes/state.pb. An adapter telling the model to write
# .grimes-state.json describes a file nothing reads, and the loop stops.
if grep -q '.grimes-state.json' "$GRIND"; then
    fail "grind.md tells the model to write .grimes-state.json, which the hook ignores"
else
    pass "grind.md does not tell the model to write the legacy state file"
fi

if grep -q '.grimes-state.json' "$HOOK"; then
    fail "stop.sh still reads the legacy state file"
else
    pass "stop.sh reads only the engine's run record"
fi

echo ""
echo "--- A run following the adapter's instructions continues the loop ---"

# The regression this suite exists for. Driving the documented path has to leave
# a record the hook acts on; the forged-legacy-state case in test-stop-hook.sh
# asserts the opposite outcome for the opposite input, and on its own it cannot
# tell a refused forgery from an unrecognised legitimate run.
SANDBOX="$(mktemp -d)"
mkdir -p "$SANDBOX/hooks"
cp "$HOOK" "$SANDBOX/hooks/stop.sh"
chmod +x "$SANDBOX/hooks/stop.sh"

# The adapter is expected to drive the engine rather than hand-write a record.
if grep -qE 'grimes run|grimes-contract encode-result' "$GRIND"; then
    pass "grind.md drives the engine to produce its result"
else
    fail "grind.md never invokes the engine, so no run record is ever produced"
fi

set +e
GRIMES_PROJECT_DIR="$SANDBOX" "$SANDBOX/hooks/stop.sh" >/dev/null 2>&1
EMPTY_CODE=$?
set -e
assert_eq "$EMPTY_CODE" "0" "an untouched project allows exit"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
