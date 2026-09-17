#!/usr/bin/env bash
# Reports one cited P2 without leaving the directory it was spawned in, which
# is where the contract CLI accumulates a report by default.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

echo "I looked at the target and found what I found."
"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path"
"$FAKES/seal.sh" 4 3 "One deletion path looks reachable only from a trusted caller."
