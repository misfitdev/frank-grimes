#!/bin/bash
#
# Contract and ledger tests (audit FG-101).
#
# Black-box: everything here goes through the grimes-contract CLI, the same way
# the hook and adapters must. Nothing reaches into the Go packages directly.
#
# Usage: ./tests/test-contracts.sh
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#   2 - Toolchain unavailable (Go not installed)
#

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FIXTURES="$PROJECT_ROOT/tests/contracts"

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

# `cond && pass || fail` misreports when pass itself returns non-zero, so the
# assertions go through helpers instead.
assert_eq() {
    if [[ "$1" == "$2" ]]; then pass "$3"; else fail "$3 (got '$1' vs '$2')"; fi
}

assert_ne() {
    if [[ "$1" != "$2" ]]; then pass "$3"; else fail "$3 (both '$1')"; fi
}

if ! command -v go &>/dev/null; then
    echo -e "${YELLOW}SKIP${NC}: go is not installed; contract tests require the toolchain"
    exit 2
fi

BIN="$(mktemp -d)/grimes-contract"
trap 'rm -rf "$(dirname "$BIN")"' EXIT

echo "========================================"
echo "Contract and Ledger Tests"
echo "========================================"
echo ""

echo "--- The codec builds ---"
if (cd "$PROJECT_ROOT" && go build -o "$BIN" ./cmd/grimes-contract) 2>/dev/null; then
    pass "grimes-contract builds"
else
    fail "grimes-contract builds"
    echo "Passed: $PASSED / Failed: $FAILED"
    exit 1
fi

echo ""
echo "--- Identity is content-addressed and stable ---"
# Evidence strings below contain literal $1/$2 shell text, not expansions.
# shellcheck disable=SC2016
ID_A=$("$BIN" id --category=SEC --path=bad-script.sh --evidence='rm -rf "$1"/*' | grep '^id:')
# shellcheck disable=SC2016
ID_B=$("$BIN" id --category=SEC --path=./bad-script.sh --evidence='rm -rf "$1"/*' | grep '^id:')
# shellcheck disable=SC2016
ID_C=$("$BIN" id --category=SEC --path=bad-script.sh --evidence='rm -rf "$2"/*' | grep '^id:')
# shellcheck disable=SC2016
ID_D=$("$BIN" id --category=COR --path=bad-script.sh --evidence='rm -rf "$1"/*' | grep '^id:')

assert_eq "$ID_A" "$ID_B" "path normalization does not change identity"
assert_ne "$ID_A" "$ID_C" "different evidence yields a different identity"
assert_ne "$ID_A" "$ID_D" "category participates in identity"
if [[ "$ID_A" =~ ^id:\ FG-SEC-[0-9a-f]{12}$ ]]; then
    pass "ID matches the schema pattern"
else
    fail "ID does not match the schema pattern: $ID_A"
fi

# Line numbers are excluded on purpose: code moving down a file is not a new
# finding, and re-reporting it as one is how a register fills with duplicates.
EV_FILE="$(mktemp)"
# shellcheck disable=SC2016  # literal evidence text, not an expansion
printf '\n\nrm -rf "$1"/*   \n\n' >"$EV_FILE"
ID_WS=$("$BIN" id --category=SEC --path=bad-script.sh --evidence-file="$EV_FILE" | grep '^id:')
rm -f "$EV_FILE"
assert_eq "$ID_A" "$ID_WS" "surrounding blank lines and trailing space do not change identity"

echo ""
echo "--- Fixtures classify as declared ---"
for fixture in "$FIXTURES"/*.textproto; do
    name="$(basename "$fixture")"
    case "$name" in
        finding.*) msg="Finding" ;;
        result.*) msg="GrimesResult" ;;
        ledger.*) msg="Ledger" ;;
        state.*) msg="LoopState" ;;
        *)
            fail "fixture $name has no recognized type prefix"
            continue
            ;;
    esac

    if "$BIN" validate --type="$msg" --format=textproto "$fixture" &>/dev/null; then
        got="valid"
    else
        got="invalid"
    fi

    if [[ "$name" == *.invalid.* ]]; then
        want="invalid"
    else
        want="valid"
    fi

    assert_eq "$got" "$want" "$name is $want"
done

echo ""
echo "--- Canonical encoding round-trips and rejects tampering ---"
VALID_RESULT="$FIXTURES/result.report-complete.valid.textproto"
BIN_OUT="$(mktemp)"
"$BIN" encode-result --raw "$VALID_RESULT" >"$BIN_OUT"

if "$BIN" validate --type=GrimesResult --format=binary "$BIN_OUT" &>/dev/null; then
    pass "canonical binary re-validates"
else
    fail "canonical binary failed validation"
fi

# Encoding twice must produce identical bytes, or no digest over a result means
# anything and the ledger reference cannot be checked.
BIN_OUT2="$(mktemp)"
"$BIN" encode-result --raw "$VALID_RESULT" >"$BIN_OUT2"
if cmp -s "$BIN_OUT" "$BIN_OUT2"; then
    pass "encoding is deterministic across runs"
else
    fail "encoding is not deterministic"
fi

# Appending a field the schema does not define must be rejected, not ignored.
TAMPERED="$(mktemp)"
cat "$BIN_OUT" >"$TAMPERED"
printf '\xf8\x7f\x01' >>"$TAMPERED"
if "$BIN" validate --type=GrimesResult --format=binary "$TAMPERED" &>/dev/null; then
    fail "unknown trailing field was accepted"
else
    pass "unknown trailing field is rejected"
fi

# The same message re-encoded with a duplicated field decodes to the same value
# but is not the canonical form; round-trip equality is what catches it.
DUPED="$(mktemp)"
cat "$BIN_OUT" "$BIN_OUT" >"$DUPED"
if "$BIN" validate --type=GrimesResult --format=binary "$DUPED" &>/dev/null; then
    fail "non-canonical concatenated encoding was accepted"
else
    pass "non-canonical encoding is rejected"
fi

echo ""
echo "--- Envelope extraction ---"
ENVELOPE="$(mktemp)"
{
    echo "Here is some assistant prose."
    echo "An earlier truncated block: GRIMES_RESULT_PROTOBUF_V2_BEGIN"
    echo ""
    "$BIN" encode-result "$VALID_RESULT"
} >"$ENVELOPE"

if "$BIN" decode-result "$ENVELOPE" | grep -q 'run_id: "run-001"'; then
    pass "last complete envelope is extracted from surrounding prose"
else
    fail "envelope extraction failed"
fi

rm -f "$BIN_OUT" "$BIN_OUT2" "$TAMPERED" "$DUPED" "$ENVELOPE"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
