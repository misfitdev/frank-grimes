#!/usr/bin/env bash
# FG-105: the orchestrator derives its own result and fails closed.
#
# Black-box: everything here goes through the grimes CLI with a fake provider,
# the same way an adapter must. Nothing reaches into the Go packages directly.
#
# Exit codes: 0 all passed, 1 a test failed, 2 the toolchain is unavailable.

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAKES="$PROJECT_ROOT/tests/fakes"

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

if ! command -v go &>/dev/null; then
    echo -e "${YELLOW}SKIP${NC}: go is not installed; orchestrator tests require the toolchain"
    exit 2
fi

BINDIR="$(mktemp -d)"
trap 'rm -rf "$BINDIR"' EXIT
GRIMES="$BINDIR/grimes"
# The provider runs with a minimal environment and inherits only PATH, so the
# fakes find grimes-contract there rather than through a bespoke variable.
export PATH="$BINDIR:$PATH"

echo "--- Build ---"
if (cd "$PROJECT_ROOT" && go build -o "$GRIMES" ./cmd/grimes) 2>/dev/null &&
    (cd "$PROJECT_ROOT" && go build -o "$BINDIR/grimes-contract" ./cmd/grimes-contract) 2>/dev/null; then
    pass "grimes and grimes-contract build"
else
    fail "grimes and grimes-contract build"
    echo "Passed: $PASSED / Failed: $FAILED"
    exit 1
fi

# Each case gets its own workspace so a leftover ledger cannot leak between them.
#
# The collector fingerprints a target's content, so a target has to exist. Both
# trees are written here: "src" is what run_grimes reviews, and "other-target"
# is the second, differently-fingerprinted target the ledger must refuse.
workspace() {
    local dir
    dir="$(mktemp -d)"
    mkdir -p "$dir/src" "$dir/other-target"
    printf 'rm -rf ./build/*\n' >"$dir/src/app.sh"
    printf 'echo other\n' >"$dir/other-target/app.sh"
    echo "$dir"
}

run_grimes() {
    local dir="$1"
    shift
    "$GRIMES" run --dir="$dir" "$@" src 2>&1 || true
}

exit_code() {
    local dir="$1"
    shift
    "$GRIMES" run --dir="$dir" "$@" src >/dev/null 2>&1
    echo $?
}

echo ""
# prototext's spacing is randomized per binary build
# (google.golang.org/protobuf/internal/detrand), so match on content, not
# exact spacing.
echo "--- A provider cannot certify its own review ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" --format=prototext)"
if echo "$OUT" | grep -qE 'legacy_color: +LEGACY_COLOR_GREEN'; then
    fail "engine adopted the provider's GREEN"
else
    pass "a GREEN-claiming provider does not produce GREEN"
fi
if echo "$OUT" | grep -qE 'producer_role: +PRODUCER_ROLE_ORCHESTRATOR'; then
    pass "the emitted result is attributed to the orchestrator"
else
    fail "result is not attributed to the orchestrator"
fi
if echo "$OUT" | grep -qE 'unmet_gates: +"adjudication"'; then
    pass "a run with no adjudicator names adjudication as unmet"
else
    fail "a run with no adjudicator does not name adjudication"
fi
rm -rf "$WS"

echo ""
echo "--- The emitted result satisfies the machine contract ---"

WS="$(workspace)"
ENVELOPE="$(mktemp)"
run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" >"$ENVELOPE"
if grimes-contract decode-result "$ENVELOPE" >/dev/null 2>&1; then
    pass "the emitted envelope decodes and revalidates"
else
    fail "the emitted envelope does not revalidate"
fi
if [[ -f "$WS/.grimes/ledger.pb" ]]; then
    pass "a completed run persists the ledger"
else
    fail "a completed run did not persist the ledger"
fi
if [[ -f "$WS/.grimes-state.json" ]]; then
    fail "the engine wrote the hook's state file"
else
    pass "the engine leaves .grimes-state.json to the stop hook"
fi
rm -f "$ENVELOPE"
rm -rf "$WS"

echo ""
echo "--- Invalid provider output fails closed ---"

for fake in provider-garbage provider-noenvelope provider-exit7; do
    WS="$(workspace)"
    CODE="$(exit_code "$WS" --provider-command="$FAKES/$fake.sh")"
    assert_eq "$CODE" "1" "$fake fails the run"
    if [[ -f "$WS/.grimes/ledger.pb" ]]; then
        fail "$fake wrote a ledger despite failing"
    else
        pass "$fake leaves no ledger behind"
    fi
    rm -rf "$WS"
done

echo ""
echo "--- Output and time are bounded ---"

WS="$(workspace)"
START=$(date +%s)
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-flood.sh" --max-output-bytes=4096)"
ELAPSED=$(($(date +%s) - START))
assert_eq "$CODE" "1" "an unbounded provider fails the run"
if [[ "$ELAPSED" -lt 30 ]]; then
    pass "the output bound trips promptly (${ELAPSED}s)"
else
    fail "the output bound took ${ELAPSED}s"
fi
rm -rf "$WS"

WS="$(workspace)"
START=$(date +%s)
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-hang.sh" --provider-timeout=2s)"
ELAPSED=$(($(date +%s) - START))
assert_eq "$CODE" "1" "a hanging provider fails the run"
if [[ "$ELAPSED" -lt 30 ]]; then
    pass "the timeout kills a provider holding the pipe open (${ELAPSED}s)"
else
    fail "the timeout took ${ELAPSED}s"
fi
rm -rf "$WS"

echo ""
echo "--- Adjudication ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" \
    --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --format=prototext)"
if echo "$OUT" | grep -qE 'zero_knowledge: +true'; then
    pass "an adjudicated run records the independent review"
else
    fail "an adjudicated run does not record the independent review"
fi
if echo "$OUT" | grep -q 'adjudicator received'; then
    fail "the adjudicator was handed findings or evidence"
else
    pass "the adjudicator receives only the target and the claimed tuple"
fi
rm -rf "$WS"

WS="$(workspace)"
CODE="$(exit_code "$WS" \
    --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-block.sh")"
assert_eq "$CODE" "4" "an independent block produces a blocking exit code"
rm -rf "$WS"

echo ""
echo "--- The loop runs only when it was asked for ---"

WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" >/dev/null
if [[ -f "$WS/.grimes/state.pb" ]]; then
    fail "a run without --auto-loop left loop state"
else
    pass "a run without --auto-loop leaves no loop state"
fi
if [[ -f "$WS/.grimes/result.pb" ]]; then
    pass "a one-shot run still records its result"
else
    fail "a one-shot run recorded no result"
fi
rm -rf "$WS"

WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --auto-loop >/dev/null
if [[ -f "$WS/.grimes/state.pb" ]]; then
    pass "--auto-loop records loop state"
else
    fail "--auto-loop recorded no loop state"
fi
rm -rf "$WS"

echo ""
echo "--- A ledger belongs to one target ---"

WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" >/dev/null
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-red.sh")"
if [[ "$CODE" == "1" ]]; then
    fail "a second run against the same target failed"
else
    pass "the same target reuses its ledger"
fi

# A second target in the same directory must not inherit the first one's
# findings: the contract pins the ledger to a single path.
set +e
OTHER="$("$GRIMES" run --dir="$WS" --provider-command="$FAKES/provider-red.sh" other-target 2>&1)"
OTHER_CODE=$?
set -e
# Both halves matter: printing the reason while exiting zero would still let a
# caller treat the run as having succeeded.
if [[ "$OTHER_CODE" == "1" ]] && grep -qi 'different target' <<<"$OTHER"; then
    pass "a ledger raised against another target is refused"
else
    fail "a second target reused the first target's ledger (exit $OTHER_CODE)"
fi
rm -rf "$WS"

echo ""
echo "--- Fix mode is not available ---"

WS="$(workspace)"
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-red.sh" --mode=fix)"
assert_eq "$CODE" "1" "fix mode is refused rather than ignored"
rm -rf "$WS"

echo ""
echo "--- A re-reported finding's severity only ratchets upward ---"

# The finding ID is derived over category, anchor, and claim, so the same claim
# at a worse severity lands on the record the first iteration wrote.
WS="$(workspace)"
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-p2.sh")"
assert_eq "$CODE" "3" "a lone P2 is conditional rather than blocking"
BEFORE="$(grimes-contract state --ledger="$WS/.grimes/ledger.pb" --json)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-p0-escalation.sh" --format=prototext)"
AFTER="$(grimes-contract state --ledger="$WS/.grimes/ledger.pb" --json)"
if echo "$OUT" | grep -qE 'decision: +DECISION_BLOCK'; then
    pass "the same claim re-reported at P0 blocks"
else
    fail "the same claim re-reported at P0 does not block"
fi
if echo "$OUT" | grep -qE 'open_p0: +1'; then
    pass "the escalated finding is counted as an open P0"
else
    fail "the escalated finding is not counted as an open P0"
fi
# Learning that a known finding is critical is new P0/P1 material. Counting it
# as nothing would let the loop stop on the iteration that found the worst news.
if echo "$OUT" | grep -qE 'new_p0_p1: +1'; then
    pass "an escalation counts toward the marginal yield"
else
    fail "an escalation does not count toward the marginal yield"
fi
# One record, not two: the escalation must not mint a second finding.
if [[ "$(echo "$OUT" | grep -cE 'id: +"FG-SEC-')" == "1" ]]; then
    pass "the escalation updates one record rather than adding a second"
else
    fail "the escalation did not leave exactly one finding"
fi

# The severity is the visible half of an escalation; the evidence that earned it
# is the half a verdict is later defended with. A record keeping the old
# evidence under the new severity would claim proof it does not hold.
#
# protojson is a read-only projection, so these are greps over it rather than a
# jq dependency the rest of the suite does not carry.
digest_of() { grep -oE '"evidenceSha256": *"[^"]*"' <<<"$1" | head -1; }
events_in() { grep -c '"iteration"' <<<"$1"; }

if [[ "$(digest_of "$BEFORE")" != "$(digest_of "$AFTER")" ]]; then
    pass "an escalation replaces the evidence digest"
else
    fail "an escalation left the old evidence digest in place"
fi
if grep -q 'eval' <<<"$AFTER" && ! grep -q 'eval' <<<"$BEFORE"; then
    pass "an escalation replaces the evidence itself"
else
    fail "an escalation left the old evidence in place"
fi
# The history is where a reader sees when the finding turned critical.
if [[ "$(events_in "$AFTER")" -gt "$(events_in "$BEFORE")" ]]; then
    pass "an escalation is recorded in the finding's history"
else
    fail "an escalation left no trace in the finding's history"
fi
rm -rf "$WS"

# A provider may report one claim twice in a single report. That is one finding
# surfaced, not two, and counting it twice would overstate the yield the loop
# stops on.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-p2-then-p1.sh" --format=prototext)"
if echo "$OUT" | grep -qE 'new_p0_p1: +1'; then
    pass "one claim reported twice in one report counts once"
else
    fail "one claim reported twice in one report was counted more than once"
fi
rm -rf "$WS"

WS="$(workspace)"
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-p0-escalation.sh")"
assert_eq "$CODE" "4" "a reported P0 blocks"
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-p3-downgrade.sh")"
assert_eq "$CODE" "4" "the same claim re-reported at P3 does not defuse the block"
rm -rf "$WS"

echo ""
echo "--- A mid-loop iteration is not recorded as a finished review ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --auto-loop --format=prototext)"
if echo "$OUT" | grep -qE 'completion_state: +COMPLETION_STATE_CONTINUE'; then
    pass "iteration 1 of 5 records itself as continuing"
else
    fail "iteration 1 of 5 does not record itself as continuing"
fi
# The record and the stop hook must reach the same conclusion, or one of them is
# lying about the same run.
set +e
"$GRIMES" loop --dir="$WS" >/dev/null 2>&1
LOOP_CODE=$?
set -e
assert_eq "$LOOP_CODE" "2" "the loop agrees another iteration is owed"
rm -rf "$WS"

WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --auto-loop --max-iterations=1 --format=prototext)"
if echo "$OUT" | grep -qE 'completion_state: +COMPLETION_STATE_ITERATION_LIMIT'; then
    pass "a run at its bound records the limit rather than completion"
else
    fail "a run at its bound does not record the limit"
fi
rm -rf "$WS"

# A second iteration over the same report adds no new P0/P1, which is where the
# review genuinely ends at RED.
WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --auto-loop >/dev/null
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --auto-loop --format=prototext)"
if echo "$OUT" | grep -qE 'completion_state: +COMPLETION_STATE_REVIEW_COMPLETE'; then
    pass "an exhausted iteration records the review as complete"
else
    fail "an exhausted iteration does not record the review as complete"
fi
if echo "$OUT" | grep -qE 'iteration: +2'; then
    pass "the second run is recorded as iteration 2"
else
    fail "the second run is not recorded as iteration 2"
fi
set +e
"$GRIMES" loop --dir="$WS" >/dev/null 2>&1
LOOP_CODE=$?
set -e
assert_eq "$LOOP_CODE" "0" "the loop agrees the exhausted review ends"
rm -rf "$WS"

echo ""
echo "--- State ---"

WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --auto-loop >/dev/null
if "$GRIMES" state --dir="$WS" --show | grep -qE 'run_id: +'; then
    pass "state --show reports the run in progress"
else
    fail "state --show does not report the run"
fi
"$GRIMES" state --dir="$WS" --clear
if "$GRIMES" state --dir="$WS" --show | grep -q 'no run in progress'; then
    pass "state --clear discards loop state"
else
    fail "state --clear left state behind"
fi
rm -rf "$WS"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
