#!/bin/bash
#
# Frank Grimes Benchmark Runner
#
# Executes a grind against a target and scores the output against the rubric.
# Designed for A/B testing: run baseline, modify skill, run again, compare.
#
# Usage:
#   ./benchmark/runner.sh <target-path>    # Run against a single target
#   ./benchmark/runner.sh --all            # Run against all targets
#   ./benchmark/runner.sh --all --compare  # Compare with baseline results
#   ./benchmark/runner.sh --score <report> # Score a saved report, no agent run
#
# The runner expects an AI agent to execute the grind. This script:
# 1. Sets up the target in a temporary directory
# 2. Invokes the grind (manual step - see instructions)
# 3. Collects the output report
# 4. Scores the report against the rubric
#
# For automated benchmarking, integrate with your agent's CLI:
#   opencode run "Run a Grimes Grind on <target>" ...
#   claude -p "Run a Grimes Grind on <target>" ...
#

set -euo pipefail

BENCHMARK_DIR="$(cd "$(dirname "$0")" && pwd)"
RESULTS_DIR="$BENCHMARK_DIR/results"
BASELINE_DIR="$RESULTS_DIR/baseline"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

pass() { echo -e "${GREEN}PASS${NC}: $*"; }
fail() { echo -e "${RED}FAIL${NC}: $*"; }
info() { echo -e "${BLUE}INFO${NC}: $*"; }
warn() { echo -e "${YELLOW}WARN${NC}: $*"; }

# Parse arguments
ALL=false
COMPARE=false
TARGET=""
SCORE_ONLY=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --all)
            ALL=true
            shift
            ;;
        --compare)
            COMPARE=true
            shift
            ;;
        --score)
            SCORE_ONLY="$2"
            shift 2
            ;;
        *)
            TARGET="$1"
            shift
            ;;
    esac
done

# Check for jq
if ! command -v jq &>/dev/null; then
    echo "ERROR: jq is required for the benchmark runner"
    exit 1
fi

# Create results directory
mkdir -p "$RESULTS_DIR"

# Timestamp for this run
TIMESTAMP=$(date '+%Y%m%d-%H%M%S')

# ============================================
# Functions
# ============================================

list_targets() {
    echo "Available targets:"
    echo ""
    echo "  Shell:"
    echo "    - benchmark/targets/shell/bad-script.sh"
    echo ""
    echo "  Go:"
    echo "    - benchmark/targets/go/bad-service/"
    echo ""
    echo "  Web (Node.js):"
    echo "    - benchmark/targets/web/bad-endpoint/"
    echo ""
}

# grep -c prints 0 and exits 1 when nothing matches, so `|| echo 0` appends a
# second line and every arithmetic test downstream fails. Always count here.
count_matches() {
    local pattern="$1"
    local file="$2"
    grep -cE "$pattern" "$file" 2>/dev/null || true
}

# Counts every occurrence, not every matching line. Routing reasons are often
# written as one prose paragraph, where a line count reports 1 for all ten.
count_occurrences() {
    local pattern="$1"
    local file="$2"
    grep -oE "$pattern" "$file" 2>/dev/null | wc -l | tr -d ' '
}

# Register rows are table lines carrying a stable ID and a severity cell.
count_register_rows() {
    count_matches '^\|[^|]*grime-[a-z0-9]+[^|]*\|.*\| *P[0-3] *\|' "$1"
}

# A continuation fragment scores like a bad review when it is really a failed
# delivery. Scoring one silently corrupts an A/B comparison, so classify it
# instead. Prints a reason and returns 1 when the report is not deliverable.
validate_report() {
    local report_file="$1"
    local reasons=()

    grep -qE '^#{1,3} Grimes Grind Report:' "$report_file" 2>/dev/null ||
        reasons+=("missing report title")
    grep -qE '^#{1,4} Verdict' "$report_file" 2>/dev/null ||
        reasons+=("missing verdict section")
    grep -q 'BLUF' "$report_file" 2>/dev/null ||
        reasons+=("missing BLUF")
    grep -q "Grimey's Final Word" "$report_file" 2>/dev/null ||
        reasons+=("missing final word")

    # A body that opens mid-table or mid-sentence is a continuation fragment.
    local first_body
    first_body=$(grep -vE '^\s*$' "$report_file" 2>/dev/null | head -1)
    if [[ "$first_body" =~ ^\| ]] || [[ "$first_body" =~ ^[a-z] ]]; then
        reasons+=("body opens mid-table or mid-sentence")
    fi

    if [[ ${#reasons[@]} -gt 0 ]]; then
        printf '%s' "${reasons[0]}"
        printf ', %s' "${reasons[@]:1}"
        printf '\n'
        return 1
    fi
    return 0
}

score_dimension() {
    local dimension="$1"
    local report_file="$2"
    local target_type="$3"

    # Structural scoring only. This checks that the report carries the fields
    # the rubric requires; it cannot judge whether a quote proves its finding
    # or whether a severity is defensible. See "Scope of the Automated Scorer".

    local score=0

    case "$dimension" in
        "evidence_quality")
            local rows e1 e2 e3 tiered
            rows=$(count_register_rows "$report_file")
            e1=$(count_matches '\bE1\b' "$report_file")
            e2=$(count_matches '\bE2\b' "$report_file")
            e3=$(count_matches '\bE3\b' "$report_file")
            tiered=$((e1 + e2 + e3))

            if [[ "$rows" -eq 0 ]] || [[ "$tiered" -eq 0 ]]; then
                score=0
            elif [[ "$tiered" -lt "$rows" ]]; then
                # Fewer tier mentions than findings: some finding cites none.
                score=1
            else
                score=3
                # E2 without a path:line locator is an unverifiable citation.
                if grep -qE '[A-Za-z0-9_./-]+:[0-9]+' "$report_file" 2>/dev/null; then
                    score=$((score + 1))
                fi
                # E1 is only reproduced if a status or exit code was recorded.
                if grep -qiE 'exit code|exit status|status [0-9]' "$report_file" 2>/dev/null; then
                    score=$((score + 1))
                fi
            fi
            ;;

        "routing_discipline")
            local included excluded routed
            included=$(count_occurrences 'included [—-]' "$report_file")
            excluded=$(count_occurrences 'excluded [—-]' "$report_file")
            routed=$((included + excluded))

            if [[ "$included" -eq 0 ]]; then
                score=0
            elif [[ "$excluded" -eq 0 ]]; then
                # Inclusions without exclusions: routing was never defended.
                score=2
            elif [[ "$included" -lt 5 ]] || [[ "$included" -gt 8 ]]; then
                # Outside the 5-8 band. Breadth is a routing failure, not rigor.
                score=2
            elif [[ "$routed" -ge 10 ]]; then
                score=5
            else
                score=3
            fi
            ;;

        "severity_assessment")
            # Check for proper severity distribution
            local p0_count p1_count p2_count p3_count
            p0_count=$(count_matches '\| *P0 *\|' "$report_file")
            p1_count=$(count_matches '\| *P1 *\|' "$report_file")
            p2_count=$(count_matches '\| *P2 *\|' "$report_file")
            p3_count=$(count_matches '\| *P3 *\|' "$report_file")

            if [[ "$p0_count" -gt 0 ]] && [[ "$p1_count" -gt 0 ]] && [[ "$p2_count" -gt 0 ]]; then
                score=4
            elif [[ "$p0_count" -gt 0 ]] || [[ "$p1_count" -gt 0 ]]; then
                score=3
            elif [[ "$p2_count" -gt 0 ]] || [[ "$p3_count" -gt 0 ]]; then
                score=2
            else
                score=1
            fi

            # Check that P0 is not overused
            if [[ "$p0_count" -gt "$p1_count" ]] && [[ "$p0_count" -gt 2 ]]; then
                score=$((score - 1)) # Penalize P0 overuse
            fi
            ;;

        "verdict_accuracy")
            local tuple=0
            grep -qi '^- \*\*Decision:\*\*' "$report_file" 2>/dev/null && ((tuple++))
            grep -qi '^- \*\*Residual risk:\*\*' "$report_file" 2>/dev/null && ((tuple++))
            grep -qi '^- \*\*Review confidence:\*\*' "$report_file" 2>/dev/null && ((tuple++))
            grep -qi '^- \*\*Review completeness:\*\*' "$report_file" 2>/dev/null && ((tuple++))

            if [[ "$tuple" -eq 0 ]]; then
                # A bare color with no tuple behind it is an asserted verdict.
                if grep -qE 'RED|YELLOW|GREEN' "$report_file" 2>/dev/null; then
                    score=1
                else
                    score=0
                fi
            elif [[ "$tuple" -lt 4 ]]; then
                score=2
            else
                score=3
                if grep -qi 'Derived color' "$report_file" 2>/dev/null; then
                    score=$((score + 1))
                fi
                if grep -qi 'Unmet gates' "$report_file" 2>/dev/null; then
                    score=$((score + 1))
                fi
            fi
            ;;

        "report_structure")
            local sections=0
            grep -qE '^### Verdict' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'BLUF' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Review Contract and Routing' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Self-Grind Reconciliation' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Terminal Risks' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Risk Register' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Survived Scrutiny' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Not Examined' "$report_file" 2>/dev/null && ((sections++))
            grep -q "Grimey's Final Word" "$report_file" 2>/dev/null && ((sections++))

            if [[ "$sections" -ge 9 ]]; then
                score=5
            elif [[ "$sections" -ge 7 ]]; then
                score=4
            elif [[ "$sections" -ge 5 ]]; then
                score=3
            elif [[ "$sections" -ge 3 ]]; then
                score=2
            elif [[ "$sections" -ge 1 ]]; then
                score=1
            else
                score=0
            fi
            ;;

        "finding_format")
            local rows populated ratio
            rows=$(count_register_rows "$report_file")

            if [[ "$rows" -eq 0 ]]; then
                score=0
            else
                # A compliant row carries a category, a tier, and an assumption
                # status alongside its ID and severity.
                populated=$(count_matches '^\|[^|]*grime-[a-z0-9]+[^|]*\|.*\|.*(COR|INT|SEC|REL|OPS|PER|VER|MNT|DEP|HUM).*\|.*(E1|E2|E3).*\|.*\| *P[0-3] *\|.*(confirmed|unconfirmed|none).*\|' "$report_file")
                ratio=$((populated * 100 / rows))

                if [[ "$ratio" -ge 90 ]]; then
                    score=5
                elif [[ "$ratio" -ge 75 ]]; then
                    score=4
                elif [[ "$ratio" -ge 50 ]]; then
                    score=3
                elif [[ "$ratio" -ge 25 ]]; then
                    score=2
                else
                    score=1
                fi
            fi
            ;;

        "fix_quality")
            # Only scored if mode=fix (check for Fix sections)
            if grep -q '### Fix for' "$report_file" 2>/dev/null; then
                local fix_count fixes_with_verification
                fix_count=$(count_matches '^### Fix for' "$report_file")
                if [[ "$fix_count" -gt 0 ]]; then
                    fixes_with_verification=$(count_matches '\*\*Verification:\*\*' "$report_file")
                    if [[ "$fixes_with_verification" -ge "$fix_count" ]]; then
                        score=5
                    elif [[ "$fixes_with_verification" -ge $((fix_count / 2)) ]]; then
                        score=3
                    else
                        score=2
                    fi
                else
                    score=1
                fi
            else
                score="N/A" # Report mode, not applicable
            fi
            ;;

        "anti_pattern_avoidance")
            local optimism authority unearned hits=0

            optimism=$(count_matches '(probably|should be|likely) (be )?(fine|ok|okay|safe)|it will be fine' "$report_file")
            authority=$(count_matches 'LLM said|AI-generated.*fine|the model (says|said)' "$report_file")
            # An acquittal phrased as an impression is the unearned kind; the
            # skill requires a performed probe and its recorded result.
            unearned=$(count_matches 'looks (fine|sound|good|correct)|appears (fine|sound|correct|safe)|seems (fine|sound|correct)' "$report_file")

            [[ "$optimism" -gt 0 ]] && ((hits++))
            [[ "$authority" -gt 0 ]] && ((hits++))
            [[ "$unearned" -gt 0 ]] && ((hits++))

            case "$hits" in
                0) score=5 ;;
                1) score=3 ;;
                2) score=1 ;;
                *) score=0 ;;
            esac
            ;;

        "self_grind")
            local recon n m k acquittals probe_free
            recon=$(grep -oE '[0-9]+ candidates, [0-9]+ survived, [0-9]+ killed' "$report_file" 2>/dev/null | head -1 || true)

            if [[ -z "$recon" ]]; then
                score=0
            else
                n=$(echo "$recon" | awk '{print $1}')
                m=$(echo "$recon" | awk '{print $3}')
                k=$(echo "$recon" | awk '{print $5}')

                if [[ $((m + k)) -ne "$n" ]]; then
                    # Arithmetic that does not close means findings are unaccounted for.
                    score=1
                else
                    score=3
                    # Killed candidates must be listed with probe and result.
                    if [[ "$k" -gt 0 ]] && grep -q 'Disproof Probe' "$report_file" 2>/dev/null; then
                        score=$((score + 1))
                    fi
                    # Acquittals require a performed probe; unprobed claims
                    # belong in Not Examined instead.
                    acquittals=$(count_matches 'Specific Probe Performed' "$report_file")
                    probe_free=$(count_matches 'looks (fine|sound|good)|appears (fine|sound|correct)' "$report_file")
                    if [[ "$acquittals" -gt 0 ]] && [[ "$probe_free" -eq 0 ]]; then
                        score=$((score + 1))
                    fi
                fi
            fi
            ;;

        *)
            score=0
            ;;
    esac

    echo "$score"
}

score_report() {
    local report_file="$1"
    local target_name="$2"
    local output_file="$3"

    local invalid_reason=""
    if ! invalid_reason=$(validate_report "$report_file"); then
        # Record the failure without scores; a fragment has no comparable score.
        jq -n \
            --arg target "$target_name" \
            --arg timestamp "$TIMESTAMP" \
            --arg reason "$invalid_reason" \
            '{target: $target, timestamp: $timestamp, valid_report: false,
              invalid_reason: $reason, scores: {}, total_score: 0,
              max_possible: 0, percentage: 0}' >"$output_file"
        fail "malformed report: $invalid_reason"
        return 1
    fi

    {
        echo "{"
        echo "  \"target\": \"$target_name\","
        echo "  \"timestamp\": \"$TIMESTAMP\","
        echo "  \"valid_report\": true,"
        echo "  \"scores\": {"
    } >"$output_file"

    local dimensions=(
        "evidence_quality"
        "routing_discipline"
        "severity_assessment"
        "verdict_accuracy"
        "report_structure"
        "finding_format"
        "fix_quality"
        "anti_pattern_avoidance"
        "self_grind"
    )

    local first=true
    local total_score=0
    local max_possible=0

    for dim in "${dimensions[@]}"; do
        local score dim_max=5
        score=$(score_dimension "$dim" "$report_file" "")

        # Fix quality is N/A for report mode
        if [[ "$score" == "N/A" ]]; then
            continue
        fi

        if [[ "$first" != true ]]; then
            echo "," >>"$output_file"
        fi
        first=false

        echo "    \"$dim\": $score" >>"$output_file"
        total_score=$((total_score + score))
        max_possible=$((max_possible + dim_max))
    done

    {
        echo ""
        echo "  },"
        echo "  \"total_score\": $total_score,"
        echo "  \"max_possible\": $max_possible,"
    } >>"$output_file"

    # Calculate percentage
    if [[ "$max_possible" -gt 0 ]]; then
        local percentage=$((total_score * 100 / max_possible))
        echo "  \"percentage\": $percentage" >>"$output_file"
    else
        echo "  \"percentage\": 0" >>"$output_file"
    fi

    echo "}" >>"$output_file"
}

run_grind_manual() {
    local target_path="$1"
    local target_name="$2"
    local work_dir="$3"

    echo ""
    echo -e "${YELLOW}========================================${NC}"
    echo -e "${YELLOW}Manual Step Required${NC}"
    echo -e "${YELLOW}========================================${NC}"
    echo ""
    echo "The benchmark runner cannot automatically execute the grind."
    echo "Please follow these steps:"
    echo ""
    echo "1. Load the Frank Grimes skill in your AI agent"
    echo "2. Run a grind on the target:"
    echo ""
    echo "   Target: $target_path"
    echo "   Work directory: $work_dir"
    echo ""
    echo "3. Save the grind output to:"
    echo "   $work_dir/grind-output.md"
    echo ""
    echo "4. Return to this terminal and press Enter to continue scoring."
    echo ""
    read -r -p "Press Enter when the grind is complete and the report is saved..."

    if [[ ! -f "$work_dir/grind-output.md" ]]; then
        echo "ERROR: Grind output not found at $work_dir/grind-output.md"
        echo "The grind must produce a report file for scoring."
        return 1
    fi
}

run_grind_with_agent() {
    local target_path="$1"
    local target_name="$2"
    local work_dir="$3"

    # This is a placeholder for agent integration.
    # In a full implementation, this would call the agent's CLI.

    echo ""
    echo "NOTE: Automatic agent integration is not yet implemented."
    echo "Please run the grind manually and save the output to:"
    echo "  $work_dir/grind-output.md"
    echo ""

    read -r -p "Press Enter when ready to continue..."
}

# ============================================
# Main Execution
# ============================================

if [[ -n "$SCORE_ONLY" ]]; then
    # Score a saved report without re-running an agent. Used to check the
    # scorer against benchmark/fixtures/ after the report template changes.
    if [[ ! -f "$SCORE_ONLY" ]]; then
        echo "ERROR: report not found: $SCORE_ONLY"
        exit 1
    fi

    score_file=$(mktemp)
    if ! score_report "$SCORE_ONLY" "$(basename "$SCORE_ONLY")" "$score_file"; then
        rm -f "$score_file"
        exit 2
    fi

    jq -r '.scores | to_entries[] | "  \(.key): \(.value)"' "$score_file"
    echo ""
    echo -e "Score: $(jq -r '.total_score' "$score_file")/$(jq -r '.max_possible' "$score_file") ($(jq -r '.percentage' "$score_file")%)"
    rm -f "$score_file"

elif [[ "$ALL" == true ]]; then
    info "Running benchmark against all targets..."
    echo ""

    # Define all targets
    TARGETS=(
        "shell:bad-script.sh:benchmark/targets/shell/bad-script.sh"
        "go:bad-service:benchmark/targets/go/bad-service"
        "web:bad-endpoint:benchmark/targets/web/bad-endpoint"
    )

    for target_entry in "${TARGETS[@]}"; do
        IFS=':' read -r target_type target_name target_path <<<"$target_entry"

        echo -e "${BLUE}========================================${NC}"
        echo -e "${BLUE}Target: $target_name ($target_type)${NC}"
        echo -e "${BLUE}========================================${NC}"

        # Create work directory
        work_dir="$RESULTS_DIR/$target_name-$TIMESTAMP"
        mkdir -p "$work_dir"

        # Copy target to work directory if it's a file
        if [[ -f "$target_path" ]]; then
            cp "$target_path" "$work_dir/"
        fi

        # Run the grind
        run_grind_manual "$target_path" "$target_name" "$work_dir"

        # Score the report
        report_file="$work_dir/grind-output.md"
        output_file="$work_dir/score.json"

        if [[ -f "$report_file" ]]; then
            score_report "$report_file" "$target_name" "$output_file"

            # Display score
            if [[ -f "$output_file" ]]; then
                percentage=$(jq -r '.percentage' "$output_file")
                total=$(jq -r '.total_score' "$output_file")
                echo ""
                echo -e "Score: ${total}/50 (${percentage}%)"
            fi
        else
            fail "No report found for $target_name"
        fi

        echo ""
    done

    # Summary
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Benchmark Summary${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""

    for target_entry in "${TARGETS[@]}"; do
        IFS=':' read -r target_type target_name target_path <<<"$target_entry"
        work_dir="$RESULTS_DIR/$target_name-$TIMESTAMP"
        output_file="$work_dir/score.json"

        if [[ -f "$output_file" ]]; then
            percentage=$(jq -r '.percentage' "$output_file")
            echo -e "$target_name: ${percentage}%"
        fi
    done

    # Compare with baseline if requested
    if [[ "$COMPARE" == true ]] && [[ -d "$BASELINE_DIR" ]]; then
        echo ""
        echo -e "${YELLOW}========================================${NC}"
        echo -e "${YELLOW}A/B Comparison with Baseline${NC}"
        echo -e "${YELLOW}========================================${NC}"
        echo ""

        for target_entry in "${TARGETS[@]}"; do
            IFS=':' read -r target_type target_name target_path <<<"$target_entry"

            baseline_file="$BASELINE_DIR/$target_name/*.json"
            current_file="$RESULTS_DIR/$target_name-$TIMESTAMP/score.json"

            if [[ -f "$current_file" ]]; then
                current_pct=$(jq -r '.percentage' "$current_file")

                # Find baseline file
                baseline_match=$(find "$BASELINE_DIR" -name "$(basename "$baseline_file")" 2>/dev/null | head -1)
                if [[ -n "$baseline_match" ]] && [[ -f "$baseline_match" ]]; then
                    baseline_pct=$(jq -r '.percentage' "$baseline_match")
                    diff=$((current_pct - baseline_pct))

                    if [[ "$diff" -gt 0 ]]; then
                        echo -e "$target_name: ${GREEN}+${diff}pp${NC} (baseline: ${baseline_pct}%, current: ${current_pct}%)"
                    elif [[ "$diff" -lt 0 ]]; then
                        echo -e "$target_name: ${RED}${diff}pp${NC} (baseline: ${baseline_pct}%, current: ${current_pct}%)"
                    else
                        echo -e "$target_name: ${NC}no change (baseline: ${baseline_pct}%, current: ${current_pct}%)"
                    fi
                else
                    echo -e "$target_name: ${YELLOW}no baseline to compare${NC} (current: ${current_pct}%)"
                fi
            fi
        done
    fi

elif [[ -n "$TARGET" ]]; then
    # Single target mode
    target_path="$TARGET"
    target_name=$(basename "$target_path")

    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}Benchmark: $target_name${NC}"
    echo -e "${BLUE}========================================${NC}"

    work_dir="$RESULTS_DIR/$target_name-$TIMESTAMP"
    mkdir -p "$work_dir"

    if [[ -f "$target_path" ]]; then
        cp "$target_path" "$work_dir/"
    fi

    run_grind_manual "$target_path" "$target_name" "$work_dir"

    report_file="$work_dir/grind-output.md"
    output_file="$work_dir/score.json"

    if [[ -f "$report_file" ]]; then
        score_report "$report_file" "$target_name" "$output_file"

        if [[ -f "$output_file" ]]; then
            percentage=$(jq -r '.percentage' "$output_file")
            total=$(jq -r '.total_score' "$output_file")
            echo ""
            echo -e "Score: ${total}/50 (${percentage}%)"
        fi
    fi

else
    # No target specified
    echo "Frank Grimes Benchmark Runner"
    echo ""
    echo "Usage:"
    echo "  ./benchmark/runner.sh <target-path>    Run against a single target"
    echo "  ./benchmark/runner.sh --all            Run against all targets"
    echo "  ./benchmark/runner.sh --all --compare  Compare with baseline results"
    echo ""
    list_targets
    exit 1
fi
