#!/usr/bin/env bash
# fg-64y.7: a spawned role is bounded to the review's own artifacts.
#
# Black-box: everything goes through the grimes CLI with fake providers.
#
# The property: the engine can say that the bytes a conclusion rests on are the
# bytes that were reviewed, and that no role saw an artifact it was meant to be
# blind to. Two things have to hold for that. A role cannot rewrite the target
# between the pass that found something and the pass that checked it
# (fg-64y.40), and a role cannot read the ledger, the result, the loop state or
# another role's staged copy (fg-64y.49). A run that waives either says so.
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
    if grep -qE "$2" <<<"$1"; then pass "$3"; else fail "$3"; fi
}

assert_no_match() {
    if grep -qE "$2" <<<"$1"; then fail "$3"; else pass "$3"; fi
}

if ! command -v go &>/dev/null; then
    echo -e "${YELLOW}SKIP${NC}: go is not installed; confinement tests require the toolchain"
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

# There is no built-in mechanism on every platform, and the whole suite is about
# what the built-in one does. Where there is none the run is refused by design,
# which is asserted once below rather than mistaken for a failure everywhere.
if ! "$GRIMES" run --dir="$(mktemp -d)" --provider-command=/bin/false x 2>&1 |
    grep -q 'no built-in confinement'; then
    HAVE_BUILTIN=1
else
    HAVE_BUILTIN=0
fi

workspace() {
    local dir
    # Under BINDIR, so the EXIT trap reaches it when an assertion exits early.
    dir="$(mktemp -d "$BINDIR/ws.XXXXXX")"
    mkdir -p "$dir/src"
    # shellcheck disable=SC2016  # the text is the target's content, not an expansion
    printf 'rm -rf ./build/*\neval "$UNTRUSTED"\n' >"$dir/src/app.sh"
    echo "$dir"
}

run_grimes() {
    local dir="$1"
    shift
    "$GRIMES" run --dir="$dir" \
        --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
        "$@" --format=prototext src 2>&1 || true
}

if [[ "$HAVE_BUILTIN" == "0" ]]; then
    echo ""
    echo "--- Without a mechanism the run is refused rather than downgraded ---"
    WS="$(workspace)"
    OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-p2.sh")"
    assert_match "$OUT" 'sandbox-command' \
        "a platform with no built-in mechanism is told how to supply one"
    assert_no_match "$OUT" 'legacy_color' \
        "a run with no mechanism produces no result"
    rm -rf "$WS"
    echo ""
    echo "Passed: $PASSED"
    echo "Failed: $FAILED"
    [[ "$FAILED" -eq 0 ]] || exit 1
    exit 0
fi

echo ""
echo "--- fg-64y.40: a role cannot rewrite the target it is reviewing ---"

# The same fake the collector suite runs under --unsafe, where the engine has to
# notice the edit after the fact. Here nothing has to notice it: the write does
# not land, so the provider fails on its own attempt.
WS="$(workspace)"
BEFORE="$(cat "$WS/src/app.sh")"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-edits-target.sh")"
assert_match "$OUT" 'provider failed' \
    "a role that tries to rewrite the target fails on the attempt"
if [[ "$(cat "$WS/src/app.sh")" == "$BEFORE" ]]; then
    pass "the target is byte-identical after the attempt"
else
    fail "the target is byte-identical after the attempt"
fi
rm -rf "$WS"

# The staged copy is what the next role reads. Rewriting it is how a provider
# hands a different artifact to the context meant to check its work.
WS="$(workspace)"
OUT="$(cd "$WS" && printf 'Clause 4: retention is 30 days.\n' |
    "$GRIMES" run --dir=. --kind=idea \
        --provider-command="$FAKES/provider-rewrites-content.sh" \
        --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
        --format=prototext - 2>&1 || true)"
assert_match "$OUT" 'provider failed' \
    "a role that tries to rewrite the staged copy fails on the attempt"
rm -rf "$WS"

echo ""
echo "--- fg-64y.49: a role cannot read what it was not handed ---"

# The fake fails naming whatever it could read, because nothing a provider
# writes to stdout reaches the run record. A first run lays the artifacts down:
# on the run that writes them they do not exist yet when its own roles spawn.
WS="$(workspace)"
run_grimes "$WS" --provider-command="$FAKES/provider-p2.sh" >/dev/null
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-reads-ledger.sh")"
assert_no_match "$OUT" 'role read:' \
    "a role finds none of the run's own artifacts readable"
assert_match "$OUT" 'legacy_color: +LEGACY_COLOR_' \
    "and reaches a verdict, rather than never having looked"
rm -rf "$WS"

# The same fake with the boundary waived. Without this the assertion above holds
# for a fake that never looked, and a check that cannot fail distinguishes
# nothing.
WS="$(workspace)"
run_grimes "$WS" --unsafe --provider-command="$FAKES/provider-p2.sh" >/dev/null
OUT="$(run_grimes "$WS" --unsafe --provider-command="$FAKES/provider-reads-ledger.sh")"
assert_match "$OUT" 'role read:.*ledger\.pb' \
    "the same role reads the ledger once the boundary is waived"
assert_match "$OUT" 'role read:.*result\.pb' \
    "and reads the verdict the run before it recorded"
rm -rf "$WS"

echo ""
echo "--- A confined role still does the job it was spawned for ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" --provider-command="$FAKES/provider-p2.sh" \
    --refuter-command="$FAKES/refuter-upheld.sh" --refuter-arg=upheld --refuter-fresh)"
assert_match "$OUT" 'legacy_color: +LEGACY_COLOR_' \
    "a confined run reaches a verdict"
assert_match "$OUT" '^  review_confidence: +REVIEW_CONFIDENCE_HIGH' \
    "a confined run with every role attested reaches high confidence"
assert_no_match "$OUT" 'unmet_gates: +"confinement"' \
    "a confined run reports nothing unmet about its boundary"
assert_match "$OUT" 'confinement: +"[^"]+"' \
    "the record names the mechanism the roles ran under"
rm -rf "$WS"

echo ""
echo "--- Waiving the boundary is recorded, not silent ---"

# The same roles as the confined run above, which is what makes the missing
# HIGH below attributable to the waiver and to nothing else.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --unsafe --provider-command="$FAKES/provider-p2.sh" \
    --refuter-command="$FAKES/refuter-upheld.sh" --refuter-arg=upheld --refuter-fresh)"
assert_match "$OUT" 'unmet_gates: +"confinement"' \
    "an unconfined run names the boundary as what is missing"
assert_match "$OUT" 'confinement: +"unsafe"' \
    "the record says the run was unconfined"
# Anchored to the run's own verdict: the adjudicator's nested verdict carries a
# confidence of its own, which this cap has nothing to do with.
assert_no_match "$OUT" '^  review_confidence: +REVIEW_CONFIDENCE_HIGH' \
    "an unconfined run cannot reach high confidence"
rm -rf "$WS"

echo ""
echo "--- An operator's wrapper is held to the same proof ---"

# The engine cannot read a wrapper's policy, so it does not try. It runs a
# negative control instead: a wrapper that blocks nothing is refused before the
# first role, because a policy that was ignored and one that was applied would
# otherwise produce the same successful run.
WS="$(workspace)"
OUT="$(run_grimes "$WS" --sandbox-command="$FAKES/wrapper-transparent.sh" \
    --provider-command="$FAKES/provider-p2.sh")"
assert_match "$OUT" 'confinement blocked nothing' \
    "a wrapper that confines nothing is refused"
assert_no_match "$OUT" 'legacy_color' \
    "a run under an unproven wrapper produces no result"
rm -rf "$WS"

WS="$(workspace)"
OUT="$(run_grimes "$WS" --sandbox-command="grimes-no-such-wrapper" \
    --provider-command="$FAKES/provider-p2.sh")"
assert_match "$OUT" 'could not run' \
    "a wrapper that cannot start is refused as a startup failure"
assert_no_match "$OUT" 'confinement blocked nothing' \
    "a wrapper that never ran is not reported as one that blocked nothing"
rm -rf "$WS"

# A command naming a directory is resolved against the directory the role is
# given, so the engine's own is the wrong one to check it against.
WS="$(workspace)"
printf '#!/usr/bin/env bash\nexec "%s/provider-p2.sh"\n' "$FAKES" >"$WS/provider.sh"
chmod +x "$WS/provider.sh"
OUT="$(cd "$BINDIR" && "$GRIMES" run --dir="$WS" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
    --provider-command="./provider.sh" --format=prototext src 2>&1 || true)"
assert_match "$OUT" 'legacy_color: +LEGACY_COLOR_' \
    "a provider named relative to the review directory is found there"
rm -rf "$WS"

echo ""
echo "--- The flags mean what they say ---"

WS="$(workspace)"
OUT="$(run_grimes "$WS" --unsafe --sandbox-command="$FAKES/wrapper-transparent.sh" \
    --provider-command="$FAKES/provider-p2.sh")"
assert_match "$OUT" 'alternatives' \
    "asking for both a wrapper and no confinement is refused"
rm -rf "$WS"

WS="$(workspace)"
OUT="$(run_grimes "$WS" --sandbox-arg=x --provider-command="$FAKES/provider-p2.sh")"
assert_match "$OUT" 'needs --sandbox-command' \
    "a sandbox argument without a sandbox command is refused"
rm -rf "$WS"

echo ""
echo "Passed: $PASSED"
echo "Failed: $FAILED"
[[ "$FAILED" -eq 0 ]] || exit 1
