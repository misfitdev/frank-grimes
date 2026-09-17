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
# Captured before the run: every branch here already holds the first commit, so
# "the branch has commits" would pass whether or not anything was committed.
MAIN_BEFORE="$(git -C "$REPO" rev-parse main)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" --commit)"
assert_match "$OUT" 'commit_sha1' \
    "an authorized batch over a passing gate is committed"
assert_match "$OUT" 'closed_finding_ids: +"FG-' \
    "and the record names what the commit closed"
BRANCH="$(sed -nE 's/^ *branch: +"(.*)"$/\1/p' <<<"$OUT" | head -1)"
TIP="$(git -C "$REPO" rev-parse "$BRANCH" 2>/dev/null || true)"
if [[ -n "$TIP" && "$TIP" != "$MAIN_BEFORE" ]]; then
    pass "the branch the record names carries a commit the run made"
else
    fail "the branch the record names carries a commit the run made ($BRANCH at ${TIP:-nothing})"
fi
if [[ "$(git -C "$REPO" rev-parse main)" == "$MAIN_BEFORE" ]]; then
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

# A later iteration reuses the worktree the first one made, and the branch it
# reports has to be the one that is there: this run's identity is not the one
# that named it.
REPO="$(repository)"
run_fix "$REPO" --auto-loop --max-iterations=3 \
    --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" --commit >/dev/null
OUT="$(run_fix "$REPO" --auto-loop --max-iterations=3 \
    --provider-command="$FAKES/provider-green.sh" --verify-command="true" --commit)"
BRANCH="$(sed -nE 's/^ *branch: +"(.*)"$/\1/p' <<<"$OUT" | head -1)"
if [[ -n "$BRANCH" ]] && git -C "$REPO" rev-parse --verify --quiet "$BRANCH" >/dev/null; then
    pass "a continued run names the branch its worktree is actually on"
else
    fail "a continued run names the branch its worktree is actually on ($BRANCH)"
fi
rm -rf "$REPO"

echo ""
echo "--- The repository's own check is a command from inside the target ---"

REPO="$(repository)"
printf 'check:\n\ttrue\n' >"$REPO/Makefile"
git -C "$REPO" add -A
git -C "$REPO" commit --quiet --message "add a check"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh")"
assert_match "$OUT" 'status: +VERIFICATION_STATUS_UNAVAILABLE' \
    "a checked-in check is not run on its own say-so"
rm -rf "$REPO"

REPO="$(repository)"
printf 'check:\n\ttrue\n' >"$REPO/Makefile"
git -C "$REPO" add -A
git -C "$REPO" commit --quiet --message "add a check"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --repository-check)"
assert_match "$OUT" 'selected_by: +GATE_SELECTION_REPOSITORY_CHECK' \
    "and runs once the operator says they have read it"
# Selected is not run. The recipe is `check: true`, so anything but a pass means
# the gate was chosen and then could not be executed.
assert_match "$OUT" 'status: +VERIFICATION_STATUS_PASSED' \
    "and the recipe it selected actually ran"
rm -rf "$REPO"

echo ""
echo "--- The gate runs inside the boundary, not beside it ---"

# A gate is a command out of the repository as much as a role is. Its working
# directory is not what stops it reaching the tree the worktree exists to keep
# out of reach.
REPO="$(repository)"
MAIN_BEFORE="$(git -C "$REPO" rev-parse main)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" \
    --verify-command="echo gate-was-here >../../src/app.sh; echo gate-was-here >../ledger.pb; echo gate-was-here >../../.git/refs/heads/main; true")"
# Asserted first: a gate that never ran leaves every path below untouched, and
# each check would pass without the boundary having done anything.
assert_match "$OUT" 'status: +VERIFICATION_STATUS_PASSED' \
    "the gate ran, so what it could not reach is the boundary's doing"
if [[ "$(cat "$REPO/src/app.sh")" != *gate-was-here* ]]; then
    pass "a gate cannot write the tree the operator is holding"
else
    fail "a gate cannot write the tree the operator is holding"
fi
if [[ ! -f "$REPO/.grimes/ledger.pb" ]] || ! grep -q gate-was-here "$REPO/.grimes/ledger.pb"; then
    pass "a gate cannot write the review's own record"
else
    fail "a gate cannot write the review's own record"
fi
if [[ "$(git -C "$REPO" rev-parse main)" == "$MAIN_BEFORE" ]]; then
    pass "a gate cannot move the repository's history"
else
    fail "a gate cannot move the repository's history"
fi
rm -rf "$REPO"

# The boundary still has to leave a real check able to run. git writes inside
# the worktree's own administrative directory to do no more than read status.
REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" \
    --verify-command="git status --porcelain >/dev/null && go version >/dev/null")"
assert_match "$OUT" 'status: +VERIFICATION_STATUS_PASSED' \
    "a gate that reads the repository and its toolchain still passes"
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
echo "--- A range target bounds a batch by what the range changed ---"

# A repository whose last commit touches one file. The range is the scope, so a
# batch is bounded by the files it changed rather than by a path prefix.
ranged_repository() {
    local dir
    dir="$(repository)"
    # shellcheck disable=SC2016 # the text is the target's content
    printf 'rm -rf ./build/*\n# second\n' >"$dir/src/app.sh"
    git -C "$dir" add -A
    git -C "$dir" commit --quiet --message "second"
    echo "$dir"
}

run_range_fix() {
    local dir="$1"
    shift
    "$GRIMES" run --dir="$dir" --mode=fix \
        --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
        "$@" --format=prototext 'HEAD^..HEAD' 2>&1 || true
}

REPO="$(ranged_repository)"
BEFORE="$(cat "$REPO/src/app.sh")"
OUT="$(run_range_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_match "$OUT" 'fix_batch' "a fix run over a range records what it did"
assert_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "a fix inside the range is verified"
if [[ "$(cat "$REPO/src/app.sh")" == "$BEFORE" ]]; then
    pass "the operator's own working tree is left byte-identical"
else
    fail "the operator's own working tree was written"
fi
rm -rf "$REPO"

# The file the batch strays into is inside the repository, and a path target
# over the whole tree would have it in scope. What puts it outside here is that
# the range never touched it.
REPO="$(ranged_repository)"
OUT="$(run_range_fix "$REPO" --provider-command="$FAKES/fixer-strays.sh" --verify-command="true")"
assert_match "$OUT" 'outside the reviewed scope' \
    "a batch editing a file the range never changed is refused"
assert_match "$OUT" 'README.md' \
    "and the refusal names what it edited"
assert_no_match "$OUT" 'legacy_color' \
    "a refused batch produces no result"
rm -rf "$REPO"

# A range selecting more than one file, and a batch that commits an edit to one
# of them. The spelling means something different once that commit lands, and a
# second iteration that re-read it would review only what the fixer touched and
# measure coverage against that.
REPO="$(repository)"
printf 'echo second\n' >"$REPO/src/other.sh"
git -C "$REPO" add -A
git -C "$REPO" commit --quiet --message "add another file"
# shellcheck disable=SC2016 # the text is the target's content
printf 'rm -rf ./build/*\n# edited\n' >"$REPO/src/app.sh"
printf 'echo second, edited\n' >"$REPO/src/other.sh"
git -C "$REPO" add -A
git -C "$REPO" commit --quiet --message "touch both"
run_range_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" \
    --verify-command="true" --commit >/dev/null
UNITS="$(grimes-contract decode-report --type=TargetInventory \
    "$REPO/.grimes/inventory.pb" 2>/dev/null | grep -cE '^ *id:' || true)"
if [[ "$UNITS" == "2" ]]; then
    pass "the first iteration reviews both files the range changed"
else
    fail "the first iteration reviews both files the range changed (got $UNITS)"
fi
SECOND="$(run_range_fix "$REPO" --provider-command="$FAKES/provider-green.sh" \
    --verify-command="true" --commit)"
# Asserted before the count: a refused second run leaves the first run's
# inventory on disk, and counting that would pass without anything having been
# collected again.
assert_no_match "$SECOND" 'error:' "a committed iteration is collected again rather than refused"
AGAIN="$(grimes-contract decode-report --type=TargetInventory \
    "$REPO/.grimes/inventory.pb" 2>/dev/null | grep -cE '^ *id:' || true)"
if [[ "$AGAIN" == "2" ]]; then
    pass "and a committed iteration still reviews both, not just what it fixed"
else
    fail "and a committed iteration still reviews both, not just what it fixed (got $AGAIN)"
fi
rm -rf "$REPO"

echo ""
echo "--- A role after the fixing one reads what was reviewed ---"

# The whole point of the mode is that the primary changes the target, so by the
# time anything else runs the bytes on disk are the repair. A reviewer shown
# those is judging work nobody asked it about: it would find no defect where
# one was reported, and the disagreement would be an artefact of the order the
# roles ran in.
REPO="$(repository)"
REVIEWED="$(cat "$REPO/src/app.sh")"
OUT="$("$GRIMES" run --dir="$REPO" --mode=fix \
    --adjudicator-command="$FAKES/adjudicator-quotes-target.sh" --adjudicator-fresh \
    --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" \
    --format=prototext src 2>&1 || true)"
SAW="$(cat "$REPO/.grimes/work/adjudicator-saw" 2>/dev/null || true)"
if [[ -n "$SAW" ]]; then
    pass "the adjudicator read the target it was judging"
else
    fail "the adjudicator read nothing (it must, or what follows proves nothing)"
fi
if [[ "$SAW" == "$REVIEWED" ]]; then
    pass "and what it read is the bytes the review was about"
else
    fail "the adjudicator was shown the batch, not what was reviewed"
fi
# The fixer still edited: the copy is beside the worktree, not instead of it.
TREE="$(worktree_of "$OUT")"
if [[ -n "$TREE" && "$(cat "$TREE/src/app.sh")" != "$REVIEWED" ]]; then
    pass "while the fixing role still changed the worktree"
else
    fail "while the fixing role still changed the worktree"
fi
assert_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "and the batch is still verified"
rm -rf "$REPO"

# A run's reviewed bytes are its own. Left behind, the next iteration would
# hand its roles a copy of a target that has since moved.
REPO="$(repository)"
run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" >/dev/null
if [[ -d "$REPO/.grimes/reviewed" ]]; then
    fail "the reviewed copy outlived the run that took it"
else
    pass "the reviewed copy does not outlive the run"
fi
rm -rf "$REPO"

echo ""
echo "--- A repair is credited only to a claim that survived an attack ---"

# The fixing role reports a defect and repairs it in one pass, so nothing has
# tested whether there was a defect to repair. The gate is evidence about the
# repair: an edit that compiles is not evidence there was something to compile
# away. A claim a second context broke has not earned one.
REPO="$(repository)"
REVIEWED="$(cat "$REPO/src/app.sh")"
OUT="$("$GRIMES" run --dir="$REPO" --mode=fix \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
    --refuter-command="$FAKES/refuter-refuted.sh" --refuter-fresh \
    --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" --commit \
    --format=prototext src 2>&1 || true)"
# Asserted first: a pass that never graded the control raises nothing above
# unattacked, and every check below would hold without an attack having run.
assert_match "$OUT" 'refuter_check: +REFUTER_CHECK_PASSED' \
    "the refutation pass was one the engine could grade"
assert_match "$OUT" 'provenance: +FINDING_PROVENANCE_REFUTED' \
    "a claim a second context broke is recorded as broken"
assert_no_match "$OUT" 'status: +FINDING_STATUS_FIXED' \
    "and the edit made for it does not make it fixed"
assert_no_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "nor verified, whatever the gate said"
assert_no_match "$OUT" 'commit_sha1' \
    "and nothing is committed over it"
# The edit is still there to read; it is credited to nothing.
TREE="$(worktree_of "$OUT")"
if [[ -n "$TREE" && "$(cat "$TREE/src/app.sh")" != "$REVIEWED" ]]; then
    pass "the edit stands in the worktree for the operator to read"
else
    fail "the edit stands in the worktree for the operator to read"
fi
rm -rf "$REPO"

# The same run with the claim upheld: the ordering must not cost a real repair
# its credit.
REPO="$(repository)"
OUT="$("$GRIMES" run --dir="$REPO" --mode=fix \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
    --refuter-command="$FAKES/refuter-upheld.sh" --refuter-fresh \
    --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" --commit \
    --format=prototext src 2>&1 || true)"
assert_match "$OUT" 'provenance: +FINDING_PROVENANCE_UPHELD' \
    "a claim that survived the attack is recorded as having survived"
assert_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "and the repair made for it is verified"
assert_match "$OUT" 'commit_sha1' \
    "and committed"
rm -rf "$REPO"

echo ""
echo "Passed: $PASSED"
echo "Failed: $FAILED"
[[ "$FAILED" -eq 0 ]] || exit 1
exit 0
