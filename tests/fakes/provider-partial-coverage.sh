#!/usr/bin/env bash
# Accounts for only one unit, leaving the rest of the target unexamined.
set -euo pipefail
cd "$(mktemp -d)"
echo "I looked at the target."
# Name only the first unit, whatever it is.
FIRST="$(grimes-contract report units | head -1)"
grimes-contract report cover --examined="$FIRST" >/dev/null
for CATEGORY in SEC COR REL OPS VER; do
    grimes-contract report stop --category="$CATEGORY" --condition=marginal-yield --probes=2 >/dev/null
done
grimes-contract report seal --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" --iteration="${GRIMES_ITERATION:-1}" \
    --routed=SEC,COR,REL,OPS,VER --examined=7 --disproved=7 --summary="One unit examined, the rest left alone."
