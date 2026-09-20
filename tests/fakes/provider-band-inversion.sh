#!/usr/bin/env bash
# A P2 that outscores a P0.
#
# Likely and systemic against unlikely and single-user: the product puts the P2
# first, and the skill puts every terminal finding ahead of it. Rank orders work
# within a band; it does not decide which band a finding is in.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I found one of each."
"$FAKES/add-finding.sh" P2 systemic likely "the wrapper is reachable from every caller"
"$FAKES/add-finding.sh" P0 single_user unlikely "one operator can reach the deletion path"
"$FAKES/seal.sh" 4 2 "A high-scoring P2 and a low-scoring P0."
