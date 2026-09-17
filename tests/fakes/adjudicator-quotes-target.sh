#!/usr/bin/env bash
# An independent reviewer that says what the artifact held when it looked.
#
# In fix mode the primary reviews and repairs in one pass, so this runs after
# the bytes on disk have been changed on purpose. What it reports is what it
# read, and the test reads it back: a reviewer shown the batch would report the
# repair, and would be judging work nobody asked it about.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"

UNIT="${1:-src/app.sh}"
SEEN="$GRIMES_TARGET_ROOT/$UNIT"
if [[ ! -f "$SEEN" ]]; then
    echo "adjudicator cannot read $SEEN" >&2
    exit 1
fi
# Written where the test can read it; a role may write only this directory.
cp "$SEEN" "$GRIMES_WORK_DIR/adjudicator-saw"

"$HERE/adjudicate.sh" DECISION_PASS RESIDUAL_RISK_LOW REVIEW_CONFIDENCE_HIGH REVIEW_COMPLETENESS_SUFFICIENT
