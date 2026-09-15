#!/usr/bin/env bash
# A refutation pass with a fixed outcome for every claim it is handed.
#
# Fails loudly if it was told anything that argues for a claim. A refuter shown
# the severity, the reporter, or the evidence narrative is checking the case for
# the claim rather than the claim, which is the hole the pass exists to close.
#
# Usage: refute.sh refuted|upheld|unavailable
set -euo pipefail

OUTCOME="$1"

for leak in GRIMES_FINDING GRIMES_EVIDENCE GRIMES_LEDGER GRIMES_SEVERITY GRIMES_SUMMARY GRIMES_CLAIMED GRIMES_TARGET_INVENTORY GRIMES_CATEGORIES; do
    if env | grep -q "^${leak}="; then
        echo "refuter received ${leak}; the claim is arguing for itself" >&2
        exit 1
    fi
done
if [[ "${GRIMES_ROLE:-}" != "refuter" ]]; then
    echo "refuter was not addressed as one" >&2
    exit 1
fi

# prototext spacing is randomized per binary build, so the pattern tolerates
# any amount of it rather than matching what one build happened to emit.
TASK="$(grimes-contract decode --type=RefutationTask "${GRIMES_CLAIMS}")"
REFS="$(echo "$TASK" | sed -n 's/^[[:space:]]*ref:[[:space:]]*"\(.*\)"[[:space:]]*$/\1/p')"

# shellcheck disable=SC2001  # per-pair substitution is what sed is for here
ESCAPED="$(echo "${GRIMES_TARGET_FINGERPRINT}" | sed 's/../\\x&/g')"

{
    echo 'schema_major: 2'
    echo "run_id: \"${GRIMES_RUN_ID}\""
    echo 'refuter_id: "fake-refuter"'
    echo "target_fingerprint_sha256: \"${ESCAPED}\""
    for ref in $REFS; do
        echo "outcomes {"
        echo "  ref: \"${ref}\""
        case "$OUTCOME" in
            unavailable)
                echo '  unavailable: "no runtime available to exercise this claim"'
                ;;
            *)
                echo "  ${OUTCOME} {"
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
    done
    echo 'completed_at { seconds: 1780000000 }'
} | grimes-contract encode-report --type=RefutationReport /dev/stdin
