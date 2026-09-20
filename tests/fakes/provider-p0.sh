#!/usr/bin/env bash
# Reports one cited P0.
#
# The severity that drives a blocking decision, so a contested claim at this
# weight is where excluding it from the blocking counts is observable.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
SEAL="$FAKES/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
"$FAKES/add-finding.sh" P0 systemic likely "caller-controlled deletion path"
"$SEAL" 4 3 "One deletion path is reachable from untrusted input."
