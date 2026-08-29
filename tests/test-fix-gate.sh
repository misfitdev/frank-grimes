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
assert_present "$GRIND" 'do not commit per fix|Never commit per fix' \
    "adapter forbids per-fix commits"
assert_present "$SKILL" 'Never commit per fix' \
    "skill forbids per-fix commits"
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
assert_present "$GRIND" 'selected_by' \
    "adapter records which rule selected the gate"

echo ""
echo "--- Verified means verified ---"
assert_present "$SKILL" 'verified. only when the gate passed' \
    "skill defines verified as gate-passed, not merely edited"
assert_present "$GRIND" 'VERIFIED closures only' \
    "adapter counts only verified closures as fixed"

echo ""
echo "--- Gate result is recorded as evidence ---"
assert_present "$SKILL" 'exit code, and bounded output as E1' \
    "skill records the gate run as E1 evidence"
assert_present "$GRIND" '"status": "passed\|failed\|unavailable\|not_applicable"' \
    "adapter reports a verification status in the structured result"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
