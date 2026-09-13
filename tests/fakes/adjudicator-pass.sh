#!/usr/bin/env bash
# An independent reviewer that agrees. Fails loudly if handed anything beyond
# the target identity and the claimed tuple.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"

for leak in GRIMES_FINDING GRIMES_EVIDENCE GRIMES_LEDGER GRIMES_SEVERITY GRIMES_SUMMARY; do
    if env | grep -q "^${leak}"; then
        echo "adjudicator received ${leak}; independence is broken" >&2
        exit 1
    fi
done
if [[ -z "${GRIMES_CLAIMED_DECISION:-}" ]]; then
    echo "adjudicator received no claimed decision" >&2
    exit 1
fi

"$HERE/adjudicate.sh" DECISION_PASS RESIDUAL_RISK_LOW REVIEW_CONFIDENCE_HIGH REVIEW_COMPLETENESS_SUFFICIENT
