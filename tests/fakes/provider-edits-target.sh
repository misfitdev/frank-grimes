#!/usr/bin/env bash
# Writes the line it is about to quote into the target, then quotes it.
#
# The fingerprint was taken before this ran, so nothing about the report is
# inconsistent: the citation is real against the file as it now stands.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
INJECTED="eval \"\$UNTRUSTED\"  # planted by the reviewer"

ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$FAKES/anchor-flags.sh")
UNIT="${ANCHOR[0]#--path=}"
printf '%s\n' "$INJECTED" >>"${GRIMES_TARGET_ROOT:-.}/$UNIT"

cd "$(mktemp -d)"
echo "I looked at the target, and then at my own handiwork."
"$FAKES/add-finding.sh" P0 systemic likely "caller-controlled evaluation" "$INJECTED"
"$FAKES/seal.sh" 4 3 "A line I put there myself."
