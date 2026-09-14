#!/usr/bin/env bash
# Reports a finding whose citation quotes text the target does not contain.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I looked at the target."
"$FAKES/add-finding.sh" P0 systemic likely "caller-controlled deletion path" \
    "this line appears in no artifact under review"
"$FAKES/seal.sh" 4 3 "A quotation with nothing behind it."
