#!/usr/bin/env bash
# fg-64y.6: fix mode applies edits in a worktree and verifies them with a gate.
#
# Black-box: everything goes through the grimes CLI against a real git
# repository, with fake fixers standing in for a coding agent.
#
# The property under test: a run may say a defect is gone only when a gate it
# named passed after the edit, the operator's own working tree is never
# written, and a batch that strayed outside the reviewed scope is refused whole.
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

for tool in go git; do
    if ! command -v "$tool" &>/dev/null; then
        echo -e "${YELLOW}SKIP${NC}: $tool is not installed; fix-mode tests need it"
        exit 2
    fi
done

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

# Where there is no built-in mechanism a run is refused by design, which the
# confinement suite asserts. Fix mode has nothing to add to that.
if "$GRIMES" run --dir="$(mktemp -d)" --provider-command=/bin/false x 2>&1 |
    grep -q 'supply --sandbox-command'; then
    echo -e "${YELLOW}SKIP${NC}: no built-in confinement on this platform"
    exit 2
fi

# repository lays out a target with one defect in it, committed, on a clean
# tree. Under BINDIR so the EXIT trap reaches it.
repository() {
    local dir
    dir="$(mktemp -d "$BINDIR/repo.XXXXXX")"
    mkdir -p "$dir/src"
    # shellcheck disable=SC2016  # the text is the target's content
    printf 'rm -rf ./build/*\n' >"$dir/src/app.sh"
    printf '# a project\n' >"$dir/README.md"
    git -C "$dir" init --quiet --initial-branch=main
    git -C "$dir" config user.email "test@example.invalid"
    git -C "$dir" config user.name "Test"
    git -C "$dir" add -A
    git -C "$dir" commit --quiet --message "first"
    echo "$dir"
}

run_fix() {
    local dir="$1"
    shift
    "$GRIMES" run --dir="$dir" --mode=fix \
        --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
        "$@" --format=prototext src 2>&1 || true
}

worktree_of() {
    sed -nE 's/^ *worktree: +"(.*)"$/\1/p' <<<"$1" | head -1
}

echo ""
echo "--- A fix lands in a worktree, not in the tree the operator is holding ---"

REPO="$(repository)"
BEFORE="$(cat "$REPO/src/app.sh")"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_match "$OUT" 'fix_batch' "a fix run records what it did with its edits"
if [[ "$(cat "$REPO/src/app.sh")" == "$BEFORE" ]]; then
    pass "the operator's own working tree is byte-identical"
else
    fail "the operator's own working tree is byte-identical"
fi
TREE="$(worktree_of "$OUT")"
if [[ -n "$TREE" && "$(cat "$TREE/src/app.sh")" != "$BEFORE" ]]; then
    pass "the edit is in the worktree the record names"
else
    fail "the edit is in the worktree the record names (worktree=$TREE)"
fi
rm -rf "$REPO"

echo ""
echo "--- Verified means a gate said so ---"

REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "a fix the gate passed over is verified"
assert_match "$OUT" 'status: +VERIFICATION_STATUS_PASSED' \
    "and the record carries the gate that passed"
rm -rf "$REPO"

REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="exit 1")"
assert_match "$OUT" 'status: +FINDING_STATUS_FIXED' \
    "a fix the gate refused is fixed and no more"
assert_no_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "a failing gate verifies nothing"
assert_match "$OUT" 'status: +VERIFICATION_STATUS_FAILED' \
    "and the record says the gate failed"
# The skill's own words: an unverified fix is an unevidenced claim.
assert_no_match "$OUT" '^  residual_risk: +RESIDUAL_RISK_LOW' \
    "an unverified fix still carries risk"
TREE="$(worktree_of "$OUT")"
if [[ -n "$TREE" && -d "$TREE" ]]; then
    pass "the worktree survives a failed gate for the operator to read"
else
    fail "the worktree survives a failed gate for the operator to read"
fi
rm -rf "$REPO"

REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh")"
assert_match "$OUT" 'status: +VERIFICATION_STATUS_UNAVAILABLE' \
    "a repository with no check leaves the gate unavailable"
assert_no_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "an unavailable gate verifies nothing"
rm -rf "$REPO"

echo ""
echo "--- A batch is bounded by the scope the review resolved ---"

REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-strays.sh" --verify-command="true")"
assert_match "$OUT" 'outside the reviewed scope' \
    "a batch that edited outside the scope is refused"
assert_match "$OUT" 'README.md' \
    "and the refusal names what it edited"
assert_no_match "$OUT" 'legacy_color' \
    "a refused batch produces no result"
if [[ -z "$(git -C "$REPO" log --oneline --all --grep='Closes FG-' 2>/dev/null)" ]]; then
    pass "a refused batch is not committed"
else
    fail "a refused batch is not committed"
fi
rm -rf "$REPO"

echo ""
echo "--- A commit is separately authorized and separately earned ---"

REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_no_match "$OUT" 'commit_sha1' \
    "a verified batch is not committed without authorization"
rm -rf "$REPO"

REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" --commit)"
assert_match "$OUT" 'commit_sha1' \
    "an authorized batch over a passing gate is committed"
assert_match "$OUT" 'closed_finding_ids: +"FG-' \
    "and the record names what the commit closed"
BRANCH="$(sed -nE 's/^ *branch: +"(.*)"$/\1/p' <<<"$OUT" | head -1)"
# Captured rather than piped into grep: pipefail plus a reader that exits early
# makes git's SIGPIPE the pipeline's exit status, which reads as no commit.
ONBRANCH="$(git -C "$REPO" log --oneline "$BRANCH" 2>/dev/null || true)"
if [[ -n "$ONBRANCH" ]]; then
    pass "the commit is on the branch the record names"
else
    fail "the commit is on the branch the record names ($BRANCH)"
fi
if [[ "$(git -C "$REPO" rev-parse main)" == "$(git -C "$REPO" rev-parse HEAD)" ]]; then
    pass "the branch the operator is on did not move"
else
    fail "the branch the operator is on did not move"
fi
rm -rf "$REPO"

REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="exit 1" --commit)"
assert_no_match "$OUT" 'commit_sha1' \
    "a failing gate is not committed over"
rm -rf "$REPO"

echo ""
echo "--- The tree has to be the one the operator is looking at ---"

REPO="$(repository)"
printf 'uncommitted\n' >>"$REPO/src/app.sh"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_match "$OUT" 'uncommitted changes' \
    "a fix run against a dirty tree is refused"
assert_no_match "$OUT" 'legacy_color' \
    "and produces no result"
rm -rf "$REPO"

echo ""
echo "--- The ledger follows the target across the change it made ---"

REPO="$(repository)"
run_fix "$REPO" --auto-loop --max-iterations=2 \
    --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" >/dev/null
OUT="$(run_fix "$REPO" --auto-loop --max-iterations=2 \
    --provider-command="$FAKES/provider-green.sh" --verify-command="true")"
assert_match "$OUT" 'iteration: +2' \
    "a second iteration runs against the bytes the first one produced"
assert_no_match "$OUT" 'holds findings for' \
    "the ledger is carried rather than refused"
assert_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "and the finding the first iteration closed stays closed"
rm -rf "$REPO"

# The lineage is a record, not a waiver: a target that moved without one is
# still refused.
REPO="$(repository)"
run_fix "$REPO" --auto-loop --max-iterations=2 \
    --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" >/dev/null
printf 'someone else was here\n' >>"$REPO/.grimes/fix/src/app.sh"
OUT="$(run_fix "$REPO" --auto-loop --max-iterations=2 \
    --provider-command="$FAKES/provider-green.sh" --verify-command="true")"
assert_match "$OUT" 'holds findings for|state holds' \
    "a target that moved outside the run is still refused"
rm -rf "$REPO"

echo ""
echo "--- Fix mode applies to targets that have a repository ---"

REPO="$(repository)"
OUT="$("$GRIMES" run --dir="$REPO" --mode=fix --kind=document \
    --provider-command="$FAKES/fixer-repairs.sh" --format=prototext README.md 2>&1 || true)"
assert_match "$OUT" 'kind code' \
    "a document target cannot be fixed"
rm -rf "$REPO"

echo ""
echo "Passed: $PASSED"
echo "Failed: $FAILED"
[[ "$FAILED" -eq 0 ]] || exit 1
exit 0
