#!/usr/bin/env bash
# Reports a P0 and records no attempt to disprove it.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I looked at the target and did not argue with myself."
DISPROOF_NONE=1 "$FAKES/add-finding.sh" P0 systemic likely "caller-controlled deletion path"
"$FAKES/seal.sh" 4 3 "A P0 nobody attacked."
