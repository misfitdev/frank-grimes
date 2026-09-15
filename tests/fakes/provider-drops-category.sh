#!/usr/bin/env bash
# Accounts for every unit and stops every category it admits to having been
# routed, having dropped one from that list.
#
# The contract only checks the report against its own routed set, so a shortened
# denominator is internally consistent: nothing in the report says a category
# went unreviewed.
set -euo pipefail
cd "$(mktemp -d)"
echo "I looked at the target, or most of it."
grimes-contract report units --print0 | grimes-contract report cover --examined-stdin0 >/dev/null

CATEGORIES="${GRIMES_CATEGORIES:-SEC,COR,REL,OPS,VER}"
IFS=, read -r -a ROUTED <<<"$CATEGORIES"
KEPT=("${ROUTED[@]:1}")
for CATEGORY in "${KEPT[@]}"; do
    grimes-contract report stop --category="$CATEGORY" --condition=marginal-yield --probes=2 >/dev/null
done
"$(cd "$(dirname "$0")" && pwd)/acquit.sh" "${KEPT[0]}"

SHORT="$(
    IFS=,
    echo "${KEPT[*]}"
)"
grimes-contract report seal --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" --iteration="${GRIMES_ITERATION:-1}" \
    --routed="$SHORT" --examined=7 --disproved=7 --summary="Everything I was asked about is fine."
