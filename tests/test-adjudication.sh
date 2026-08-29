#!/bin/bash
#
# Independent adjudication contract tests (audit FG-103).
#
# The P0 these guard: a reviewer confirming its own pass. Two properties have
# to hold. The adjudicator must be told nothing about the primary review, and
# a pass must be unreachable without its confirmation.
#
# Usage: ./tests/test-adjudication.sh
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SKILL="$PROJECT_ROOT/skills/frank-grimes/SKILL.md"
GRIND="$PROJECT_ROOT/adapters/claude-code/commands/grind.md"
AGENT="$PROJECT_ROOT/adapters/claude-code/agents/grimey-verifier.md"

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

assert_present() {
    if grep -qiE "$2" "$1" 2>/dev/null; then
        pass "$3"
    else
        fail "$3"
    fi
}

assert_absent() {
    if grep -qiE "$2" "$1" 2>/dev/null; then
        fail "$3"
    else
        pass "$3"
    fi
}

echo "========================================"
echo "Independent Adjudication Contract Tests"
echo "========================================"
echo ""

echo "--- The adjudicator exists and is read-only ---"
if [[ -f "$AGENT" ]]; then
    pass "verifier subagent is defined"
else
    fail "verifier subagent is defined"
fi
assert_present "$AGENT" '^name: grimey-verifier' \
    "verifier declares a name"
assert_present "$AGENT" '^description:' \
    "verifier declares a description"
assert_present "$AGENT" '^disallowedTools:.*(Write|Edit)' \
    "verifier is denied Write and Edit at the tool layer"
assert_present "$AGENT" 'do not commit, stage, stash, reset' \
    "verifier is forbidden from writing git history"
assert_absent "$AGENT" '^tools:.*\bWrite\b' \
    "verifier does not allowlist Write"

echo ""
echo "--- The prompt carries no knowledge of the primary review ---"
# Extract the fenced prompt template the adapter tells the orchestrator to send.
# shellcheck disable=SC2016  # backticks are literal markdown fence characters
PROMPT_BLOCK=$(awk '/passing exactly this and nothing else/,/^```$/' "$GRIND" | sed -n '/```text/,/```/p')

if [[ -n "$PROMPT_BLOCK" ]]; then
    pass "adapter defines an explicit verifier prompt template"
else
    fail "adapter defines an explicit verifier prompt template"
fi

# The template may mention only target identity and the claimed tuple.
for leak in 'evidence' 'grime[- ][a-z0-9]{3}' 'grime[- ]id' 'finding' 'severity' 'P[0-3]' 'fix' 'register' 'ledger' 'BLUF' 'E[123]\b'; do
    if echo "$PROMPT_BLOCK" | grep -qiE "$leak"; then
        fail "verifier prompt template leaks: $leak"
    else
        pass "verifier prompt template does not leak: $leak"
    fi
done

assert_present "$GRIND" 'Do NOT include findings, evidence, severities' \
    "adapter explicitly forbids leaking findings into the prompt"
assert_present "$AGENT" 'you will not be shown it|have not seen their review' \
    "verifier states it has not seen the primary review"
assert_present "$AGENT" 'treat its presence as a finding' \
    "verifier reports contamination rather than proceeding"

echo ""
echo "--- A pass is unreachable without confirmation ---"
assert_present "$SKILL" 'cannot confirm its own acquittal' \
    "skill forbids self-confirmation"
assert_present "$SKILL" 'stricter decision wins' \
    "skill resolves disagreement toward the stricter decision"
assert_present "$SKILL" 'never upgrades a primary .block. or .conditional' \
    "an independent pass cannot relax a primary block or conditional"
assert_present "$SKILL" 'not available.*review_confidence = low|review_confidence = low' \
    "unavailable adjudication forces low confidence"
assert_present "$SKILL" 'Absence of a second opinion is not agreement' \
    "skill refuses to read silence as confirmation"
assert_present "$SKILL" 'caps the verdict at .conditional|caps at .conditional' \
    "accepted-but-unfixed P0 caps the verdict"

echo ""
echo "--- The old self-certifying escape hatch is gone ---"
assert_absent "$GRIND" 'All P0 risks mitigated or explicitly accepted with timeline' \
    "adapter no longer grants GREEN to an accepted P0"
assert_present "$GRIND" 'Required before any .pass.' \
    "adapter requires adjudication before a pass"
assert_present "$SKILL" 'independent adjudication confirmed it' \
    "skill requires confirmation before a pass"
assert_present "$GRIND" '^  - Agent$' \
    "adapter is permitted to invoke a subagent"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
