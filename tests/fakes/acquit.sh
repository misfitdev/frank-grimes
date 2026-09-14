#!/usr/bin/env bash
# Records one acquittal carrying the control that showed its probe can fail.
#
# Without a controlled acquittal (or an E1 finding) a review has run nothing
# known to be capable of failing, and the engine holds completeness at
# inconclusive however much of the target it accounted for.
set -euo pipefail

CATEGORY="${1:-SEC}"

ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$(cd "$(dirname "$0")" && pwd)/anchor-flags.sh")

grimes-contract report acquit --category="$CATEGORY" "${ANCHOR[@]}" \
    --claim="the target rejects an unsigned request" \
    --scope="the request path, not the token store" \
    --probe-action="run the suite" --probe-exit=0 --probe-output="all assertions held" \
    --control-mutation="drop the signature check" \
    --control-action="run the suite" --control-exit=1 \
    --control-output="the unsigned request was accepted" >/dev/null
