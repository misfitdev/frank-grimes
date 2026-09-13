#!/usr/bin/env bash
# Reports one cited P0, anchored in whatever kind of target this is.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
# shellcheck disable=SC2016  # the quoted text is evidence, not an expansion
"$FAKES/add-finding.sh" P0 systemic likely \
    "caller-controlled deletion path" 'rm -rf "$1"/*'
"$FAKES/seal.sh" 6 5 "One caller-controlled deletion path survived the grind."
