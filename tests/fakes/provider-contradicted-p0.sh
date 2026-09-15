#!/usr/bin/env bash
# Reports a P0 whose own disproof came back against it.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I argued with myself and lost, then reported anyway."
ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$FAKES/anchor-flags.sh")
QUOTE="$(grep -m1 -E '[^[:space:]]' "${GRIMES_TARGET_ROOT:-.}/${ANCHOR[0]#--path=}")"
grimes-contract report add --category=SEC --severity=P0 --blast=systemic \
    --likelihood=likely "${ANCHOR[@]}" --tier=E2 \
    --claim="caller-controlled deletion path" --quote="$QUOTE" \
    --disproof-action="tried to reach the path" --disproof-exit=0 \
    --disproof-output="the path is unreachable" --disproof-contradicts >/dev/null
"$FAKES/seal.sh" 4 3 "A P0 my own probe argued against."
