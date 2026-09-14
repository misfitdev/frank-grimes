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
# NUL-separated: a unit id is arbitrary text and may contain a comma or a
# newline, which every other separator would split in the wrong place.
if [[ -n "${GRIMES_TARGET_INVENTORY:-}" && -f "$GRIMES_TARGET_INVENTORY" ]]; then
    grimes-contract report units --print0 |
        grimes-contract report cover --examined-stdin0 >/dev/null
fi

# Every routed category records what ended its grind. The routed set comes from
# the engine, not from here: a provider that names its own shorter list is
# answering a request nobody made.
CATEGORIES="${GRIMES_CATEGORIES:-SEC,COR,REL,OPS,VER}"
IFS=, read -r -a ROUTED <<<"$CATEGORIES"
for CATEGORY in "${ROUTED[@]}"; do
    grimes-contract report stop --category="$CATEGORY" \
        --condition=marginal-yield --probes=2 >/dev/null
done

# One acquittal carrying the control that showed its probe can fail.
"$(cd "$(dirname "$0")" && pwd)/acquit.sh" "${ROUTED[0]}"

grimes-contract report seal \
    --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" \
    --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" \
    --iteration="${GRIMES_ITERATION:-1}" \
    --routed="$CATEGORIES" \
    --examined="$1" --disproved="$2" --summary="$3"
