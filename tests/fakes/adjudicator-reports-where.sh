#!/usr/bin/env bash
# Writes down where it was told the target is, and what it can read there.
#
# The roles after a fixing one are pointed at a copy taken before the batch,
# and what the test needs to know is what that copy actually holds: a link the
# copy kept has to resolve inside it, and one that pointed out of the tree must
# not be there to follow at all.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"

{
    echo "content=${GRIMES_TARGET_CONTENT:-}"
    echo "root=${GRIMES_TARGET_ROOT:-}"
    for f in "$@"; do
        if [[ -e "$GRIMES_TARGET_ROOT/$f" ]]; then
            echo "read $f=$(cat "$GRIMES_TARGET_ROOT/$f" 2>&1)"
        else
            echo "absent $f"
        fi
    done
} >"$GRIMES_WORK_DIR/where" 2>&1

"$HERE/adjudicate.sh" DECISION_PASS RESIDUAL_RISK_LOW REVIEW_CONFIDENCE_HIGH REVIEW_COMPLETENESS_SUFFICIENT
