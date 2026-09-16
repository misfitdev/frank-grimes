#!/usr/bin/env bash
# Reports one cited P2 and then fixes it, in the worktree it was handed.
#
# The edit is the whole point: a fixing role is the one context allowed to
# change the bytes under review, and only inside the copy the engine made.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$FAKES/anchor-flags.sh")
UNIT="${ANCHOR[0]#--path=}"
TARGET="${GRIMES_TARGET_ROOT:-.}/$UNIT"

QUOTE="$(grep -m1 -E '[^[:space:]]' "$TARGET")"

echo "I looked at the target and then repaired it."
"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path" "$QUOTE"
"$FAKES/seal.sh" 4 3 "One deletion path, repaired."

# After sealing: the report describes the target as it was reviewed.
# shellcheck disable=SC2016  # the text is the repaired target, not an expansion
printf 'rm -rf -- "${BUILD:?}"/*\n' >"$TARGET"
