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
PROJECT_ROOT="$(cd "$BENCHMARK_DIR/.." && pwd)"
RUBBER="$BENCHMARK_DIR/rubric.md"
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
        *)
            TARGET="$1"
            shift
            ;;
    esac
done

# Check for jq
if ! command -v jq &> /dev/null; then
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

score_dimension() {
    local dimension="$1"
    local report_file="$2"
    local target_type="$3"

    # This is a heuristic scoring function. In a full implementation,
    # this would use an LLM to evaluate the report against the rubric.
    # For now, we check for the presence of key elements.

    local score=0
    local max_score=5

    case "$dimension" in
        "evidence_quality")
            # Check for specific evidence patterns
            if grep -q '### Issue:' "$report_file" 2>/dev/null; then
                local issue_count=$(grep -c '### Issue:' "$report_file" 2>/dev/null || echo 0)
                if [[ "$issue_count" -ge 10 ]]; then
                    score=4
                elif [[ "$issue_count" -ge 5 ]]; then
                    score=3
                elif [[ "$issue_count" -ge 2 ]]; then
                    score=2
                else
                    score=1
                fi
                # Check for evidence field
                if grep -q '**Evidence:**' "$report_file" 2>/dev/null; then
                    score=$((score + 1))
                fi
            else
                score=0
            fi
            ;;

        "category_coverage")
            # Count distinct categories mentioned
            local categories=$(grep -oP '\*\*Category:\*\* \K.*' "$report_file" 2>/dev/null | sort -u | wc -l || echo 0)
            if [[ "$categories" -ge 10 ]]; then
                score=5
            elif [[ "$categories" -ge 7 ]]; then
                score=4
            elif [[ "$categories" -ge 5 ]]; then
                score=3
            elif [[ "$categories" -ge 3 ]]; then
                score=2
            elif [[ "$categories" -ge 1 ]]; then
                score=1
            else
                score=0
            fi
            ;;

        "severity_assessment")
            # Check for proper severity distribution
            local p0_count=$(grep -c '**Severity:** P0' "$report_file" 2>/dev/null || echo 0)
            local p1_count=$(grep -c '**Severity:** P1' "$report_file" 2>/dev/null || echo 0)
            local p2_count=$(grep -c '**Severity:** P2' "$report_file" 2>/dev/null || echo 0)
            local p3_count=$(grep -c '**Severity:** P3' "$report_file" 2>/dev/null || echo 0)

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
                score=$((score - 1))  # Penalize P0 overuse
            fi
            ;;

        "verdict_accuracy")
            if grep -q '### Verdict:' "$report_file" 2>/dev/null; then
                local verdict=$(grep '### Verdict:' "$report_file" | head -1 | sed 's/### Verdict: //')
                if [[ "$verdict" == *"RED"* ]] || [[ "$verdict" == *"YELLOW"* ]] || [[ "$verdict" == *"GREEN"* ]]; then
                    score=3
                    # Check for justification
                    if grep -q 'BLUF' "$report_file" 2>/dev/null; then
                        score=4
                    fi
                    # Check for stop condition reference
                    if grep -qi 'P0.*mitigat' "$report_file" 2>/dev/null; then
                        score=5
                    fi
                else
                    score=1
                fi
            else
                score=0
            fi
            ;;

        "report_structure")
            local sections=0
            grep -q '### Verdict:' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'BLUF' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Top 3 Risks' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Origin Assessment' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Risk Register' "$report_file" 2>/dev/null && ((sections++))
            grep -q 'Survived Scrutiny' "$report_file" 2>/dev/null && ((sections++))
            grep -q "Grimey's Final Word" "$report_file" 2>/dev/null && ((sections++))

            if [[ "$sections" -ge 7 ]]; then
                score=5
            elif [[ "$sections" -ge 5 ]]; then
                score=4
            elif [[ "$sections" -ge 4 ]]; then
                score=3
            elif [[ "$sections" -ge 2 ]]; then
                score=2
            elif [[ "$sections" -ge 1 ]]; then
                score=1
            else
                score=0
            fi
            ;;

        "issue_format")
            local issues_with_grime_id=$(grep -c '**Grime ID:**' "$report_file" 2>/dev/null || echo 0)
            local issues_with_evidence=$(grep -c '**Evidence:**' "$report_file" 2>/dev/null || echo 0)
            local issues_with_category=$(grep -c '**Category:**' "$report_file" 2>/dev/null || echo 0)
            local issues_with_severity=$(grep -c '**Severity:**' "$report_file" 2>/dev/null || echo 0)

            local total_issues=$(grep -c '### Issue:' "$report_file" 2>/dev/null || echo 0)

            if [[ "$total_issues" -eq 0 ]]; then
                score=0
            else
                local compliant=$((issues_with_grime_id + issues_with_evidence + issues_with_category + issues_with_severity))
                local max_compliant=$((total_issues * 4))

                if [[ "$max_compliant" -gt 0 ]]; then
                    local ratio=$((compliant * 100 / max_compliant))
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
            fi
            ;;

        "fix_quality")
            # Only scored if mode=fix (check for Fix sections)
            if grep -q '### Fix for' "$report_file" 2>/dev/null; then
                local fix_count=$(grep -c '### Fix for' "$report_file" 2>/dev/null || echo 0)
                if [[ "$fix_count" -gt 0 ]]; then
                    local fixes_with_verification=$(grep -c '**Verification:**' "$report_file" 2>/dev/null || echo 0)
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
                score="N/A"  # Report mode, not applicable
            fi
            ;;

        "anti_pattern_avoidance")
            # Check for anti-pattern indicators
            local optimism_creed=0
            if grep -qi 'probably be fine' "$report_file" 2>/dev/null; then
                optimism_creed=1
            fi

            local authority_deference=0
            if grep -qi 'LLM said' "$report_file" 2>/dev/null || grep -qi 'AI-generated.*fine' "$report_file" 2>/dev/null; then
                authority_deference=1
            fi

            if [[ "$optimism_creed" -eq 0 ]] && [[ "$authority_deference" -eq 0 ]]; then
                score=5
            elif [[ "$optimism_creed" -eq 0 ]] || [[ "$authority_deference" -eq 0 ]]; then
                score=3
            else
                score=1
            fi
            ;;

        "voice_and_tone")
            # Check for Grimey voice indicators
            local has_clinical=0
            local has_final_word=0

            if grep -qi 'broken' "$report_file" 2>/dev/null; then
                has_clinical=1
            fi

            if grep -q "Grimey's Final Word" "$report_file" 2>/dev/null; then
                has_final_word=1
            fi

            if [[ "$has_clinical" -eq 1 ]] && [[ "$has_final_word" -eq 1 ]]; then
                score=4
            elif [[ "$has_clinical" -eq 1 ]] || [[ "$has_final_word" -eq 1 ]]; then
                score=2
            else
                score=1
            fi
            ;;

        "origin_assessment")
            if grep -q 'Origin Assessment' "$report_file" 2>/dev/null; then
                if grep -q '\[x\]' "$report_file" 2>/dev/null || grep -q '\[ \]' "$report_file" 2>/dev/null; then
                    score=4
                else
                    score=2
                fi
            else
                score=0
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

    echo "{" > "$output_file"
    echo "  \"target\": \"$target_name\"," >> "$output_file"
    echo "  \"timestamp\": \"$TIMESTAMP\"," >> "$output_file"
    echo "  \"scores\": {" >> "$output_file"

    local dimensions=(
        "evidence_quality"
        "category_coverage"
        "severity_assessment"
        "verdict_accuracy"
        "report_structure"
        "issue_format"
        "fix_quality"
        "anti_pattern_avoidance"
        "voice_and_tone"
        "origin_assessment"
    )

    local first=true
    local total_score=0
    local max_possible=0

    for dim in "${dimensions[@]}"; do
        local score=$(score_dimension "$dim" "$report_file" "")
        local dim_max=5

        # Fix quality is N/A for report mode
        if [[ "$score" == "N/A" ]]; then
            continue
        fi

        if [[ "$first" != true ]]; then
            echo "," >> "$output_file"
        fi
        first=false

        echo "    \"$dim\": $score" >> "$output_file"
        total_score=$((total_score + score))
        max_possible=$((max_possible + dim_max))
    done

    echo "" >> "$output_file"
    echo "  }," >> "$output_file"
    echo "  \"total_score\": $total_score," >> "$output_file"
    echo "  \"max_possible\": $max_possible," >> "$output_file"

    # Calculate percentage
    if [[ "$max_possible" -gt 0 ]]; then
        local percentage=$((total_score * 100 / max_possible))
        echo "  \"percentage\": $percentage" >> "$output_file"
    else
        echo "  \"percentage\": 0" >> "$output_file"
    fi

    echo "}" >> "$output_file"
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
    read -p "Press Enter when the grind is complete and the report is saved..."

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

    read -p "Press Enter when ready to continue..."
}

# ============================================
# Main Execution
# ============================================

if [[ "$ALL" == true ]]; then
    info "Running benchmark against all targets..."
    echo ""

    # Define all targets
    TARGETS=(
        "shell:bad-script.sh:benchmark/targets/shell/bad-script.sh"
        "go:bad-service:benchmark/targets/go/bad-service"
        "web:bad-endpoint:benchmark/targets/web/bad-endpoint"
    )

    for target_entry in "${TARGETS[@]}"; do
        IFS=':' read -r target_type target_name target_path <<< "$target_entry"

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
                local percentage=$(jq -r '.percentage' "$output_file")
                local total=$(jq -r '.total_score' "$output_file")
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
        IFS=':' read -r target_type target_name target_path <<< "$target_entry"
        work_dir="$RESULTS_DIR/$target_name-$TIMESTAMP"
        output_file="$work_dir/score.json"

        if [[ -f "$output_file" ]]; then
            local percentage=$(jq -r '.percentage' "$output_file")
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
            IFS=':' read -r target_type target_name target_path <<< "$target_entry"

            baseline_file="$BASELINE_DIR/$target_name/*.json"
            current_file="$RESULTS_DIR/$target_name-$TIMESTAMP/score.json"

            if [[ -f "$current_file" ]]; then
                local current_pct=$(jq -r '.percentage' "$current_file")

                # Find baseline file
                local baseline_match=$(ls $baseline_file 2>/dev/null | head -1)
                if [[ -n "$baseline_match" ]] && [[ -f "$baseline_match" ]]; then
                    local baseline_pct=$(jq -r '.percentage' "$baseline_match")
                    local diff=$((current_pct - baseline_pct))

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
            local percentage=$(jq -r '.percentage' "$output_file")
            local total=$(jq -r '.total_score' "$output_file")
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
