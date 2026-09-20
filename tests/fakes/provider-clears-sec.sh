#!/usr/bin/env bash
# Clears the claim that provider-overturns.sh later raises a finding for.
#
# The category is stated rather than taken from the routed set, so the two
# address the same content: an acquittal and a finding are the same claim only
# if the category, the anchor and the claim all agree.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I tested this and it held."
"$FAKES/acquit.sh" SEC
"$FAKES/seal.sh" 4 3 "The claim held under a probe shown capable of failing."
