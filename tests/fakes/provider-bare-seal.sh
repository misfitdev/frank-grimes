#!/usr/bin/env bash
# Seals the working report directly, without writing anything to it first.
#
# The only path that reaches the seal's own refusal: every other command writes
# the report and is refused before it gets there.
set -euo pipefail

echo "I found nothing of my own."
grimes-contract report seal \
    --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" \
    --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" \
    --iteration="${GRIMES_ITERATION:-1}" \
    --mode="${GRIMES_MODE:-report}" \
    --routed="${GRIMES_CATEGORIES:-SEC}" \
    --examined=1 --disproved=0 --summary="Sealing what I found here."
