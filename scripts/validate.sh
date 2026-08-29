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
check test -f "$PROJECT_ROOT/adapters/claude-code/agents/grimey-verifier.md" "Claude Code: independent verifier subagent exists"
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

# The .proto is the sole normative machine contract. Generated bindings that
# have drifted from it are worse than absent: they compile, so the drift is
# invisible until a message validates against a schema nobody wrote.
echo "--- Machine Contract ---"

check test -f "$PROJECT_ROOT/proto/frank_grimes/v2/contracts.proto" "contract proto exists"
check test -f "$PROJECT_ROOT/buf.yaml" "buf module config exists"

if command -v buf &>/dev/null; then
    if (cd "$PROJECT_ROOT" && buf lint) &>/dev/null; then
        pass "buf lint passes"
    else
        fail "buf lint fails"
    fi

    if command -v go &>/dev/null; then
        GEN_BEFORE=$(find "$PROJECT_ROOT/gen" -name '*.go' -exec shasum {} \; 2>/dev/null | shasum | cut -d' ' -f1)
        if (cd "$PROJECT_ROOT" && PATH="$PATH:$(go env GOPATH)/bin" buf generate) &>/dev/null; then
            GEN_AFTER=$(find "$PROJECT_ROOT/gen" -name '*.go' -exec shasum {} \; 2>/dev/null | shasum | cut -d' ' -f1)
            if [[ "$GEN_BEFORE" == "$GEN_AFTER" ]]; then
                pass "generated bindings match the contract"
            else
                fail "generated bindings drifted from the contract; run 'just gen' and commit"
            fi
        else
            warn "buf generate failed - skipping codegen drift check"
        fi
    else
        warn "go not found - skipping codegen drift check"
    fi
else
    warn "buf not found - skipping contract lint"
fi

# Only the codec may write the ledger; a JSON file is a projection for reading.
if grep -rq 'ledger.json' "$PROJECT_ROOT/hooks" "$PROJECT_ROOT/adapters" 2>/dev/null; then
    fail "ledger.json referenced as input; the authoritative ledger is .grimes/ledger.pb"
else
    pass "no component treats the JSON projection as control input"
fi

echo ""

# SKILL.md is the sole normative methodology. A second copy in an adapter or a
# README does not stay in sync; it goes stale and then contradicts the skill.
# These markers are normative definitions, not mentions, so they may appear in
# exactly one file.
echo "--- Methodology Ownership ---"

# shellcheck disable=SC2016  # backticks are literal markdown, not substitution
NORMATIVE_MARKERS=(
    'Phase 2: Default Assumption'
    'E1 — reproduced'
    'E3 — inferred'
    'Route exactly'
    'Derive `RED` from'
)

for marker in "${NORMATIVE_MARKERS[@]}"; do
    OWNERS=$(grep -rlF "$marker" \
        "$PROJECT_ROOT/skills" \
        "$PROJECT_ROOT/adapters" \
        "$PROJECT_ROOT/hooks" \
        "$PROJECT_ROOT/README.md" \
        "$PROJECT_ROOT/docs" 2>/dev/null | grep -v '/audit/' || true)
    OWNER_COUNT=$(echo "$OWNERS" | grep -c . || true)

    if [[ "$OWNER_COUNT" -eq 1 ]] && [[ "$OWNERS" == *"skills/frank-grimes/SKILL.md" ]]; then
        pass "methodology marker defined only in the skill: $marker"
    elif [[ "$OWNER_COUNT" -eq 0 ]]; then
        fail "methodology marker missing from the skill: $marker"
    else
        fail "methodology marker defined outside the skill: $marker"
        echo "$OWNERS"
    fi
done

# Each adapter must point at the skill rather than paraphrase it.
for adapter in "adapters/claude-code/commands/grind.md" "adapters/opencode/AGENTS.md" "adapters/codify/AGENTS.md"; do
    if grep -qF "SKILL.md" "$PROJECT_ROOT/$adapter" 2>/dev/null; then
        pass "$adapter references SKILL.md"
    else
        fail "$adapter does not reference SKILL.md"
    fi
done

echo ""

# Ground truth that claims a defect the target does not have makes recall and
# precision unmeasurable: a correct grind gets penalized for declining to report
# a phantom. Each expected set must name its known false positives.
for target in shell go web; do
    EXPECTED="$PROJECT_ROOT/benchmark/targets/$target/expected-issues.md"
    if grep -q '^## Explicitly Not Defects' "$EXPECTED" 2>/dev/null; then
        pass "$target expectations record their known false positives"
    else
        fail "$target expectations have no 'Explicitly Not Defects' section"
    fi
done

# The old sets rewarded volume, which is the padding the rubric now penalizes.
if grep -rqE 'identify at least [0-9]+' "$PROJECT_ROOT/benchmark/targets" 2>/dev/null; then
    fail "expected sets still set a finding-count target; precision is the standard"
else
    pass "expected sets judge precision rather than finding count"
fi

echo ""

# The scorer greps for the report template's section names and field labels, so
# editing the template silently breaks scoring. The fixtures are the tripwire:
# a conforming report must score high and a theatrical one must not.
if command -v jq &>/dev/null; then
    CONFORMING=$("$PROJECT_ROOT/benchmark/runner.sh" --score "$PROJECT_ROOT/benchmark/fixtures/conforming-report.md" 2>/dev/null | grep -oE '\([0-9]+%\)' | tr -d '()%')
    NONCONFORMING=$("$PROJECT_ROOT/benchmark/runner.sh" --score "$PROJECT_ROOT/benchmark/fixtures/nonconforming-report.md" 2>/dev/null | grep -oE '\([0-9]+%\)' | tr -d '()%')

    if [[ "${CONFORMING:-0}" -ge 90 ]]; then
        pass "conforming fixture scores ${CONFORMING}% (>= 90)"
    else
        fail "conforming fixture scores ${CONFORMING:-0}%; scorer has drifted from the report template"
    fi

    if [[ "${NONCONFORMING:-100}" -le 50 ]]; then
        pass "non-conforming fixture scores ${NONCONFORMING}% (<= 50)"
    else
        fail "non-conforming fixture scores ${NONCONFORMING:-100}%; scorer no longer discriminates"
    fi

    # A continuation fragment must be classified as a failed delivery rather
    # than scored, or a truncated run silently enters an A/B comparison.
    if "$PROJECT_ROOT/benchmark/runner.sh" --score "$PROJECT_ROOT/benchmark/fixtures/truncated-report.md" &>/dev/null; then
        fail "truncated fixture was scored; malformed reports must be rejected"
    else
        pass "truncated fixture is rejected as a malformed report"
    fi
else
    warn "jq not found - skipping benchmark scorer regression check"
fi

echo ""

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
