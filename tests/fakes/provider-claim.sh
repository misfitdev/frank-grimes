#!/usr/bin/env bash
# Reports one cited P2 per claim, where a claim is the words up to the next
# "--". The engine splits a provider command on spaces, so a claim arrives as
# several arguments rather than one.
#
# Two runs of this with differently worded claims are the same defect described
# twice, which is what a reworded restatement looks like to the engine.
#
# Usage: provider-claim.sh <word>... [-- <word>...]
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

CLAIMS=()
CURRENT=()
for word in "$@"; do
    if [[ "$word" == "--" ]]; then
        CLAIMS+=("${CURRENT[*]}")
        CURRENT=()
        continue
    fi
    CURRENT+=("$word")
done
CLAIMS+=("${CURRENT[*]}")

cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
for claim in "${CLAIMS[@]}"; do
    "$FAKES/add-finding.sh" P2 local_component unlikely "$claim"
done
"$FAKES/seal.sh" 4 3 "One deletion path looks reachable only from a trusted caller."
