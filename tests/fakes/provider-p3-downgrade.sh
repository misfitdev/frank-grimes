#!/usr/bin/env bash
# Re-reports the same claim at P3. A re-report must not defuse a finding.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
SEAL="$FAKES/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
"$FAKES/add-finding.sh" P3 local_component unlikely "caller-controlled deletion path"
"$SEAL" 5 4 "On another look the deletion path seems cosmetic."
