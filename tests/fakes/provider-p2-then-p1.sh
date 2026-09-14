#!/usr/bin/env bash
# Reports one claim twice in a single report, at P2 and then P1.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
SEAL="$FAKES/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path"
"$FAKES/add-finding.sh" P1 service likely "caller-controlled deletion path"
"$SEAL" 4 2 "The deletion path looked minor, then worse."
