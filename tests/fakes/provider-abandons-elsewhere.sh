#!/usr/bin/env bash
# Accumulates into a report of its own choosing and dies without sealing.
#
# The default path is a default, not a rule: a role may name its own file, and
# what it leaves behind is still this pass's to clean up.
set -euo pipefail

# No mkdir: the engine made the one directory this pass may write, and making
# it again means creating .grimes, which the boundary refuses.
grimes-contract report add --file=.grimes/work/mine.textproto \
    --category=SEC --severity=P0 --blast=systemic --likelihood=likely \
    --path="${GRIMES_TARGET_SCOPE:-src}/app.sh" --tier=E2 \
    --claim="a claim nobody sealed" \
    --quote="$(grep -m1 -E '[^[:space:]]' "${GRIMES_TARGET_ROOT:-.}/${GRIMES_TARGET_SCOPE:-src}/app.sh")" \
    --disproof-action="tried" --disproof-exit=1 --disproof-output="still there" >/dev/null
echo "this pass is not coming back either" >&2
exit 1
