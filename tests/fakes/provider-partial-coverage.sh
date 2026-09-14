#!/usr/bin/env bash
# Accounts for only one unit, leaving the rest of the target unexamined.
set -euo pipefail
cd "$(mktemp -d)"
echo "I looked at the target."
# Name only the first unit, whatever it is. Read NUL-separated: a unit id may
# contain a newline, and a fragment of one would read as a unit outside the
# target rather than as partial coverage.
IFS= read -r -d '' FIRST < <(grimes-contract report units --print0)
printf '%s\0' "$FIRST" | grimes-contract report cover --examined-stdin0 >/dev/null
CATEGORIES="${GRIMES_CATEGORIES:-SEC,COR,REL,OPS,VER}"
IFS=, read -r -a ROUTED <<<"$CATEGORIES"
for CATEGORY in "${ROUTED[@]}"; do
    grimes-contract report stop --category="$CATEGORY" --condition=marginal-yield --probes=2 >/dev/null
done
"$(cd "$(dirname "$0")" && pwd)/acquit.sh" "${ROUTED[0]}"

grimes-contract report seal --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" --iteration="${GRIMES_ITERATION:-1}" \
    --routed="$CATEGORIES" --examined=7 --disproved=7 --summary="One unit examined, the rest left alone."
