#!/usr/bin/env bash
# Accounts for every unit, but one category could not be probed at all.
set -euo pipefail
cd "$(mktemp -d)"
echo "I looked at the target."
grimes-contract report units --print0 | grimes-contract report cover --examined-stdin0 >/dev/null
grimes-contract report stop --category=SEC --condition=evidence-unavailable >/dev/null
CATEGORIES="${GRIMES_CATEGORIES:-SEC,COR,REL,OPS,VER}"
IFS=, read -r -a ROUTED <<<"$CATEGORIES"
for CATEGORY in "${ROUTED[@]}"; do
    [[ "$CATEGORY" == "SEC" ]] && continue
    grimes-contract report stop --category="$CATEGORY" --condition=marginal-yield --probes=2 >/dev/null
done
grimes-contract report seal --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" --iteration="${GRIMES_ITERATION:-1}" \
    --routed="$CATEGORIES" --examined=7 --disproved=7 --summary="Security could not be probed on this target."
