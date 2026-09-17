#!/bin/bash
#
# Fix-gate contract tests (audit FG-102).
#
# These are static contract checks over the normative skill text and the Claude
# adapter. They assert the documented gate, not a live grind: a live run needs
# an agent, but the gate has to be unambiguous in the instructions before any
# agent can follow it.
#
# Usage: ./tests/test-fix-gate.sh
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SKILL="$PROJECT_ROOT/skills/frank-grimes/SKILL.md"
GRIND="$PROJECT_ROOT/adapters/claude-code/commands/grind.md"
PROTO="$PROJECT_ROOT/proto/frank_grimes/v2/contracts.proto"

RED='\033[0;31m'
GREEN='\033[0;32m'
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

# assert_present <file> <pattern> <description>
assert_present() {
    if grep -qiE "$2" "$1" 2>/dev/null; then
        pass "$3"
    else
        fail "$3"
    fi
}

# assert_absent <file> <pattern> <description>
assert_absent() {
    if grep -qiE "$2" "$1" 2>/dev/null; then
        fail "$3"
    else
        pass "$3"
    fi
}

echo "========================================"
echo "Fix Gate Contract Tests"
echo "========================================"
echo ""

echo "--- Report is the default ---"
assert_present "$SKILL" 'Reporting is the default' \
    "skill states reporting is the default"
assert_present "$SKILL" 'make no edits and no commits' \
    "skill forbids edits and commits in report mode"
assert_present "$GRIND" 'mode=report.*default|default to .mode=report' \
    "adapter defaults to report mode"
assert_absent "$GRIND" 'Fix \(Recommended\)' \
    "adapter no longer recommends fix mode"

echo ""
echo "--- No commit without a passing gate ---"
# The original defect: commit after each fix, with nothing in between.
assert_absent "$GRIND" 'Commit after each fix' \
    "adapter no longer commits after each fix"
assert_present "$SKILL" 'Never commit per fix' \
    "skill forbids per-fix commits"
# The adapter must defer to the skill rather than restate the rule.
assert_present "$GRIND" 'Fix Mode and the Commit Gate' \
    "adapter references the skill's commit gate section"
assert_present "$SKILL" 'exited zero|exits zero' \
    "skill requires a zero exit before committing"
assert_present "$SKILL" 'nonzero.*make no commit|nonzero.*no commit' \
    "skill forbids a commit after a failing gate"
assert_present "$SKILL" 'unavailable.*make no commit|unavailable.*no commit' \
    "skill forbids a commit when the gate is unavailable"

echo ""
echo "--- Commit is a separate privilege from fix ---"
assert_present "$SKILL" 'separately from fix authorization' \
    "skill separates commit authorization from fix authorization"
assert_present "$GRIND" 'name: commit' \
    "adapter exposes an explicit commit argument"
assert_present "$GRIND" 'name: verify-command' \
    "adapter exposes an explicit verify-command argument"

echo ""
echo "--- Gate selection is ordered and recorded ---"
for rule in 'supplied explicitly|supplied' 'aggregate check' 'documented' 'unavailable'; do
    assert_present "$SKILL" "$rule" "skill documents gate selection rule: $rule"
done
# The rule that selected the gate has no field in the contract yet, so nothing
# downstream can record it. The skill assertion above covers the rule itself.
assert_present "$PROTO" 'message Verification' \
    "the contract defines a verification record"

echo ""
echo "--- Verified means verified ---"
assert_present "$SKILL" 'verified. only when the gate passed' \
    "skill defines verified as gate-passed, not merely edited"
assert_present "$PROTO" 'uint32 verified' \
    "the contract counts verified closures separately"

echo ""
echo "--- Gate result is recorded as evidence ---"
assert_present "$SKILL" 'exit code, and bounded output as E1' \
    "skill records the gate run as E1 evidence"
assert_present "$PROTO" 'VERIFICATION_STATUS_(PASSED|FAILED|UNAVAILABLE|NOT_APPLICABLE)' \
    "the contract names every verification status"

echo ""
echo "--- A ledger crosses a fix only with a record of the change ---"
assert_present "$SKILL" 'recorded the previous fingerprint as the new target.s parent' \
    "skill carries a ledger across a fix only against a recorded parent"
assert_present "$SKILL" 'unexplained change' \
    "skill names a fingerprint change with no record as unexplained"
assert_present "$SKILL" 'anchor that vanished is evidence that something moved' \
    "skill refuses a vanished anchor as proof of a repair"

echo ""
echo "--- A fix batch is bounded by the scope the review resolved ---"
assert_present "$SKILL" 'invalidates the batch rather than shrinking it' \
    "skill refuses an out-of-scope batch whole"
assert_present "$SKILL" 'not re-reported as new' \
    "skill carries findings across a fix without re-reporting them"
assert_present "$SKILL" 'has regressed, which is a different fact' \
    "skill separates a regression from a defect nobody fixed"

echo ""
echo "--- An unverifiable batch is kept, not discarded ---"
assert_present "$SKILL" 'kept rather than discarded, and the record says where' \
    "skill keeps edits an unavailable gate could not test"
assert_present "$SKILL" 'Prose is not a command' \
    "skill resolves the documented-command rule through a reader, not the engine"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
