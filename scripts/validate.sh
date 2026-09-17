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

# Verify stop.sh reads the engine's run record
if grep -q '.grimes/state.pb' "$PROJECT_ROOT/hooks/stop.sh"; then
    pass "stop.sh reads the engine's run record"
else
    fail "stop.sh does not reference .grimes/state.pb"
fi

# The hook decides nothing. A verdict string compared in shell is the defect
# that let a review certify its own pass.
if grep -qE 'last_verdict|==[[:space:]]*"?(GREEN|RED|YELLOW)"?' "$PROJECT_ROOT/hooks/stop.sh"; then
    fail "stop.sh compares a verdict string; the engine owns the verdict"
else
    pass "stop.sh does not compare a verdict string"
fi

# An installed hook runs from a versioned cache directory, so resolving the
# repository from the script's own location finds the plugin, not the project.
if grep -qE 'CLAUDE_PROJECT_DIR|GRIMES_PROJECT_DIR' "$PROJECT_ROOT/hooks/stop.sh"; then
    pass "stop.sh resolves the repository from the environment"
else
    fail "stop.sh resolves the repository from its own path only"
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

    echo "toolchain: $(go version 2>/dev/null || echo 'go not found') | $(buf --version 2>/dev/null) | protoc $(protoc --version 2>/dev/null | cut -d' ' -f2) | $(protoc-gen-go --version 2>&1 || echo 'protoc-gen-go not found')"

    # Existence is not the guarantee. buf resolves the plugin off PATH, so an
    # unpinned copy earlier in PATH produces different bindings while still
    # satisfying a presence check.
    if command -v mise &>/dev/null && command -v protoc-gen-go &>/dev/null; then
        PINNED="$(mise which protoc-gen-go 2>/dev/null || true)"
        RESOLVED="$(command -v protoc-gen-go)"
        if [[ -z "$PINNED" ]]; then
            warn "mise does not manage protoc-gen-go; codegen would use $RESOLVED"
        elif [[ "$PINNED" == "$RESOLVED" ]]; then
            pass "protoc-gen-go resolves to the pinned binary"
        else
            fail "protoc-gen-go resolves to $RESOLVED, not the pinned $PINNED"
        fi
    fi

    # GOPATH/bin is deliberately not added to PATH here: that is where an
    # unpinned ad-hoc `go install protoc-gen-go` lands, and letting it resolve
    # is the drift this check exists to catch.
    if ! command -v protoc-gen-go &>/dev/null; then
        fail "protoc-gen-go not found; run 'mise install' to get the pinned version"
    elif ! (cd "$PROJECT_ROOT" && git rev-parse --is-inside-work-tree) &>/dev/null; then
        warn "not a git work tree - skipping codegen drift check"
    elif ! (cd "$PROJECT_ROOT" && buf generate) &>/dev/null; then
        fail "buf generate failed"
    # --porcelain rather than `git diff` so a newly generated untracked file
    # counts as drift too.
    elif [[ -n "$(cd "$PROJECT_ROOT" && git status --porcelain -- gen)" ]]; then
        fail "generated bindings drifted from the contract; run 'just gen' and commit"
    else
        pass "generated bindings match the contract"
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

# The tier caps constrain a severity paired with its evidence, so they are
# declared on both Finding and CandidateFinding. Two copies of a rule drift, and
# the copy nobody updates is the one that lets a bad finding through.
for rule in p0_requires_e1_or_e2 e3_caps_at_p1; do
    EXPRS=$(awk -v r="$rule" '
        $0 ~ "id: \"(finding|candidate)\\." r "\"" { grab = 1; next }
        grab && /expression:/ { sub(/^ *expression: */, ""); print; grab = 0 }
    ' "$PROJECT_ROOT/proto/frank_grimes/v2/contracts.proto" | sort -u | wc -l | tr -d ' ')
    COUNT=$(grep -c "$rule" "$PROJECT_ROOT/proto/frank_grimes/v2/contracts.proto")
    if [[ "$COUNT" -lt 2 ]]; then
        fail "$rule is declared $COUNT time(s); Finding and CandidateFinding both need it"
    elif [[ "$EXPRS" == "1" ]]; then
        pass "$rule is identical on Finding and CandidateFinding"
    else
        fail "$rule has drifted between Finding and CandidateFinding"
    fi
done

echo ""
echo "--- Orchestrator ---"

check test -d "$PROJECT_ROOT/cmd/grimes" "cmd/grimes/ exists"
check test -f "$PROJECT_ROOT/internal/engine/verdict.go" "verdict derivation has a single home"
check test -f "$PROJECT_ROOT/internal/adjudicate/resolve.go" "adjudication resolution has a single home"

# A verdict decided in two places drifts, and the second copy is the one nobody
# updates. Returning a colour is the derivation; reading one to format output is
# not, so this looks for the assignment rather than the mention.
DUPLICATES=""
while IFS= read -r match; do
    rel="${match%%:*}"
    rel="${rel#"$PROJECT_ROOT"/}"
    case "$rel" in
        internal/engine/weight.go | *_test.go) continue ;;
    esac
    DUPLICATES="$DUPLICATES $rel"
done < <(grep -rnE 'return pb\.LegacyColor_' \
    "$PROJECT_ROOT/internal" "$PROJECT_ROOT/cmd" --include='*.go' 2>/dev/null || true)

if [[ -n "$DUPLICATES" ]]; then
    fail "colour derived outside internal/engine/weight.go:$DUPLICATES"
else
    pass "the legacy colour is derived in one place"
fi

# The run record's completion state and the stop hook's decision are the same
# claim about the same iteration. A second place that names a completion state
# lets the record say a review finished while the loop keeps going. The pattern
# matches any named constant rather than a set of assignment syntaxes, since a
# plain assignment or a local variable would otherwise carry one past it.
DUPLICATES=""
while IFS= read -r match; do
    rel="${match%%:*}"
    rel="${rel#"$PROJECT_ROOT"/}"
    case "$rel" in
        internal/engine/loop.go | *_test.go) continue ;;
    esac
    DUPLICATES="$DUPLICATES $rel"
done < <(grep -rn 'pb\.CompletionState_' \
    "$PROJECT_ROOT/internal" "$PROJECT_ROOT/cmd" --include='*.go' 2>/dev/null || true)

if [[ -n "$DUPLICATES" ]]; then
    fail "completion state named outside internal/engine/loop.go:$DUPLICATES"
else
    pass "the completion state is derived in one place"
fi

# An adapter carries wiring, never methodology: a colour or decision decided
# there is a second implementation of Phase 7.
if grep -rqE 'LEGACY_COLOR_|DECISION_PASS' "$PROJECT_ROOT/adapters" "$PROJECT_ROOT/hooks" 2>/dev/null; then
    fail "adapters or hooks name contract verdict values; the engine owns them"
else
    pass "no adapter or hook decides a verdict"
fi

# A flag the engine refuses must not be documented as one a caller can use.
# --scope was advertised in four places and rejected in all of them, which a
# live run found the hard way.
echo "--- Documented Flags ---"

REFUSED_FLAGS=("--scope")
for flag in "${REFUSED_FLAGS[@]}"; do
    FOUND=$(grep -rlF -- "$flag " \
        "$PROJECT_ROOT/README.md" \
        "$PROJECT_ROOT/adapters" \
        "$PROJECT_ROOT/docs" 2>/dev/null |
        grep -v '/audit/' |
        xargs -r grep -lF -- "grind $flag" 2>/dev/null || true)
    ALSO=$(grep -rn -- "$flag recent-changes\|$flag whole-repo" \
        "$PROJECT_ROOT/README.md" "$PROJECT_ROOT/adapters" "$PROJECT_ROOT/docs" 2>/dev/null |
        grep -v '/audit/' || true)
    if [[ -n "$FOUND$ALSO" ]]; then
        fail "the engine refuses $flag, and it is documented as usable"
        echo "$FOUND$ALSO"
    else
        pass "no document offers $flag, which the engine refuses"
    fi
done

echo ""

# SKILL.md is the sole normative methodology. A second copy in an adapter or a
# README does not stay in sync; it goes stale and then contradicts the skill.
# These markers are normative definitions, not mentions, so they may appear in
# exactly one file.
echo "--- Methodology Ownership ---"

# shellcheck disable=SC2016  # backticks are literal markdown, not substitution
NORMATIVE_MARKERS=(
    'Phase 4: Default Assumption'
    'E1 (reproduced)'
    'E3 (inferred)'
    'Route exactly'
    'Derive `RED` from'
    'A check that cannot fail distinguishes nothing'
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

# The site publishes the phase names. Numbering that disagrees with the skill
# teaches a reader a structure the tool does not have.
SITE="$PROJECT_ROOT/docs/index.html"
SKILL_PHASES=$(grep -oE '^### Phase [^:]+: [^(]+' "$PROJECT_ROOT/skills/frank-grimes/SKILL.md" |
    sed 's/^### //; s/[[:space:]]*$//' | sort)
SITE_PHASES=$(
    grep -oE 'phase-label">Phase [^<]+</span>[[:space:]]*$' "$SITE" >/dev/null 2>&1 || true
    python3 - "$SITE" <<'PYEOF' 2>/dev/null || true
import re, sys
html = open(sys.argv[1]).read()
for label, title in re.findall(r'phase-label">([^<]+)</span>\s*<h3 class="phase-title">([^<]+)</h3>', html):
    if label.startswith("Phase"):
        print(f"{label}: {title}".replace("&amp;", "&"))
PYEOF
)
SITE_PHASES=$(echo "$SITE_PHASES" | grep . | sort)

if [[ "$SKILL_PHASES" == "$SITE_PHASES" ]]; then
    pass "docs site phase names match the skill"
else
    fail "docs site phase names drifted from the skill"
    diff <(echo "$SKILL_PHASES") <(echo "$SITE_PHASES") | sed 's/^/    /'
fi

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

# The aggregate gate is worth nothing if nothing runs it. A workflow that fires
# only on a release tag lets every intermediate commit through unchecked, which
# is how toolchain and codegen drift reached a release before.
GATE_WORKFLOW=""
for wf in "$PROJECT_ROOT"/.github/workflows/*.yml; do
    if grep -q 'pull_request' "$wf" && grep -q 'just check' "$wf"; then
        GATE_WORKFLOW="$(basename "$wf")"
        break
    fi
done
if [[ -n "$GATE_WORKFLOW" ]]; then
    pass "the aggregate gate runs on pull requests ($GATE_WORKFLOW)"
else
    fail "no workflow runs 'just check' on pull_request"
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
