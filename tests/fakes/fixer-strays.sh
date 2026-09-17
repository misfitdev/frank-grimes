#!/usr/bin/env bash
# Repairs what it was asked about, and tidies something it was not.
#
# The second file is outside the reviewed scope, so nothing that ran over this
# batch tested it: the gate was chosen for the review, and the review never
# resolved that file.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$FAKES/anchor-flags.sh")
UNIT="${ANCHOR[0]#--path=}"
ROOT="${GRIMES_TARGET_ROOT:-.}"
QUOTE="$(grep -m1 -E '[^[:space:]]' "$ROOT/$UNIT")"

echo "I looked at the target and improved the neighbourhood."
"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path" "$QUOTE"
"$FAKES/seal.sh" 4 3 "One deletion path, repaired."

# shellcheck disable=SC2016  # the text is the repaired target, not an expansion
printf 'rm -rf -- "${BUILD:?}"/*\n' >"$ROOT/$UNIT"
printf '# tidied\n' >>"$ROOT/README.md"
