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

assert_match() {
    if grep -qE "$2" <<<"$1"; then pass "$3"; else fail "$3"; fi
}

assert_no_match() {
    if grep -qE "$2" <<<"$1"; then fail "$3"; else pass "$3"; fi
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
    # Two lines, because an escalation has to be able to cite evidence the
    # milder report did not, and every citation has to be real.
    # shellcheck disable=SC2016  # the text is the target's content, not an expansion
    printf 'rm -rf ./build/*\neval "$UNTRUSTED"\n' >"$dir/src/app.sh"
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
echo "--- A role's pass is recorded whether or not it succeeded ---"

# The pass worth diagnosing is the one that failed, so the record is written
# before the failure is returned. No prompt text produces it.
WS="$(workspace)"
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-exit7.sh")"
assert_eq "$CODE" "1" "a failing provider fails the run"
LOG="$WS/.grimes/work/primary.log"
if [[ -f "$LOG" ]]; then
    pass "the engine recorded the failing pass"
else
    fail "no record of the failing pass"
fi
# What the role emitted, and what became of it: a record naming neither is not
# a diagnosis.
if grep -q 'GRIMES_REPORT_PROTOBUF_V2_BEGIN' "$LOG" 2>/dev/null; then
    pass "the record holds what the role wrote"
else
    fail "the record does not hold what the role wrote"
fi
if grep -qE 'exit: .*7' "$LOG" 2>/dev/null; then
    pass "the record names how the role exited"
else
    fail "the record does not name how the role exited"
fi
rm -rf "$WS"

# A pass that worked leaves the same record; diagnosis is not reserved for
# failure. The run is conditional rather than clean because no adjudicator was
# asked for, but it reached a verdict, which an operational failure does not.
WS="$(workspace)"
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-green.sh")"
assert_eq "$CODE" "3" "the green run reached a verdict"
LOG="$WS/.grimes/work/primary.log"
if [[ -f "$LOG" ]]; then
    pass "a passing role is recorded too"
else
    fail "a passing role left no record"
fi
# Without this the assertion above cannot tell a role that worked from one that
# did not: both leave a log.
if grep -q '^exit: 0$' "$LOG" 2>/dev/null; then
    pass "and the record shows it exited cleanly"
else
    fail "the record does not show a clean exit"
fi
rm -rf "$WS"

echo ""
echo "--- Both halves report which build they are ---"

# The two are one contract in two halves and nothing can stop a mismatched pair
# being installed, so each has to be able to say what it is.
for BINARY in "$GRIMES" "$(dirname "$GRIMES")/grimes-contract"; do
    NAME="$(basename "$BINARY")"
    set +e
    OUT="$("$BINARY" --version 2>&1)"
    CODE=$?
    set -e
    assert_eq "$CODE" "0" "$NAME answers --version"
    # Either a commit or an honest unknown. A stamp is absent exactly when the
    # build was one nobody can vouch for, and saying so beats naming a commit
    # these bytes did not come from.
    if grep -qE "^$NAME ([0-9a-f]{12}( \(with uncommitted changes\))?|unknown)$" <<<"$OUT"; then
        pass "$NAME names itself and its build"
    else
        fail "$NAME reported: $OUT"
    fi
done

echo ""
echo "--- A provider that cannot nest its own sandbox is told so ---"

# The engine's report is "no report envelope", which is true and useless: the
# operator still has to recognise a syscall name to find out why.
WS="$(workspace)"
set +e
OUT="$("$GRIMES" run --dir="$WS" --provider-command="$FAKES/provider-nested-sandbox.sh" src 2>&1)"
CODE=$?
set -e
assert_eq "$CODE" "1" "a provider refused a nested sandbox fails the run"
if grep -q 'second sandbox inside this one' <<<"$OUT"; then
    pass "and the refusal names the collision"
else
    fail "the refusal does not name the collision"
fi
# Naming --unsafe as the way out would trade the engine's boundary for the
# provider's, which is the opposite of what is wanted.
if grep -q 'waives the engine' <<<"$OUT"; then
    pass "and says why --unsafe is the wrong reach"
else
    fail "the refusal does not warn against --unsafe"
fi
rm -rf "$WS"

echo ""
echo "--- Invalid provider output fails closed ---"

for fake in provider-garbage provider-noenvelope provider-exit7 provider-sealed-but-silent provider-silent; do
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

# A provider that exits cleanly says nothing further about itself, so what it
# wrote is the only account of why the run failed. Without it the operator
# cannot tell a provider that reviewed and then described its work from one
# that never ran a review at all.
WS="$(workspace)"
OUT="$("$GRIMES" run --dir="$WS" --provider-command="$FAKES/provider-sealed-but-silent.sh" \
    --format=prototext src 2>&1 || true)"
assert_match "$OUT" 'no report envelope' "a provider that sealed nothing to stdout fails the run"
assert_match "$OUT" 'exited cleanly' "and the refusal says the provider did not itself fail"
assert_match "$OUT" 'I found one issue' "and quotes what the provider put on stdout"
# Where an agent CLI leaves its account of what it did.
assert_match "$OUT" 'report seal' "and quotes what the provider put on stderr"
assert_match "$OUT" "provider's own stdout" "and says where a report has to arrive"
rm -rf "$WS"

# Nothing to quote, which is itself the answer: the provider never spoke.
WS="$(workspace)"
OUT="$("$GRIMES" run --dir="$WS" --provider-command="$FAKES/provider-silent.sh" \
    --format=prototext src 2>&1 || true)"
assert_match "$OUT" 'no output at all' "a silent provider is reported as having written nothing"
assert_no_match "$OUT" 'ending:' "and nothing is quoted back that it did not write"
rm -rf "$WS"

echo ""
echo "--- A sealed report is delivered by the contract CLI, not by what a model says ---"

# The failure this closes: a provider reviews, seals, and then summarises its
# work. The envelope existed, inside that CLI's own transcript, and never
# reached the engine. Delivery is now something the contract CLI did.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-seals-then-talks.sh" --format=prototext)"
assert_no_match "$OUT" 'no report envelope' \
    "a provider that sealed and then talked is not rejected"
assert_match "$OUT" 'candidates_examined: +4' \
    "and the report it sealed is the one the engine read"
if [[ -f "$WS/.grimes/ledger.pb" ]]; then
    pass "and the run reached a ledger"
else
    fail "the run reached no ledger"
fi
# Nothing may outlive the pass that sealed it: a later iteration that sealed
# nothing would otherwise collect this one and re-report its findings.
if compgen -G "$WS/.grimes/work/*.envelope" >/dev/null; then
    fail "the sealed envelope outlived the pass that wrote it"
else
    pass "the sealed envelope does not outlive its pass"
fi
rm -rf "$WS"

# An envelope named for some other pass is not this pass's report. Left
# collectable, it would answer a request nobody made it for.
# Marked bytes rather than a real report: what is under test is whether the
# engine reaches for a file this pass did not write, and an engine that does
# reach for it fails on the contents instead, which is a different refusal.
WS="$(workspace)"
mkdir -p "$WS/.grimes/work"
{
    echo "GRIMES_REPORT_PROTOBUF_V2_BEGIN"
    echo "bm90IHRoaXMgcGFzcydzIHJlcG9ydA=="
    echo "GRIMES_REPORT_PROTOBUF_V2_END"
} >"$WS/.grimes/work/0000000000000000.envelope"
OUT="$("$GRIMES" run --dir="$WS" --provider-command="$FAKES/provider-noenvelope.sh" \
    --format=prototext src 2>&1 || true)"
assert_match "$OUT" 'no report envelope' \
    "an envelope belonging to another pass is not collected"
if [[ -f "$WS/.grimes/work/0000000000000000.envelope" ]]; then
    pass "and another pass's envelope is left where it was"
else
    fail "another pass's envelope was consumed"
fi
rm -rf "$WS"

# An adjudicator is a spawned role like any other, and its verdict reaches the
# engine the same way. A reviewer that sealed and then described its verdict
# has still reviewed; losing it would read as a reviewer that did not answer.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-seals-then-talks.sh" \
    --adjudicator-fresh --format=prototext)"
assert_match "$OUT" 'requested: +1' "one reviewer was asked for"
if [[ "$(grep -cE '^    reviewer_id:' <<<"$OUT")" == "1" ]]; then
    pass "an adjudicator that sealed and then talked is counted as having answered"
else
    fail "an adjudicator that sealed and then talked is counted as having answered"
fi
assert_no_match "$OUT" 'unmet_gates: +"adjudication"' \
    "and the adjudication gate is met"
rm -rf "$WS"

# A role can put something other than a file where the engine collects. This
# read happens after the process has been waited on, so nothing else would have
# interrupted it.
WS="$(workspace)"
set +e
"$GRIMES" run --dir="$WS" --provider-command="$FAKES/provider-fifo-envelope.sh" src \
    >/dev/null 2>&1 &
FIFO_PID=$!
WAITED=0
while kill -0 "$FIFO_PID" 2>/dev/null && [[ "$WAITED" -lt 30 ]]; do
    sleep 1
    WAITED=$((WAITED + 1))
done
if kill -0 "$FIFO_PID" 2>/dev/null; then
    kill -9 "$FIFO_PID" 2>/dev/null
    wait "$FIFO_PID" 2>/dev/null
    CODE=124
else
    wait "$FIFO_PID"
    CODE=$?
fi
set -e
if [[ "$CODE" == "124" ]]; then
    fail "the engine waited on a pipe a role left where its report goes"
else
    pass "a role that left a pipe where its report goes does not hold the engine"
fi
assert_eq "$CODE" "1" "and the run fails for want of a report"
rm -rf "$WS"

# Sealed and then dead. The envelope was delivered but never collected, and a
# repeat of this run, role and iteration computes the same path.
WS="$(workspace)"
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-seals-then-fails.sh")"
assert_eq "$CODE" "1" "a provider that sealed and then died fails the run"
if compgen -G "$WS/.grimes/work/*.envelope" >/dev/null; then
    fail "a failed pass left its sealed report for the next one to collect"
else
    pass "a failed pass leaves no sealed report behind"
fi
# The retry has to produce its own answer rather than inherit one.
OUT="$("$GRIMES" run --dir="$WS" --provider-command="$FAKES/provider-noenvelope.sh" \
    --format=prototext src 2>&1 || true)"
assert_match "$OUT" 'no report envelope' "and a retry is not answered by it"
rm -rf "$WS"

# A verdict reached for another run, over bytes that happen to match. The
# fingerprint admits it; the run is what does not.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-other-run.sh" \
    --adjudicator-fresh --format=prototext)"
assert_match "$OUT" 'requested: +1' "a reviewer answering for another run was still asked for"
if [[ "$(grep -cE '^    reviewer_id:' <<<"$OUT")" == "0" ]]; then
    pass "and its verdict is not counted as an answer"
else
    fail "a verdict reached for another run was admitted"
fi
assert_match "$OUT" 'unmet_gates: +"adjudication"' \
    "and the run is left without adjudication"
assert_no_match "$OUT" 'decision: +DECISION_PASS' \
    "so it cannot carry the run to a pass"
rm -rf "$WS"

echo ""
echo "--- Output and time are bounded ---"

# Volume is not a failure. A role that talks past the bound and then finishes
# keeps its tail and its pass; only a role that never stops is ended, and the
# timeout is what ends it.
WS="$(workspace)"
START=$(date +%s)
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-flood.sh" --max-output-bytes=4096 --provider-timeout=5s)"
ELAPSED=$(($(date +%s) - START))
assert_eq "$CODE" "1" "a provider that never stops fails the run"
if [[ "$ELAPSED" -lt 60 ]]; then
    pass "the timeout ends it promptly (${ELAPSED}s)"
else
    fail "an unbounded provider ran ${ELAPSED}s"
fi
# Bounded on disk as well as in memory: the record is the tail, not the flood.
LOGSIZE=$(wc -c <"$WS/.grimes/work/primary.log" 2>/dev/null || echo 0)
if [[ "$LOGSIZE" -lt 100000 ]]; then
    pass "the record is bounded (${LOGSIZE} bytes)"
else
    fail "the record holds ${LOGSIZE} bytes of an unbounded provider"
fi
rm -rf "$WS"

# A descendant that leaves the process group survives the group kill and keeps
# the pipe open. Every other fake stays in the group, so nothing else covers it.
#
# The run still ends: os/exec closes its own pipes once WaitDelay expires, which
# bounds the read no signal can reach. The bound is the deadline plus that delay,
# not the deadline alone.
WS="$(workspace)"
START=$(date +%s)
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-reparent.sh" --provider-timeout=2s)"
ELAPSED=$(($(date +%s) - START))
assert_eq "$CODE" "1" "a provider whose descendant escapes the group fails the run"
if [[ "$ELAPSED" -lt 30 ]]; then
    pass "an escaped descendant does not hold the run open (${ELAPSED}s)"
else
    fail "an escaped descendant held the run for ${ELAPSED}s"
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
# The fake refuses to adjudicate if it was handed findings, evidence, a ledger,
# a severity, a summary, or the claimed verdict. A refusal reaches the engine as
# a failed adjudication, which the engine treats as absence rather than as a run
# failure — so a leak shows up here, as a review that was never recorded. The
# fake's own message cannot be asserted directly: it goes to stderr and the
# engine swallows it. internal/provider asserts the environment directly.
#
# Unknown is the honest default: nothing about an opaque command says whether
# it opened a new session or reused the caller's.
if echo "$OUT" | grep -qE 'context_origin: +CONTEXT_ORIGIN_UNKNOWN'; then
    pass "an adjudicated run records the independent review as unknown-origin"
else
    fail "an adjudicated run does not record the context origin"
fi
# A second opinion that may have seen the first cannot raise confidence to the
# level a pass requires. Asserted through the gate rather than by grepping the
# confidence value: the adjudicator states a tuple of its own inside the record,
# so a bare grep would match its claim rather than the engine's derivation.
if echo "$OUT" | grep -qE 'unmet_gates: +"review_confidence"'; then
    pass "an unknown-origin second opinion cannot reach high confidence"
else
    fail "an unknown-origin second opinion reached high confidence"
fi
if echo "$OUT" | grep -qE 'unmet_gates: +"independent_context"'; then
    pass "the record names the context as the unmet gate"
else
    fail "the record does not name the context gate"
fi
rm -rf "$WS"

WS="$(workspace)"
CODE="$(exit_code "$WS" \
    --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-block.sh")"
assert_eq "$CODE" "4" "an independent block produces a blocking exit code"
rm -rf "$WS"

# The asymmetry: an unknown-origin reviewer may make a verdict worse but never
# better. Capping its confidence must not also mute its objection.
WS="$(workspace)"
CODE="$(exit_code "$WS" \
    --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-block.sh")"
assert_eq "$CODE" "4" "an unknown-origin reviewer can still block"
rm -rf "$WS"

# The operator is the only party who can say a command begins a fresh context.
WS="$(workspace)"
OUT="$(run_grimes "$WS" \
    --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" \
    --adjudicator-fresh --format=prototext)"
if echo "$OUT" | grep -qE 'context_origin: +CONTEXT_ORIGIN_ENGINE_SPAWNED'; then
    pass "an operator-asserted fresh context is recorded as engine-spawned"
else
    fail "an operator-asserted fresh context was not recorded"
fi
if echo "$OUT" | grep -qE 'unmet_gates: +"independent_context"'; then
    fail "a known-origin context still names the context gate"
else
    pass "a known-origin context clears the context gate"
fi
rm -rf "$WS"

# The flag asserts something about a command, so it needs one.
WS="$(workspace)"
CODE="$(exit_code "$WS" --provider-command="$FAKES/provider-green.sh" --adjudicator-fresh)"
assert_eq "$CODE" "1" "--adjudicator-fresh without an adjudicator is refused"
rm -rf "$WS"

# The wiring every adapter documents, run end to end: a second context that
# hands its tuple back through `grimes-contract adjudicate` and is told the run
# only through the environment the engine exported.
WS="$(workspace)"
OUT="$(run_grimes "$WS" \
    --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-documented.sh" \
    --adjudicator-fresh --format=prototext)"
if echo "$OUT" | grep -qE 'context_origin: +CONTEXT_ORIGIN_ENGINE_SPAWNED'; then
    pass "the documented adjudicator wiring reaches the engine"
else
    fail "the documented adjudicator wiring reaches the engine"
fi
if echo "$OUT" | grep -qE 'unmet_gates: +"adjudication"'; then
    fail "the documented wiring still leaves adjudication unmet"
else
    pass "the documented wiring clears the adjudication gate"
fi
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
echo "--- A pass delivers only what it opened ---"

# The first pass adds a P0 and dies without sealing. The engine clears what its
# own pass left, so the next one finds nothing to inherit.
WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-abandons.sh" >/dev/null
if [[ ! -f "$WS/.grimes/work/report.textproto" ]]; then
    pass "a pass that died leaves no report behind"
else
    fail "a pass that died leaves no report behind"
fi
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-in-place.sh" --format=prototext)"
assert_match "$OUT" 'total: +1' "the next pass delivers only its own candidate"
assert_no_match "$OUT" 'open_p0' "and not the P0 the abandoned pass had opened"
rm -rf "$WS"

# The default path is a default, not a rule. A pass that accumulated into a file
# of its own still leaves it to the pass that spawned it.
WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-abandons-elsewhere.sh" >/dev/null
# The engine's own role logs live here too; what this is about is the report
# the role opened and the marker claiming it.
LEFT="$(find "$WS/.grimes/work" -type f ! -name '*.log' 2>/dev/null | wc -l | tr -d ' ')"
if [[ "$LEFT" == "0" ]]; then
    pass "a pass that named its own report leaves neither it nor its marker"
else
    fail "a pass that named its own report leaves neither it nor its marker ($LEFT left)"
fi
rm -rf "$WS"

# With the report left in place by hand, the refusal is what stands between one
# pass's candidates and another pass's report.
WS="$(workspace)"
mkdir -p "$WS/.grimes/work"
printf 'someone-elses-pass' >"$WS/.grimes/work/report.textproto.pass"
: >"$WS/.grimes/work/report.textproto"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-in-place.sh" --format=prototext)"
assert_match "$OUT" 'another pass' \
    "a report another pass opened cannot be added to"
assert_no_match "$OUT" 'legacy_color' "and the run produces no result"
rm -rf "$WS"

# A pass that adds nothing still seals whatever it finds, which is how an
# abandoned report reaches a record without anyone reporting its candidates.
WS="$(workspace)"
mkdir -p "$WS/.grimes/work"
(cd "$WS" && GRIMES_PASS=someone-elses-pass GRIMES_TARGET_ROOT="$WS" GRIMES_TARGET_KIND=code \
    "$BINDIR/grimes-contract" report add --category=SEC --severity=P0 --blast=systemic \
    --likelihood=likely --path=src/app.sh --tier=E2 --claim="a claim nobody sealed" \
    --quote="$(grep -m1 -E '[^[:space:]]' "$WS/src/app.sh")" \
    --disproof-action="tried" --disproof-exit=1 --disproof-output="still there" >/dev/null)
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-bare-seal.sh" --format=prototext)"
assert_match "$OUT" 'not this pass' \
    "a report another pass opened is not sealed by the pass that found it"
assert_no_match "$OUT" 'legacy_color' "and that run produces no result either"
rm -rf "$WS"

echo ""
echo "--- A panel is every reviewer that was asked for ---"

# Two reviewers, both of which answer. The record says so, and neither the
# count nor the reviewers' own identities come from anything but the run.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" \
    --adjudicator-command="$FAKES/adjudicator-agrees.sh" \
    --adjudicator-fresh --format=prototext)"
assert_match "$OUT" 'requested: +2' "a panel records how many reviewers it asked for"
if [[ "$(grep -cE '^    reviewer_id:' <<<"$OUT")" == "2" ]]; then
    pass "and records each one that answered"
else
    fail "and records each one that answered"
fi
assert_no_match "$OUT" 'unmet_gates: +"adjudication"' \
    "a panel that answered in full clears the adjudication gate"
rm -rf "$WS"

# One of the two never answers. The reviews that did arrive look exactly like a
# smaller panel nobody asked for, which is what the count is for.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" \
    --adjudicator-command="$FAKES/adjudicator-absent.sh" \
    --adjudicator-fresh --format=prototext)"
assert_match "$OUT" 'requested: +2' "a reviewer that did not answer is still one that was asked for"
assert_match "$OUT" 'unmet_gates: +"adjudication"' \
    "and a short panel leaves the adjudication gate unmet"
assert_no_match "$OUT" 'legacy_color: +LEGACY_COLOR_GREEN' \
    "a short panel cannot reach a pass"
rm -rf "$WS"

# The strictest of them decides, and the record names that reviewer as the one
# whose verdict stood, whichever order they were asked in.
for ORDER in "pass:block" "block:pass"; do
    FIRST="${ORDER%%:*}"
    SECOND="${ORDER##*:}"
    WS="$(workspace)"
    OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" \
        --adjudicator-command="$FAKES/adjudicator-$FIRST.sh" \
        --adjudicator-command="$FAKES/adjudicator-$SECOND.sh" \
        --adjudicator-fresh --format=prototext)"
    assert_match "$OUT" 'legacy_color: +LEGACY_COLOR_RED' \
        "a panel holding one block decides as that reviewer did ($FIRST then $SECOND)"
    rm -rf "$WS"
done

# The reviewer that answered blocked, and the one that did not answer must not
# be able to soften that: a shortfall caps a pass, it does not cap a block.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-block.sh" \
    --adjudicator-command="$FAKES/adjudicator-absent.sh" \
    --adjudicator-fresh --format=prototext)"
assert_match "$OUT" 'legacy_color: +LEGACY_COLOR_RED' \
    "a block from the reviewer that answered survives the one that did not"
rm -rf "$WS"

# A reviewer whose context nobody established makes the whole panel uncertain,
# even beside one whose context is known.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" \
    --adjudicator-command="$FAKES/adjudicator-agrees.sh" --format=prototext)"
assert_match "$OUT" 'unmet_gates: +"independent_context"' \
    "an unattested panel is recorded as unknown-origin"
rm -rf "$WS"

echo ""
echo "--- The flags mean what they say ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-arg=-p --adjudicator-command="$FAKES/adjudicator-pass.sh" --format=prototext)"
assert_match "$OUT" 'needs --adjudicator-command' \
    "an argument before any reviewer belongs to no reviewer"
rm -rf "$WS"

echo ""
echo "--- Fix mode is available for a repository ---"

# Not a repository, so the run is refused for that and not for the mode.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --mode=fix)"
assert_match "$OUT" 'git repository' "fix mode is refused for want of a repository, not by the flag"
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

# A second iteration over the same report adds no new P0/P1. Nothing attacked
# the surviving findings, so the loop stops without having finished: silence is
# this reviewer out of ideas, not the target out of defects.
WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --auto-loop >/dev/null
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-red.sh" --auto-loop --format=prototext)"
if echo "$OUT" | grep -qE 'completion_state: +COMPLETION_STATE_BOUNDED'; then
    pass "a quiet iteration short of refutation records itself as bounded"
else
    fail "a quiet iteration short of refutation does not record itself as bounded"
fi
if echo "$OUT" | grep -qE 'iteration: +2'; then
    pass "the second run is recorded as iteration 2"
else
    fail "the second run is not recorded as iteration 2"
fi
set +e
LOOP_OUT="$("$GRIMES" loop --dir="$WS" 2>/dev/null)"
LOOP_CODE=$?
set -e
assert_eq "$LOOP_CODE" "0" "the loop agrees the bounded review ends"
if echo "$LOOP_OUT" | grep -q 'stopped short'; then
    pass "the loop says it stopped short rather than completed"
else
    fail "the loop reports a bounded review as complete"
fi
if echo "$LOOP_OUT" | grep -q 'refutation remains unmet'; then
    pass "the loop names what the review stopped short of"
else
    fail "the loop does not name what the review stopped short of"
fi
rm -rf "$WS"

# The same quiet iteration, with every surviving claim put to a context that
# broke a control first. Now the silence is an exhausted review.
WS="$(workspace)"
COMPLETE=(--provider-command="$FAKES/provider-red.sh"
    --refuter-command="$FAKES/refuter-upheld.sh" --refuter-fresh --auto-loop)
run_grimes "$WS" "${COMPLETE[@]}" >/dev/null
OUT="$(run_grimes "$WS" "${COMPLETE[@]}" --format=prototext)"
if echo "$OUT" | grep -qE 'completion_state: +COMPLETION_STATE_REVIEW_COMPLETE'; then
    pass "a quiet iteration with coverage and refutation exhausted records completion"
else
    fail "a quiet iteration with coverage and refutation exhausted does not record completion"
fi
rm -rf "$WS"

# Refutation exhausted, coverage not: a routed category that never reached a
# stop leaves the review bounded however quiet the iteration was.
WS="$(workspace)"
SHORT=(--provider-command="$FAKES/provider-drops-category.sh"
    --refuter-command="$FAKES/refuter-upheld.sh" --refuter-fresh --auto-loop)
run_grimes "$WS" "${SHORT[@]}" >/dev/null
OUT="$(run_grimes "$WS" "${SHORT[@]}" --format=prototext)"
if echo "$OUT" | grep -qE 'completion_state: +COMPLETION_STATE_BOUNDED'; then
    pass "a routed category short of its stop keeps the review bounded"
else
    fail "a routed category short of its stop does not keep the review bounded"
fi
rm -rf "$WS"

SKILL="$PROJECT_ROOT/skills/frank-grimes/SKILL.md"
if grep -q 'it may not end the review on its own' "$SKILL"; then
    pass "the skill refuses to let marginal yield end a review"
else
    fail "the skill lets marginal yield end a review"
fi
if grep -q 'Bounded is an honest ending' "$SKILL"; then
    pass "the skill names the ending a review short of its conditions gets"
else
    fail "the skill does not name the ending a review short of its conditions gets"
fi

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
echo "--- A reworded claim is a successor, not a discovery ---"

ledger_field() {
    "$BINDIR/grimes-contract" decode --type=Ledger "$1/.grimes/ledger.pb" 2>/dev/null | grep -aE "$2" || true
}

FIRST="the retention clause contradicts the stated period"
REWORD="the stated period conflicts with the retention clause"

# The same defect described twice derives two IDs. Without a link the ledger
# holds two findings where there is one, and the reword reads as news.
WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-claim.sh $FIRST" --auto-loop >/dev/null
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-claim.sh $REWORD" --auto-loop --format=prototext)"
if ledger_field "$WS" 'supersedes: ' | grep -q 'FG-'; then
    pass "a reworded claim names the finding it replaces"
else
    fail "a reworded claim was recorded with no link: $(ledger_field "$WS" 'key:|claim:')"
fi
if [[ "$(ledger_field "$WS" 'status: +FINDING_STATUS_SUPERSEDED' | wc -l | tr -d ' ')" == "1" ]]; then
    pass "the claim it replaces is marked superseded"
else
    fail "the replaced claim was left standing"
fi
if grep -qE 'total: +1$' <<<"$OUT"; then
    pass "one defect described twice counts once"
else
    fail "a reworded claim was counted as a second finding: $(grep -aE 'total:' <<<"$OUT")"
fi
if grep -qE 'new_p2_p3: +[1-9]' <<<"$OUT"; then
    fail "a successor was counted toward the iteration's yield"
else
    pass "a successor is not counted toward the iteration's yield"
fi
rm -rf "$WS"

# A defect recorded as fixed, then described again in different words, is the
# same news as one that came back under its own identity.
WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-claim.sh $FIRST" --auto-loop >/dev/null
FIXED_ID="$(ledger_field "$WS" 'key: ' | head -1 | sed -E 's/.*"(.*)".*/\1/')"
"$BINDIR/grimes-contract" ledger transition --ledger="$WS/.grimes/ledger.pb" \
    --id="$FIXED_ID" --to=fixed --iteration=1 --actor=test >/dev/null
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-claim.sh $REWORD" --auto-loop --format=prototext)"
if grep -qE 'oscillation_detected: +true' <<<"$OUT"; then
    pass "a fixed finding restated in new words is not a fresh discovery"
else
    fail "a fixed-then-reworded finding read as new: $(grep -aE 'oscillation|total:' <<<"$OUT")"
fi
rm -rf "$WS"

# Two records at one anchor in one category are two defects nothing here can
# tell apart, so a third claim links to neither rather than guessing.
WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-claim.sh $FIRST -- and another thing entirely" --auto-loop >/dev/null
run_grimes "$WS" --provider-command="$FAKES/provider-claim.sh $REWORD" --auto-loop >/dev/null
if ledger_field "$WS" 'supersedes: ' | grep -q 'FG-'; then
    fail "an ambiguous restatement picked one of two records to replace"
else
    pass "an ambiguous restatement replaces nothing"
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
