#!/usr/bin/env bash
# fg-64y.43: confidence comes from surviving an attack, not from self-report.
#
# Black-box: everything goes through the grimes CLI with fake providers.
#
# The property: every other input to review_confidence is something the
# reporting context asserted about its own finding. A run whose findings nobody
# else attacked cannot reach the confidence a pass requires, and one whose
# finding another context broke ranks with an evidence conflict.
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

assert_match() {
    if echo "$1" | grep -qE "$2"; then pass "$3"; else fail "$3"; fi
}

assert_no_match() {
    if echo "$1" | grep -qE "$2"; then fail "$3"; else pass "$3"; fi
}

if ! command -v go &>/dev/null; then
    echo -e "${YELLOW}SKIP${NC}: go is not installed; refutation tests require the toolchain"
    exit 2
fi

BINDIR="$(mktemp -d)"
trap 'rm -rf "$BINDIR"' EXIT
GRIMES="$BINDIR/grimes"
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

workspace() {
    local dir
    dir="$(mktemp -d)"
    mkdir -p "$dir/src"
    # shellcheck disable=SC2016  # the text is the target's content, not an expansion
    printf 'rm -rf ./build/*\neval "$UNTRUSTED"\n' >"$dir/src/app.sh"
    echo "$dir"
}

# The surviving finding is a P2, so the decision reaches a pass and confidence
# is what is left standing between the run and GREEN.
run_grimes() {
    local dir="$1"
    shift
    "$GRIMES" run --dir="$dir" \
        --provider-command="$FAKES/provider-p2.sh" \
        --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
        "$@" --format=prototext src 2>&1 || true
}

echo ""
echo "--- A finding nobody attacked cannot reach high confidence ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS")"
assert_match "$OUT" 'unmet_gates: +"review_confidence"' \
    "a self-reported finding does not reach high confidence"
assert_match "$OUT" 'unmet_gates: +"refutation"' \
    "the record names refutation as what is missing"
rm -rf "$WS"

echo ""
echo "--- Surviving an attack from a separate context raises it ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" \
    --refuter-command="$FAKES/refuter-upheld.sh" --refuter-fresh)"
assert_match "$OUT" 'provenance: +FINDING_PROVENANCE_UPHELD' \
    "the record says the claim survived an attack rather than that it was reported"
assert_no_match "$OUT" 'unmet_gates: +"refutation"' \
    "an upheld claim clears the refutation gate"
assert_no_match "$OUT" 'unmet_gates: +"review_confidence"' \
    "a claim that survived an independent attack reaches high confidence"
rm -rf "$WS"

echo ""
echo "--- A broken claim ranks with an evidence conflict ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" \
    --refuter-command="$FAKES/refuter-refuted.sh" --refuter-fresh)"
assert_match "$OUT" 'review_confidence: +REVIEW_CONFIDENCE_LOW' \
    "a refuted finding forces low confidence"
assert_match "$OUT" 'provenance: +FINDING_PROVENANCE_REFUTED' \
    "the record says the claim was broken"
assert_match "$OUT" 'unmet_gates: +"refutation"' \
    "a refuted finding leaves the refutation gate unmet"
rm -rf "$WS"

echo ""
echo "--- An attack that could not be mounted acquits nothing ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" \
    --refuter-command="$FAKES/refuter-unavailable.sh" --refuter-fresh)"
assert_match "$OUT" 'unmet_gates: +"refutation"' \
    "an unavailable attack does not clear the refutation gate"
assert_match "$OUT" 'unmet_gates: +"review_confidence"' \
    "an unavailable attack does not raise confidence"
rm -rf "$WS"

echo ""
echo "--- fg-64y.44: a refuter has to break a control before it is believed ---"

# Every pass carries one claim the engine established to be false about this
# target. A refuter that upholds it has been shown incapable of refuting, so
# nothing it said about the real claims is recorded.
WS="$(workspace)"
OUT="$(run_grimes "$WS" \
    --refuter-command="$FAKES/refuter-rubberstamp.sh" --refuter-fresh)"
assert_match "$OUT" 'refuter_check: +REFUTER_CHECK_FAILED' \
    "the record says the refuter failed its own check"
assert_match "$OUT" 'provenance: +FINDING_PROVENANCE_UNATTACKED' \
    "a rubber-stamped claim is left unattacked"
assert_match "$OUT" 'unmet_gates: +"refutation"' \
    "a rubber-stamp pass does not clear the refutation gate"
assert_match "$OUT" 'unmet_gates: +"review_confidence"' \
    "a rubber-stamp pass raises nothing"
rm -rf "$WS"

WS="$(workspace)"
OUT="$(run_grimes "$WS" \
    --refuter-command="$FAKES/refuter-upheld.sh" --refuter-fresh)"
assert_match "$OUT" 'refuter_check: +REFUTER_CHECK_PASSED' \
    "a refuter that kills the control is believed"
# The control is a claim about the target that nobody found, so it must not be
# countable as one. The planted identifier appearing anywhere in the record
# would mean it had reached the ledger.
assert_no_match "$OUT" 'fgq[0-9a-f]{13}' \
    "the control never reaches the record"
assert_match "$OUT" 'total: +1' \
    "the control is not counted as a finding"
rm -rf "$WS"

# No refutation pass at all is not a failed check; it is no check.
WS="$(workspace)"
OUT="$(run_grimes "$WS")"
assert_no_match "$OUT" 'refuter_check:' \
    "a run with no refuter records no check rather than a failed one"
rm -rf "$WS"

echo ""
echo "--- The attacker's own context has to be known ---"
# Without --refuter-fresh the engine cannot tell whether the attack came from
# the context that formed the claim. A claim upholding itself is not a survival.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --refuter-command="$FAKES/refuter-upheld.sh")"
assert_match "$OUT" 'unmet_gates: +"refutation"' \
    "an unknown-origin attacker cannot clear the refutation gate"
assert_match "$OUT" 'provenance: +FINDING_PROVENANCE_UNATTACKED' \
    "an unknown-origin upholding leaves the claim unattacked"
assert_match "$OUT" 'unmet_gates: +"review_confidence"' \
    "an unknown-origin attacker cannot raise confidence"
rm -rf "$WS"

echo ""
echo "--- The flags mean what they say ---"

WS="$(workspace)"
"$GRIMES" run --dir="$WS" --provider-command="$FAKES/provider-p2.sh" \
    --refuter-fresh src >/dev/null 2>&1 && CODE=0 || CODE=$?
if [[ "$CODE" == "1" ]]; then
    pass "--refuter-fresh without a refuter is refused"
else
    fail "--refuter-fresh without a refuter is refused (exit $CODE)"
fi
rm -rf "$WS"

echo ""
echo "--- The documented path supplies an attacking context ---"

SKILL="$PROJECT_ROOT/skills/frank-grimes/SKILL.md"
GRIND="$PROJECT_ROOT/adapters/claude-code/commands/grind.md"
AGENT="$PROJECT_ROOT/adapters/claude-code/agents/grimey-refuter.md"

if [[ -f "$AGENT" ]]; then
    pass "the refuter subagent is defined"
else
    fail "the refuter subagent is defined"
fi
assert_match "$(cat "$AGENT" 2>/dev/null)" '^disallowedTools:.*(Write|Edit)' \
    "the refuter is denied Write and Edit at the tool layer"
assert_match "$(cat "$AGENT" 2>/dev/null)" 'Do not report findings of your own' \
    "the refuter is barred from reviewing the target"

# The template the adapter tells the orchestrator to send. Everything it must
# not carry is an argument for the claim rather than a statement of it.
# shellcheck disable=SC2016  # backticks are literal markdown fence characters
PROMPT_BLOCK=$(awk '/^### Refutation$/{f=1;next} f&&/^#{2,3} /{f=0} f' "$GRIND" |
    sed -n '/```text/,/```/p')
if [[ -n "$PROMPT_BLOCK" ]]; then
    pass "the adapter defines an explicit refuter prompt template"
else
    fail "the adapter defines an explicit refuter prompt template"
fi
for leak in 'severity' 'P[0-3]' 'grime[- ]id' 'evidence' 'disproof' 'tier' 'E[123]\b'; do
    assert_no_match "$PROMPT_BLOCK" "$leak" \
        "the refuter prompt template does not leak: $leak"
done

assert_match "$(cat "$SKILL")" 'Silence is not survival' \
    "the skill refuses to read an unmounted attack as survival"
assert_match "$(cat "$SKILL")" 'shown capable of failing before its result means anything' \
    "the skill holds the attacking context to the rule it holds probes to"
assert_match "$(cat "$SKILL")" 'is not a finding and is never counted as one' \
    "the skill keeps the control out of the finding set"
assert_match "$(cat "$GRIND")" 'confirm with Grep that it appears nowhere' \
    "the adapter establishes the control's falsity rather than asserting it"
assert_match "$(cat "$GRIND")" 'do not send it first or last' \
    "the adapter does not let position mark the control"
# A refuter told that one claim is planted can pass the check by hunting for the
# plant instead of attacking anything.
assert_no_match "$(cat "$AGENT")" 'control' \
    "the refuter is not told a control is coming"

echo ""
echo "Passed: $PASSED"
echo "Failed: $FAILED"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
