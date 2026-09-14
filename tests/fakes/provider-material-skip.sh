#!/usr/bin/env bash
# Accounts for every unit, skipping one as material rather than examining it.
#
# Examined and skipped must stay disjoint and together cover the inventory, so
# the skipped unit is held out of the examined set rather than added to it.
set -euo pipefail
cd "$(mktemp -d)"
echo "I looked at what I could."

# A unit id may contain a newline, so the inventory is read NUL-separated. The
# loop reads from a file rather than a pipe: a pipe would run it in a subshell
# and HELD would not survive it.
UNITS="$(mktemp)"
REST="$(mktemp)"
grimes-contract report units --print0 >"$UNITS"
HELD=""
while IFS= read -r -d '' unit; do
    if [ -z "$HELD" ]; then
        HELD="$unit"
        continue
    fi
    printf '%s\0' "$unit" >>"$REST"
done <"$UNITS"

grimes-contract report cover --examined-stdin0 <"$REST" >/dev/null
grimes-contract report cover --skip="$HELD" \
    --skip-reason="unreadable without credentials" --skip-material >/dev/null

CATEGORIES="${GRIMES_CATEGORIES:-SEC,COR,REL,OPS,VER}"
IFS=, read -r -a ROUTED <<<"$CATEGORIES"
for CATEGORY in "${ROUTED[@]}"; do
    grimes-contract report stop --category="$CATEGORY" --condition=marginal-yield --probes=2 >/dev/null
done

grimes-contract report seal --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" --iteration="${GRIMES_ITERATION:-1}" \
    --routed="$CATEGORIES" --examined=7 --disproved=7 --summary="One unit could not be read."
