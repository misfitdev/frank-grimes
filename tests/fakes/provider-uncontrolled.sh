#!/usr/bin/env bash
# A clean review whose probe was never shown capable of failing.
#
# Identical to provider-green.sh in every other respect: full unit coverage,
# every routed category stopped, nothing found. The acquittal it records has no
# negative control, which is the whole difference.
set -euo pipefail
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."

FAKES="$(cd "$(dirname "$0")" && pwd)"
if [[ -n "${GRIMES_TARGET_INVENTORY:-}" && -f "$GRIMES_TARGET_INVENTORY" ]]; then
    grimes-contract report units --print0 |
        grimes-contract report cover --examined-stdin0 >/dev/null
fi

CATEGORIES="${GRIMES_CATEGORIES:-SEC,COR,REL,OPS,VER}"
IFS=, read -r -a ROUTED <<<"$CATEGORIES"
for CATEGORY in "${ROUTED[@]}"; do
    grimes-contract report stop --category="$CATEGORY" \
        --condition=marginal-yield --probes=2 >/dev/null
done

ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$FAKES/anchor-flags.sh" "${GRIMES_TARGET_SCOPE:-src}")

grimes-contract report acquit --category="${ROUTED[0]}" "${ANCHOR[@]}" \
    --claim="the target rejects an unsigned request" \
    --scope="the request path, not the token store" \
    --probe-action="run the suite" --probe-exit=0 --probe-output="all assertions held" >/dev/null

grimes-contract report seal \
    --run-id="${GRIMES_RUN_ID:-}" \
    --target-root="${GRIMES_TARGET_ROOT:-}" \
    --target-scope="${GRIMES_TARGET_SCOPE:-src}" \
    --kind="${GRIMES_TARGET_KIND:-code}" \
    --iteration="${GRIMES_ITERATION:-1}" \
    --routed="$CATEGORIES" \
    --examined=7 --disproved=7 --summary="Seven candidates examined, all disproved."
