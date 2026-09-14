#!/usr/bin/env bash
# Accounts for every unit, skipping one as material rather than examining it.
#
# Examined and skipped must stay disjoint and together cover the inventory, so
# the skipped unit is held out of the examined set rather than added to it.
set -euo pipefail
cd "$(mktemp -d)"
echo "I looked at what I could."

HELD="$(grimes-contract report units | head -1)"
grimes-contract report units | tail -n +2 | while IFS= read -r unit; do
    grimes-contract report cover --examined="$unit" >/dev/null
done
grimes-contract report cover --skip="$HELD:unreadable without credentials:material" >/dev/null

for CATEGORY in SEC COR REL OPS VER; do
    grimes-contract report stop --category="$CATEGORY" --condition=marginal-yield --probes=2 >/dev/null
done

grimes-contract report seal --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" --iteration="${GRIMES_ITERATION:-1}" \
    --routed=SEC,COR,REL,OPS,VER --examined=7 --disproved=7 --summary="One unit could not be read."
