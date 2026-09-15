#!/usr/bin/env bash
# A refutation pass with a fixed outcome for every claim it is handed.
#
# Fails loudly if it was told anything that argues for a claim. A refuter shown
# the severity, the reporter, or the evidence narrative is checking the case for
# a claim rather than the claim, which is the hole the pass exists to close.
#
# The engine plants a control claim about an identifier that is not in the
# target. This fake breaks it the way an honest refuter would, by looking: if
# the claim names an identifier absent from the artifact, it is refuted. Pass
# `rubberstamp` to get the failure mode instead, where the fixed outcome is
# applied to the control too.
#
# Usage: refute.sh refuted|upheld|unavailable [rubberstamp]
set -euo pipefail

OUTCOME="$1"
STAMP="${2:-}"

for leak in GRIMES_FINDING GRIMES_EVIDENCE GRIMES_LEDGER GRIMES_SEVERITY GRIMES_SUMMARY GRIMES_CLAIMED GRIMES_TARGET_INVENTORY GRIMES_CATEGORIES; do
    if env | grep -q "^${leak}="; then
        echo "refuter was told ${leak}" >&2
        exit 1
    fi
done

if [[ "${GRIMES_ROLE:-}" != "refuter" ]]; then
    echo "refuter invoked as role ${GRIMES_ROLE:-none}" >&2
    exit 1
fi

# prototext spacing is randomized per binary build, so every pattern here
# tolerates any amount of it rather than matching what one build happened to
# emit. A claim's ref is the first field of its block and its text the last, so
# pairing them in order needs no brace tracking.
TASK="$(grimes-contract decode --type=RefutationTask "${GRIMES_CLAIMS}")"
PAIRS="$(echo "$TASK" | awk '
    match($0, /^[[:space:]]*ref:[[:space:]]*"/) { ref = $0; sub(/^[[:space:]]*ref:[[:space:]]*"/, "", ref); sub(/"[[:space:]]*$/, "", ref); next }
    match($0, /^[[:space:]]*claim:[[:space:]]*"/) { c = $0; sub(/^[[:space:]]*claim:[[:space:]]*"/, "", c); sub(/"[[:space:]]*$/, "", c); print ref "\t" c }
')"

# shellcheck disable=SC2001 # a per-pair substitution is what sed is for here
ESCAPED="$(echo "${GRIMES_TARGET_FINGERPRINT}" | sed 's/../\\x&/g')"

outcome_for() {
    local claim="$1" token
    token="$(echo "$claim" | grep -oE 'fgq[0-9a-f]{13}' || true)"
    if [[ -n "$token" && "$STAMP" != "rubberstamp" ]] &&
        ! grep -rqF "$token" --exclude-dir=.grimes . 2>/dev/null; then
        echo refuted
        return
    fi
    echo "$OUTCOME"
}

{
    echo 'schema_major: 2'
    echo "run_id: \"${GRIMES_RUN_ID}\""
    echo 'refuter_id: "fake-refuter"'
    echo "target_fingerprint_sha256: \"${ESCAPED}\""
    while IFS=$'\t' read -r ref claim; do
        [[ -n "$ref" ]] || continue
        echo "outcomes {"
        echo "  ref: \"${ref}\""
        case "$(outcome_for "$claim")" in
            unavailable)
                echo '  unavailable: "no runtime available to exercise this claim"'
                ;;
            refuted)
                echo '  refuted {'
                echo '    executed_command {'
                echo '      action: "grep -rF <identifier> ."'
                echo '      cwd { value: "." }'
                echo '      exit_code: 1'
                echo '      output_excerpt: "no such identifier in the target"'
                echo '    }'
                echo '    completed_at { seconds: 1780000000 }'
                echo '  }'
                ;;
            *)
                echo '  upheld {'
                echo '    executed_command {'
                echo '      action: "sh -c true"'
                echo '      cwd { value: "." }'
                echo '      exit_code: 0'
                echo '      output_excerpt: "attacked the claim directly"'
                echo '    }'
                echo '    completed_at { seconds: 1780000000 }'
                echo '  }'
                ;;
        esac
        echo "}"
    done <<<"$PAIRS"
    echo 'completed_at { seconds: 1780000000 }'
} | grimes-contract encode-report --type=RefutationReport /dev/stdin
