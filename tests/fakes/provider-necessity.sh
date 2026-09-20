#!/usr/bin/env bash
# Reports a finding whose repair is a deletion.
#
# None of the other ten categories expresses this: maintainability is the cost
# of keeping something, and necessity is whether there was anything to keep.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I looked at whether this earns its keep."
FINDING_CATEGORY=NEC "$FAKES/add-finding.sh" P2 local_component plausible \
    "the wrapper has one call site and the suite passes without it"
"$FAKES/seal.sh" 4 3 "One construct nothing needs."
