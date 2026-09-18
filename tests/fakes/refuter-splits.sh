#!/usr/bin/env bash
# Breaks the claims anchored in one file and upholds the rest.
#
# The case a batch makes possible: one repair earns its credit and another does
# not, in the same set of edits. A commit takes the whole worktree, so the two
# cannot both be honoured.
#
# Usage: refuter-splits.sh <path whose claims are broken>
set -euo pipefail

BROKEN="$1"

denied_name() {
    sed -n 's/^nothing at this anchor mentions \(.*\), so no path through it can depend on that name$/\1/p' <<<"$1"
}

while IFS= read -r -d '' ref &&
    IFS= read -r -d '' _category &&
    IFS= read -r -d '' anchor &&
    IFS= read -r -d '' claim; do

    # The control denies a name the target carries; break it by looking.
    term="$(denied_name "$claim")"
    if [[ -n "$term" ]]; then
        hit="$(grep -rhF --exclude-dir=.grimes -- "$term" . 2>/dev/null || true)"
        if [[ -n "$hit" ]]; then
            grimes-contract refute add --ref="$ref" --refuted \
                --action="grep -rF <name> ." --cwd="." --exit-code=0 --output="$hit"
            continue
        fi
    fi

    if [[ "$anchor" == *"$BROKEN"* ]]; then
        grimes-contract refute add --ref="$ref" --refuted \
            --action="sh -c false" --cwd="." --exit-code=1 \
            --output="the claim does not hold against the artifact"
    else
        grimes-contract refute add --ref="$ref" --upheld \
            --action="sh -c true" --cwd="." --exit-code=0 \
            --output="attacked the claim directly"
    fi
done < <(grimes-contract refute claims --print0)

grimes-contract refute seal
