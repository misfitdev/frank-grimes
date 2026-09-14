#!/usr/bin/env bash
# Seals whatever candidates the caller added, the way the adapter documents.
#
# The engine owns the run's identity, the target's, and the iteration. A report
# answers one request, so a provider echoes what it was given rather than
# minting its own; every fake goes through this for that reason.
set -euo pipefail

# Account for every unit the engine resolved. A real provider does the same:
# the inventory is the denominator its coverage is measured against, and a unit
# left unaccounted for caps the verdict rather than passing quietly.
#
# Callers cd into a scratch directory before building a report, so the path is
# absolute and exported by the engine rather than found relative to here.
if [[ -n "${GRIMES_TARGET_INVENTORY:-}" && -f "$GRIMES_TARGET_INVENTORY" ]]; then
    UNITS="$(grimes-contract decode-report --type=TargetInventory "$GRIMES_TARGET_INVENTORY" |
        grep -oE 'id: +"[^"]*"' | sed 's/.*"\(.*\)"/\1/' | paste -sd, -)"
    [[ -n "$UNITS" ]] && grimes-contract report cover --examined="$UNITS" >/dev/null
fi

# Every routed category records what ended its grind.
for CATEGORY in SEC COR REL OPS VER; do
    grimes-contract report stop --category="$CATEGORY" \
        --condition=marginal-yield --probes=2 >/dev/null
done

grimes-contract report seal \
    --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" \
    --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" \
    --iteration="${GRIMES_ITERATION:-1}" \
    --routed=SEC,COR,REL,OPS,VER \
    --examined="$1" --disproved="$2" --summary="$3"
