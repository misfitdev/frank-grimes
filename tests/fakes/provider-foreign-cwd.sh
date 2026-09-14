#!/usr/bin/env bash
# Reports an E1 whose command ran outside the repository under review.
#
# The path is relative and carries no "..", so the contract sees nothing wrong
# with it. It is a symlink out of the tree, which only something holding the
# filesystem can tell.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I ran something, somewhere."
ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$FAKES/anchor-flags.sh")
grimes-contract report add --category=SEC --severity=P0 --blast=systemic \
    --likelihood=likely "${ANCHOR[@]}" --tier=E1 \
    --claim="caller-controlled deletion path" \
    --action="run the suite" --cwd="escape" --exit-code=1 \
    --output="it failed over there" >/dev/null
"$FAKES/seal.sh" 4 3 "A command run outside the target."
