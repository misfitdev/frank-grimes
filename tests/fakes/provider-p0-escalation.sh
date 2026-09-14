#!/usr/bin/env bash
# Re-reports provider-p2's claim at P0, on stronger evidence.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
SEAL="$FAKES/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
# The stronger evidence, quoted from the target: an escalation that reused the
# milder report's citation would claim a severity its proof does not carry.
# shellcheck disable=SC2016  # the quoted text is evidence, not an expansion
"$FAKES/add-finding.sh" P0 systemic likely "caller-controlled deletion path" 'eval "$UNTRUSTED"'
"$SEAL" 5 4 "The deletion path reaches an unauthenticated caller."
