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
# A refusal an operator cannot act on is one they will work around.
assert_match "$OUT" 'Commit or stash the work' \
    "and the refusal says what to do about it"
rm -rf "$REPO"

# Untracked tooling state is the ordinary case: the provider itself writes into
# the repository, so a second run is refused by the leavings of the first.
REPO="$(repository)"
mkdir -p "$REPO/.toolstate"
printf 'left by the provider\n' >"$REPO/.toolstate/session.json"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_match "$OUT" 'uncommitted changes' \
    "an untracked directory leaves the tree dirty"
printf '.toolstate/\n' >>"$REPO/.git/info/exclude"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_no_match "$OUT" 'uncommitted changes' \
    "and excluding it in the repository lets the run proceed"
rm -rf "$REPO"

# An exclusion passed through the environment is not honoured, deliberately:
# the same mechanism decides which edits git admits to. An operator who reached
# for it has to be told, or the exclusion looks ignored for no reason.
REPO="$(repository)"
mkdir -p "$REPO/.toolstate"
printf 'left by the provider\n' >"$REPO/.toolstate/session.json"
EXCLUDES="$(mktemp)"
printf '.toolstate/\n' >"$EXCLUDES"
OUT="$(GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.excludesFile GIT_CONFIG_VALUE_0="$EXCLUDES" \
    run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_match "$OUT" 'uncommitted changes' \
    "an exclude file named in the environment does not clean the tree"
assert_match "$OUT" 'exclude file named in GIT_CONFIG' \
    "and the refusal says the exclude file it named was not used"
rm -f "$EXCLUDES"
rm -rf "$REPO"

# Git configuration in the environment is ordinary -- a credential helper is
# configured through the same variables. The note answers an operator who named
# an exclude file, so it must not greet one who did not.
REPO="$(repository)"
mkdir -p "$REPO/.toolstate"
printf 'left by the provider\n' >"$REPO/.toolstate/session.json"
OUT="$(GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=credential.helper GIT_CONFIG_VALUE_0=cache \
    run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true")"
assert_match "$OUT" 'uncommitted changes' \
    "a dirty tree is still refused with unrelated git configuration set"
assert_no_match "$OUT" 'exclude file named in GIT_CONFIG' \
    "and nothing is said about an exclude file nobody named"
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
echo "--- The copy is a copy, and what it holds resolves inside it ---"

# A link is copied as a link so the copy keeps the shape the review saw, but a
# link is read at the far end. One pointing out of the tree would resolve to
# whatever is there now, and a role reading the copy would be handed live bytes
# under a name saying they were reviewed.
REPO="$(repository)"
OUTSIDE="$(mktemp -d)"
printf 'not part of any review\n' >"$OUTSIDE/elsewhere.txt"
mkdir -p "$REPO/lib"
printf 'shared\n' >"$REPO/lib/shared.sh"
ln -s "$OUTSIDE/elsewhere.txt" "$REPO/src/escape.txt"
ln -s "../lib/shared.sh" "$REPO/src/inside.sh"
git -C "$REPO" add -A
git -C "$REPO" commit --quiet --message "add links"
"$GRIMES" run --dir="$REPO" --mode=fix \
    --adjudicator-command="$FAKES/adjudicator-reports-where.sh" --adjudicator-fresh \
    --adjudicator-arg=src/escape.txt --adjudicator-arg=src/inside.sh \
    --provider-command="$FAKES/fixer-repairs.sh" --verify-command="true" \
    --format=prototext src >/dev/null 2>&1 || true
WHERE="$(cat "$REPO/.grimes/work/where" 2>/dev/null || true)"
if [[ -n "$WHERE" ]]; then
    pass "the role reported where it was pointed"
else
    fail "the role reported nothing (it must, or what follows proves nothing)"
fi
assert_match "$WHERE" 'absent src/escape.txt' \
    "a link out of the tree is not carried into the copy"
assert_match "$WHERE" 'read src/inside.sh=shared' \
    "and a link within it still resolves, inside the copy"
assert_no_match "$WHERE" 'not part of any review' \
    "so nothing outside the review is readable through the copy"
rm -rf "$REPO" "$OUTSIDE"

# A directory whose name begins with two dots is a name, not an escape. Read as
# one, a later role is handed the whole copy instead of the path under review.
REPO="$(repository)"
mkdir -p "$REPO/..generated"
printf 'echo generated\n' >"$REPO/..generated/app.sh"
git -C "$REPO" add -A
git -C "$REPO" commit --quiet --message "add a dotted directory"
"$GRIMES" run --dir="$REPO" --mode=fix \
    --adjudicator-command="$FAKES/adjudicator-reports-where.sh" --adjudicator-fresh \
    --provider-command="$FAKES/provider-green.sh" \
    --format=prototext '..generated' >/dev/null 2>&1 || true
WHERE="$(cat "$REPO/.grimes/work/where" 2>/dev/null || true)"
assert_match "$WHERE" 'content=.*/\.\.generated' \
    "a scope whose name begins with dots is the scope the later role is given"
rm -rf "$REPO"

# The copy leaves out the repository's own directory, so a target inside it is
# one the later roles would be pointed at and find nothing at.
REPO="$(repository)"
OUT="$("$GRIMES" run --dir="$REPO" --mode=fix --kind=code \
    --provider-command="$FAKES/fixer-repairs.sh" --format=prototext .git 2>&1 || true)"
assert_match "$OUT" 'not reviewable in fix mode' \
    "a target the copy cannot carry is refused rather than half-staged"
rm -rf "$REPO"

echo ""
echo "--- A commit takes the whole worktree, so it takes none of a mixed batch ---"

# One repair earns its credit and another does not, in a single set of edits.
# A commit cannot honour both: it carries the tree entire, so the broken
# repair would ride along in a commit crediting the surviving one. Committing
# a subset is not the answer either -- the gate passed over the tree as a
# whole, and a partial commit is a state nothing verified.
REPO="$(repository)"
printf 'echo other\n' >"$REPO/src/other.sh"
git -C "$REPO" add -A
git -C "$REPO" commit --quiet --message "add a second file"
OUT="$("$GRIMES" run --dir="$REPO" --mode=fix \
    --provider-command="$FAKES/fixer-two-files.sh" \
    --refuter-command="$FAKES/refuter-splits.sh" --refuter-arg=other.sh --refuter-fresh \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
    --verify-command="true" --commit --format=prototext src 2>&1 || true)"
assert_match "$OUT" 'refuter_check: +REFUTER_CHECK_PASSED' \
    "the refutation pass was one the engine could grade"
assert_match "$OUT" 'provenance: +FINDING_PROVENANCE_UPHELD' \
    "one claim survived the attack"
assert_match "$OUT" 'provenance: +FINDING_PROVENANCE_REFUTED' \
    "and one did not"
assert_match "$OUT" 'status: +FINDING_STATUS_VERIFIED' \
    "the surviving claim is still credited with its repair"
assert_no_match "$OUT" 'commit_sha1' \
    "but nothing is committed while a broken repair shares the tree"
rm -rf "$REPO"

# The same batch with both claims upheld: withholding is about the broken one,
# not about there having been two.
REPO="$(repository)"
printf 'echo other\n' >"$REPO/src/other.sh"
git -C "$REPO" add -A
git -C "$REPO" commit --quiet --message "add a second file"
OUT="$("$GRIMES" run --dir="$REPO" --mode=fix \
    --provider-command="$FAKES/fixer-two-files.sh" \
    --refuter-command="$FAKES/refuter-splits.sh" --refuter-arg=nothing-matches --refuter-fresh \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
    --verify-command="true" --commit --format=prototext src 2>&1 || true)"
assert_no_match "$OUT" 'provenance: +FINDING_PROVENANCE_REFUTED' \
    "a batch whose claims all survived carries no broken repair"
assert_match "$OUT" 'commit_sha1' \
    "and it is committed"
rm -rf "$REPO"

echo ""
echo "--- A gate may walk the repository it is checking ---"

# The worktree sits inside the review's own directory, so a tool that looks
# upward for its configuration -- buf, golangci-lint and go all do -- stats that
# directory on the way past. Denied outright it returns EPERM rather than
# ENOENT, and those tools treat it as fatal. A boundary that breaks the checks
# the review depends on has stopped being a boundary and started being a fault.
#
# Metadata only. A tool that recurses into the review's own directory still
# cannot, and that stays refused: the grant that would let it list what is in
# there turns out to hand over the contents as well.
REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" \
    --verify-command="stat .. >/dev/null")"
assert_match "$OUT" 'status: +VERIFICATION_STATUS_PASSED' \
    "a gate that walks the repository is not defeated by the review's own directory"
rm -rf "$REPO"

# What the walk must still not reach. The ledger is the record of the review
# the gate is part of; a gate that could read it could be written to agree.
REPO="$(repository)"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh" \
    --verify-command="cat ../ledger.pb >/dev/null 2>&1 && exit 1; exit 0")"
assert_match "$OUT" 'status: +VERIFICATION_STATUS_PASSED' \
    "and still cannot read the review's own record"
rm -rf "$REPO"

echo ""
echo "--- The gate runs against a relative --dir ---"

# A boundary is enforced against the path the kernel resolves, so a review
# directory named relatively has to be resolved before it becomes one. Every
# other case here passes an absolute --dir, which is how this went unseen.
REPO="$(repository)"
OUT="$(cd "$REPO" && "$GRIMES" run --dir=. --mode=fix \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh \
    --provider-command="$FAKES/fixer-repairs.sh" \
    --verify-command="true" --format=prototext src 2>&1 || true)"
assert_match "$OUT" 'status: +VERIFICATION_STATUS_PASSED' \
    "a relative --dir reaches the gate"
rm -rf "$REPO"

echo ""
echo "--- A worktree add that fails leaves nothing for the next run ---"

# git registers the worktree before running the repository's post-checkout
# hook, so a hook that fails leaves the registration without the directory.
# Left there, every later run is refused a path it never made.
REPO="$(repository)"
printf '#!/bin/sh\nexit 1\n' >"$REPO/.git/hooks/post-checkout"
chmod +x "$REPO/.git/hooks/post-checkout"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh")"
if grep -q 'creating the worktree' <<<"$OUT"; then
    pass "a hook that fails, fails the run"
else
    fail "the failing hook did not fail the run"
fi

# The run that matters is the next one, after the operator clears what the
# failure left. The directory goes; the registration is what outlives it, and a
# run that found the directory would reopen it instead of adding one.
rm -f "$REPO/.git/hooks/post-checkout"
rm -rf "$REPO/.grimes"
OUT="$(run_fix "$REPO" --provider-command="$FAKES/fixer-repairs.sh")"
if grep -q 'already registered' <<<"$OUT"; then
    fail "the failed add blocked the next run"
else
    pass "the next run is not blocked by the failed add"
fi
assert_match "$OUT" 'worktree: +"' "and it builds a worktree of its own"
rm -rf "$REPO"

echo ""
echo "Passed: $PASSED"
echo "Failed: $FAILED"
[[ "$FAILED" -eq 0 ]] || exit 1
exit 0
