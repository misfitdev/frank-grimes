#!/usr/bin/env bash
# Raises a finding for the very claim every sealed report acquits.
#
# The transition the ledger has to keep: the review said it had tested this
# claim and cleared it, and a later pass found a defect for the same claim in
# the same place. Both halves matter, and the acquittal is what makes the
# finding interesting.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I looked again at something I had cleared."
# The same category, anchor and claim acquit.sh uses, so the two address the
# same content.
"$FAKES/add-finding.sh" P1 local_component likely "the target rejects an unsigned request"
"$FAKES/seal.sh" 4 3 "A claim cleared earlier turns out to be a defect."
