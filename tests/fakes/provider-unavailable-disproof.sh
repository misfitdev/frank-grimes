#!/usr/bin/env bash
# Reports a P0 whose disproof could not be attempted, and says why.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I tried to argue with myself and could not."
DISPROOF_UNAVAILABLE="the service is not reachable from here" \
    "$FAKES/add-finding.sh" P0 systemic likely "caller-controlled deletion path"
"$FAKES/seal.sh" 4 3 "A P0 I could not test."
