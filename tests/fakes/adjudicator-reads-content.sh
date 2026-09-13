#!/usr/bin/env bash
# An independent reviewer that reads the artifact before agreeing.
#
# Zero knowledge is about the first report, not the target: an adjudicator that
# cannot see what it is judging cannot form an opinion of its own.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"

if [[ -z "${GRIMES_TARGET_CONTENT:-}" || ! -e "$GRIMES_TARGET_CONTENT" ]]; then
    echo "adjudicator cannot read the artifact it is judging" >&2
    exit 1
fi
for leak in GRIMES_FINDING GRIMES_EVIDENCE GRIMES_LEDGER GRIMES_SEVERITY GRIMES_SUMMARY; do
    if env | grep -q "^${leak}"; then
        echo "adjudicator received ${leak}; independence is broken" >&2
        exit 1
    fi
done

"$HERE/adjudicate.sh" DECISION_PASS RESIDUAL_RISK_LOW REVIEW_CONFIDENCE_HIGH REVIEW_COMPLETENESS_SUFFICIENT
