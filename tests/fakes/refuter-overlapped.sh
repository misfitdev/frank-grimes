#!/usr/bin/env bash
# A refuter that is still mid-pass while a second run over the same directory
# starts and finishes one. It answers its claims, announces that it is holding,
# waits for the sentinel, then re-reads the claims it was given.
#
# If the second run's task landed on top of its own, the refs it answered are no
# longer the refs it was issued and the control it broke is not the control it
# will be graded on, so it exits rather than sealing a report about someone
# else's run.
#
# Usage: refuter-overlapped.sh <holding marker> <sentinel>
set -euo pipefail

MARKER="$1"
SENTINEL="$2"

before="$(cat "$GRIMES_CLAIMS")"

while IFS= read -r -d '' ref &&
    IFS= read -r -d '' _category &&
    IFS= read -r -d '' _anchor &&
    IFS= read -r -d '' claim; do

    token="$(grep -oE 'fgq[0-9a-f]{13}' <<<"$claim" || true)"
    if [[ -n "$token" ]] && ! grep -rqF "$token" --exclude-dir=.grimes . 2>/dev/null; then
        grimes-contract refute add --ref="$ref" --refuted \
            --action="grep -rF <identifier> ." --cwd="." --exit-code=1 \
            --output="no such identifier in target"
    else
        grimes-contract refute add --ref="$ref" --upheld \
            --action="sh -c true" --cwd="." --exit-code=0 \
            --output="attacked the claim directly"
    fi
done < <(grimes-contract refute claims --print0)

: >"$MARKER"
for _ in $(seq 1 200); do
    [[ -e "$SENTINEL" ]] && break
    sleep 0.1
done

if [[ "$(cat "$GRIMES_CLAIMS")" != "$before" ]]; then
    echo "the claims changed underneath this pass" >&2
    exit 1
fi

grimes-contract refute seal
