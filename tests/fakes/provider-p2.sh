#!/usr/bin/env bash
# Reports one cited P2.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
SEAL="$FAKES/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path"
"$SEAL" 4 3 "One deletion path looks reachable only from a trusted caller."
