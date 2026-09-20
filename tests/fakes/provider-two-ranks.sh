#!/usr/bin/env bash
# Reports two findings whose order in the record is not their order here.
#
# The first is reachable but small, the second is systemic but improbable. A
# single severity letter cannot separate them; a matrix over probability and
# impact puts the second first.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
SEAL="$FAKES/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
"$FAKES/add-finding.sh" P1 single_user likely "caller-controlled deletion path"
"$FAKES/add-finding.sh" P1 systemic unlikely "eval of untrusted input"
"$SEAL" 4 2 "Two findings of the same severity and different weight."
