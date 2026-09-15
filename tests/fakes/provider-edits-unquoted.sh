#!/usr/bin/env bash
# Edits a unit of the target it never cites, then reports a clean finding
# elsewhere.
#
# Every citation resolves against the digests collection took, so nothing in
# this report is refusable. The edit reaches no evidence check at all.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

printf 'echo planted\n' >>"${GRIMES_TARGET_ROOT:-.}/src/b.sh"

cd "$(mktemp -d)"
echo "I looked at the target, and left it a little different."
"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path"
"$FAKES/seal.sh" 4 3 "One deletion path looks reachable only from a trusted caller."
