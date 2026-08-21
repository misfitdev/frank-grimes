#!/bin/bash
#
# Frank Grimes Validation Script
#
# Validates the standalone repository structure, configuration files, and scripts.
# Run from the project root: ./scripts/validate.sh
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PASSED=0
FAILED=0

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

pass() {
    echo -e "${GREEN}PASS${NC}: $*"
    PASSED=$((PASSED + 1))
}

fail() {
    echo -e "${RED}FAIL${NC}: $*"
    FAILED=$((FAILED + 1))
}

warn() {
    echo -e "${YELLOW}WARN${NC}: $*"
}

check() {
    local test_cmd="$1"
    local test_args=()
    local description=""

    # Parse arguments: everything until the last string argument is the test command+args
    # The last argument is the description
    local args=("$@")
    local num_args=${#args[@]}

    if [[ $num_args -lt 2 ]]; then
        echo "ERROR: check() requires at least 2 arguments" >&2
        return 1
    fi

    # Last argument is the description
    description="${args[$((num_args - 1))]}"

    # Everything before the last argument is the test command and its args
    test_cmd="${args[0]}"
    test_args=("${args[@]:1:$((num_args - 2))}")

    if "$test_cmd" "${test_args[@]}"; then
        pass "$description"
    else
        fail "$description"
    fi
}

echo "========================================"
echo "Frank Grimes Validation"
echo "========================================"
echo ""

# ============================================
# 1. Directory Structure
# ============================================
echo "--- Directory Structure ---"

check test -d "$PROJECT_ROOT/skill" "skill/ directory exists"
check test -f "$PROJECT_ROOT/skill/SKILL.md" "skill/SKILL.md exists"
check test -d "$PROJECT_ROOT/hooks" "hooks/ directory exists"
check test -f "$PROJECT_ROOT/hooks/stop.sh" "hooks/stop.sh exists"
check test -d "$PROJECT_ROOT/adapters" "adapters/ directory exists"
check test -f "$PROJECT_ROOT/adapters/README.md" "adapters/README.md exists"
check test -d "$PROJECT_ROOT/adapters/claude-code" "adapters/claude-code/ exists"
check test -d "$PROJECT_ROOT/adapters/opencode" "adapters/opencode/ exists"
check test -d "$PROJECT_ROOT/adapters/codify" "adapters/codify/ exists"
check test -d "$PROJECT_ROOT/scripts" "scripts/ directory exists"
check test -f "$PROJECT_ROOT/scripts/validate.sh" "scripts/validate.sh exists"
check test -d "$PROJECT_ROOT/benchmark" "benchmark/ directory exists"

echo ""

# ============================================
# 2. JSON Validation
# ============================================
echo "--- JSON Validation ---"

if command -v jq &> /dev/null; then
    check jq empty "$PROJECT_ROOT/adapters/claude-code/plugin.json" "plugin.json is valid JSON"
    check jq empty "$PROJECT_ROOT/adapters/claude-code/hooks.json" "hooks.json is valid JSON"
else
    warn "jq not found - skipping JSON validation"
fi

echo ""

# ============================================
# 3. Bash Syntax Validation
# ============================================
echo "--- Bash Syntax Validation ---"

check bash -n "$PROJECT_ROOT/hooks/stop.sh" "stop.sh has valid bash syntax"
check bash -n "$PROJECT_ROOT/scripts/validate.sh" "validate.sh has valid bash syntax"

echo ""

# ============================================
# 4. SKILL.md Frontmatter Validation
# ============================================
echo "--- SKILL.md Frontmatter Validation ---"

SKILL_MD="$PROJECT_ROOT/skill/SKILL.md"

if [[ -f "$SKILL_MD" ]]; then
    # Extract frontmatter between first --- markers
    FRONTMATTER=$(sed -n '1,/^---$/p' "$SKILL_MD" | sed '1d;$d')

    # Check for name field
    if echo "$FRONTMATTER" | grep -qE '^name:'; then
        NAME_VALUE=$(echo "$FRONTMATTER" | grep -E '^name:' | sed 's/name:[[:space:]]*//' | tr -d '"' | tr -d "'")
        if [[ -n "$NAME_VALUE" ]]; then
            if [[ "$NAME_VALUE" == "frank-grimes" ]]; then
                pass "SKILL.md frontmatter name is 'frank-grimes'"
            else
                fail "SKILL.md frontmatter name is '$NAME_VALUE', expected 'frank-grimes'"
            fi
        else
            fail "SKILL.md frontmatter name is empty"
        fi
    else
        fail "SKILL.md frontmatter missing 'name' field"
    fi

    # Check for description field
    if echo "$FRONTMATTER" | grep -qE '^description:'; then
        # Handle multi-line YAML folded scalar (description: >)
        # Extract description value, handling both single-line and multi-line formats
        DESC_VALUE=$(echo "$FRONTMATTER" | awk '
            /^description: >$/ {
                # Multi-line folded scalar - get the rest of the frontmatter
                in_desc = 1
                next
            }
            /^description: / {
                # Single-line description
                sub(/^description: */, "")
                print
                exit
            }
            in_desc && /^  / {
                # Indented continuation line (2+ spaces) - strip leading whitespace
                sub(/^  */, "")
                print
                next
            }
            in_desc && !/^  / {
                # End of description
                exit
            }
        ')
        if [[ ${#DESC_VALUE} -gt 20 ]]; then
            pass "SKILL.md frontmatter description is present and substantial (${DESC_VALUE:0:80}...)"
        else
            fail "SKILL.md frontmatter description is too short"
        fi
    else
        fail "SKILL.md frontmatter missing 'description' field"
    fi
else
    fail "SKILL.md not found"
fi

echo ""

# ============================================
# 5. Provider-Neutral Language Check
# ============================================
echo "--- Provider-Neutral Language Check ---"

# Check README.md for platform-specific language that should be removed
if [[ -f "$PROJECT_ROOT/README.md" ]]; then
    # Check for Claude-specific installation commands
    if grep -qi "/plugin marketplace" "$PROJECT_ROOT/README.md"; then
        fail "README.md contains '/plugin marketplace' (Claude-specific install command)"
    else
        pass "README.md does not contain '/plugin marketplace'"
    fi

    if grep -qi "/plugin install" "$PROJECT_ROOT/README.md"; then
        fail "README.md contains '/plugin install' (Claude-specific install command)"
    else
        pass "README.md does not contain '/plugin install'"
    fi

    if grep -qi "claude-plugins" "$PROJECT_ROOT/README.md"; then
        fail "README.md contains 'claude-plugins' (Claude-specific reference)"
    else
        pass "README.md does not contain 'claude-plugins'"
    fi

    # "Claude Code" references are allowed in provider-specific adapter sections
    pass "README.md provider-neutral check complete"
else
    fail "README.md not found"
fi

echo ""

# ============================================
# 6. Adapter Completeness
# ============================================
echo "--- Adapter Completeness ---"

# Claude Code adapter
check test -f "$PROJECT_ROOT/adapters/claude-code/plugin.json" "Claude Code: plugin.json exists"
check test -f "$PROJECT_ROOT/adapters/claude-code/hooks.json" "Claude Code: hooks.json exists"
check test -f "$PROJECT_ROOT/adapters/claude-code/commands/grind.md" "Claude Code: commands/grind.md exists"
check test -f "$PROJECT_ROOT/adapters/claude-code/commands/help.md" "Claude Code: commands/help.md exists"
check test -f "$PROJECT_ROOT/adapters/claude-code/commands/cancel.md" "Claude Code: commands/cancel.md exists"

# OpenCode adapter
check test -f "$PROJECT_ROOT/adapters/opencode/AGENTS.md" "OpenCode: AGENTS.md exists"

# Codex adapter
check test -f "$PROJECT_ROOT/adapters/codify/AGENTS.md" "Codex: AGENTS.md exists"

echo ""

# ============================================
# 7. Stop Hook Integration Check
# ============================================
echo "--- Stop Hook Integration Check ---"

# Verify stop.sh references project-local state file, not hardcoded cache path
if grep -q '~/.cache/claude-plugins' "$PROJECT_ROOT/hooks/stop.sh"; then
    fail "stop.sh contains hardcoded cache path (~/.cache/claude-plugins)"
else
    pass "stop.sh does not contain hardcoded cache path"
fi

# Verify stop.sh uses .grimes-state.json
if grep -q '.grimes-state.json' "$PROJECT_ROOT/hooks/stop.sh"; then
    pass "stop.sh references .grimes-state.json (project-local)"
else
    fail "stop.sh does not reference .grimes-state.json"
fi

echo ""

# ============================================
# 8. Gitignore Check
# ============================================
echo "--- Gitignore Check ---"

if [[ -f "$PROJECT_ROOT/.gitignore" ]]; then
    check grep -q '.grimes-state.json' "$PROJECT_ROOT/.gitignore" ".gitignore excludes .grimes-state.json"
    check grep -q 'GRIMES_REPORT.md' "$PROJECT_ROOT/.gitignore" ".gitignore excludes GRIMES_REPORT.md"
    check grep -q 'API_QUALITY_REPORT.md' "$PROJECT_ROOT/.gitignore" ".gitignore excludes API_QUALITY_REPORT.md"
    check grep -q '\.grimes/' "$PROJECT_ROOT/.gitignore" ".gitignore excludes .grimes/ directory"
else
    fail ".gitignore not found"
fi

echo ""

# ============================================
# Summary
# ============================================
echo "========================================"
echo "Validation Summary"
echo "========================================"
echo -e "Passed: ${GREEN}$PASSED${NC}"
echo -e "Failed: ${RED}$FAILED${NC}"
echo ""

if [[ $FAILED -gt 0 ]]; then
    echo -e "${RED}Validation FAILED${NC}"
    exit 1
else
    echo -e "${GREEN}Validation PASSED${NC}"
    exit 0
fi
