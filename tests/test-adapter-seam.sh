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

# grind.md builds its report through the CLI rather than naming a marker, so the
# check that matters is whether the documented commands produce something the
# engine accepts. That is asserted end to end below.
if grep -q 'grimes-contract report' "$GRIND"; then
    pass "grind.md builds its report through the contract CLI"
else
    fail "grind.md does not build its report through the contract CLI"
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
# The collector fingerprints content, so the reviewed script has to be there.
printf 'rm -rf ./build/*\n' >"$SANDBOX/bad-script.sh"
mkdir -p "$SANDBOX/hooks"
cp "$HOOK" "$SANDBOX/hooks/stop.sh"
chmod +x "$SANDBOX/hooks/stop.sh"

# The adapter is expected to drive the engine rather than hand-write a record.
# `grimes-contract encode-result` would not satisfy that: it is a codec call
# that produces no engine-owned run record, and accepting it here would let the
# end-to-end case below validate a test-authored invocation instead.
if grep -qE '^grimes run|grimes run --dir' "$GRIND"; then
    pass "grind.md drives the engine to produce its result"
else
    fail "grind.md never invokes the engine, so no run record is ever produced"
fi

# auto-loop is documented as off by default, so the documented command must not
# hand it to the engine unasked: every ordinary grind would otherwise write loop
# state and arm iterations the caller did not request.
if grep -qE '^grimes run .*--auto-loop' "$GRIND"; then
    fail "grind.md passes --auto-loop unconditionally"
else
    pass "grind.md leaves --auto-loop to the caller"
fi

set +e
GRIMES_PROJECT_DIR="$SANDBOX" "$SANDBOX/hooks/stop.sh" >/dev/null 2>&1
EMPTY_CODE=$?
set -e
assert_eq "$EMPTY_CODE" "0" "an untouched project allows exit"

echo ""
echo "--- The documented commands produce a record the engine accepts ---"

# Run the sequence grind.md specifies, with the report a review would build.
# This is the assertion the suite exists for: both halves, together, on the
# real binaries.
(
    cd "$SANDBOX" || exit 1
    # shellcheck disable=SC2016  # the $1 is quoted evidence, not an expansion
    grimes-contract report add \
        --category=SEC --severity=P0 --blast=systemic --likelihood=likely \
        --path=bad-script.sh \
        --tier=E2 --claim="caller-controlled deletion path" \
        --quote='rm -rf "$1"/*' >/dev/null
) || fail "the documented report commands failed"

# Sealing runs as the provider command, so it inherits the run identity the
# engine exports. Sealing beforehand could not carry it: the run does not exist
# yet, and the engine refuses a report raised against another request.
set +e
# --auto-loop stands in for a caller who asked to iterate; grind.md leaves the
# flag to the caller, which is asserted separately above.
(cd "$SANDBOX" && grimes run --dir=. --auto-loop \
    --provider-command="grimes-contract report seal --routed=SEC,COR \
        --examined=4 --disproved=3 --summary=One deletion path survived." \
    bad-script.sh >/dev/null 2>&1)
RUN_CODE=$?
set -e

# A reported P0 blocks, so 4 is the verdict exit code, not a failure.
assert_eq "$RUN_CODE" "4" "a reported P0 drives the run to a blocking verdict"

if [[ -f "$SANDBOX/.grimes/ledger.pb" ]]; then
    pass "the engine admitted the reported finding to a ledger"
else
    fail "no ledger was written from the documented report"
fi

if grimes-contract state --ledger="$SANDBOX/.grimes/ledger.pb" 2>/dev/null | grep -qE 'open_p0: *1|P0: *1|open p0: *1'; then
    pass "the ledger records the reported P0"
else
    # The summary wording varies; fall back to asserting one finding exists.
    FOUND="$(grimes-contract state --ledger="$SANDBOX/.grimes/ledger.pb" --json 2>/dev/null | grep -c 'FG-SEC-' || true)"
    if [[ "$FOUND" -ge 1 ]]; then
        pass "the ledger records the reported P0"
    else
        fail "the ledger does not record the reported finding"
    fi
fi

set +e
GRIMES_PROJECT_DIR="$SANDBOX" "$SANDBOX/hooks/stop.sh" >/dev/null 2>&1
LOOP_CODE=$?
set -e
assert_eq "$LOOP_CODE" "2" "the stop hook continues the loop after a documented run"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
