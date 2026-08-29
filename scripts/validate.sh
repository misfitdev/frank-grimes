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

check test -d "$PROJECT_ROOT/skills/frank-grimes" "skills/frank-grimes/ directory exists"
check test -f "$PROJECT_ROOT/skills/frank-grimes/SKILL.md" "skills/frank-grimes/SKILL.md exists"
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

if command -v jq &>/dev/null; then
    check jq empty "$PROJECT_ROOT/.claude-plugin/plugin.json" "Claude plugin.json is valid JSON"
    check jq empty "$PROJECT_ROOT/.claude-plugin/marketplace.json" "Claude marketplace.json is valid JSON"
    check jq empty "$PROJECT_ROOT/.codex-plugin/plugin.json" "Codex plugin.json is valid JSON"
    check jq empty "$PROJECT_ROOT/adapters/claude-code/hooks.json" "hooks.json is valid JSON"

    # Both manifests namespace their components off name, and Claude pins the
    # cached copy to version, so a stale version string blocks user updates.
    for manifest in ".claude-plugin/plugin.json" ".codex-plugin/plugin.json"; do
        if [[ "$(jq -r '.name' "$PROJECT_ROOT/$manifest")" == "frank-grimes" ]]; then
            pass "$manifest declares name 'frank-grimes'"
        else
            fail "$manifest name is not 'frank-grimes'"
        fi
    done

    CLAUDE_VERSION=$(jq -r '.version' "$PROJECT_ROOT/.claude-plugin/plugin.json")
    CODEX_VERSION=$(jq -r '.version' "$PROJECT_ROOT/.codex-plugin/plugin.json")
    if [[ "$CLAUDE_VERSION" == "$CODEX_VERSION" ]]; then
        pass "plugin manifests agree on version $CLAUDE_VERSION"
    else
        fail "plugin manifest versions differ: Claude $CLAUDE_VERSION, Codex $CODEX_VERSION"
    fi

    # Codex rejects a manifest carrying unsupported fields.
    if jq -e 'has("hooks")' "$PROJECT_ROOT/.codex-plugin/plugin.json" >/dev/null; then
        fail ".codex-plugin/plugin.json contains 'hooks' (rejected by Codex validation)"
    else
        pass ".codex-plugin/plugin.json omits the unsupported 'hooks' field"
    fi
else
    warn "jq not found - skipping JSON validation"
fi

# Schema checks the field-level assertions above cannot make. Not run with
# --strict: that flags the repo's own CLAUDE.md, which is contributor
# instructions rather than plugin-shipped context.
if command -v claude &>/dev/null; then
    check claude plugin validate "$PROJECT_ROOT/.claude-plugin/plugin.json" "Claude plugin manifest passes claude plugin validate"
    check claude plugin validate "$PROJECT_ROOT" "Claude marketplace manifest passes claude plugin validate"
else
    warn "claude not found - skipping plugin manifest schema validation"
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

SKILL_MD="$PROJECT_ROOT/skills/frank-grimes/SKILL.md"

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
# 5. Provider Neutrality and Coverage
# ============================================
echo "--- Provider Neutrality and Coverage ---"

# Every provider loads skills/ verbatim, so naming one there breaks the others.
# Provider-specific setup belongs in adapters/ and README.md, which must cover
# each supported platform rather than omit it.
SUPPORTED_PROVIDERS=("Claude Code" "OpenCode" "Codex")

SKILL_LEAK=$(grep -rniE 'claude|codex|opencode|anthropic|openai' "$PROJECT_ROOT/skills" || true)
if [[ -n "$SKILL_LEAK" ]]; then
    fail "skills/ names a specific provider:"
    echo "$SKILL_LEAK"
else
    pass "skills/ is provider-neutral"
fi

for doc in "README.md" "adapters/README.md"; do
    if [[ -f "$PROJECT_ROOT/$doc" ]]; then
        for provider in "${SUPPORTED_PROVIDERS[@]}"; do
            if grep -qi "$provider" "$PROJECT_ROOT/$doc"; then
                pass "$doc documents $provider"
            else
                fail "$doc does not document $provider"
            fi
        done
    else
        fail "$doc not found"
    fi
done

echo ""

# ============================================
# 6. Adapter Completeness
# ============================================
echo "--- Adapter Completeness ---"

# Plugin manifests
check test -f "$PROJECT_ROOT/.claude-plugin/plugin.json" "Claude Code: .claude-plugin/plugin.json exists"
check test -f "$PROJECT_ROOT/.claude-plugin/marketplace.json" "Claude Code: .claude-plugin/marketplace.json exists"
check test -f "$PROJECT_ROOT/.codex-plugin/plugin.json" "Codex: .codex-plugin/plugin.json exists"

# Claude Code adapter
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
# shellcheck disable=SC2088  # the tilde is a literal to search for, not a path to expand
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

# An installed plugin runs from a versioned cache directory, so a hook path
# relative to the working directory silently stops firing.
if grep -q 'CLAUDE_PLUGIN_ROOT' "$PROJECT_ROOT/adapters/claude-code/hooks.json"; then
    pass "hooks.json resolves stop.sh through \${CLAUDE_PLUGIN_ROOT}"
else
    fail "hooks.json does not resolve stop.sh through \${CLAUDE_PLUGIN_ROOT}"
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
