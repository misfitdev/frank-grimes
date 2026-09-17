#!/usr/bin/env bash
# A second reviewer that reaches the same verdict as adjudicator-pass.sh.
#
# Identical in behaviour and distinct as a command, which is what a panel of
# two needs: the engine refuses the same command twice.
# An independent reviewer that agrees. Fails loudly if handed anything beyond
# the target identity — including the verdict it is meant to reach on its own.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"

for leak in GRIMES_FINDING GRIMES_EVIDENCE GRIMES_LEDGER GRIMES_SEVERITY GRIMES_SUMMARY GRIMES_CLAIMED; do
    if env | grep -q "^${leak}"; then
        echo "adjudicator received ${leak}; independence is broken" >&2
        exit 1
    fi
done
if [[ "${GRIMES_ROLE:-}" != "adjudicator" ]]; then
    echo "adjudicator was not addressed as one" >&2
    exit 1
fi

"$HERE/adjudicate.sh" DECISION_PASS RESIDUAL_RISK_LOW REVIEW_CONFIDENCE_HIGH REVIEW_COMPLETENESS_SUFFICIENT
