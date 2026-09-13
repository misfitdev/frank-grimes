#!/usr/bin/env bash
# Seals whatever candidates the caller added, the way the adapter documents.
#
# The engine owns the run's identity, the target's, and the iteration. A report
# answers one request, so a provider echoes what it was given rather than
# minting its own; every fake goes through this for that reason.
set -euo pipefail

grimes-contract report seal \
    --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" \
    --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" \
    --iteration="${GRIMES_ITERATION:-1}" \
    --routed=SEC,COR,REL,OPS,VER \
    --examined="$1" --disproved="$2" --summary="$3"
