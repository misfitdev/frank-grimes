#!/usr/bin/env bash
# Breaks every claim the first time it runs and upholds every claim after that.
#
# Two contexts that did not form a claim, reaching opposite answers. Attempts
# accumulate on a finding across passes, so the second run leaves one claim
# carrying both: the state the engine has to report rather than resolve.
#
# The turn is taken from a marker rather than from GRIMES_ITERATION, which is 1
# in every separate invocation -- an iteration only advances through the stop
# hook, and a fake cannot drive that.
set -euo pipefail

MARKER="${GRIMES_WORK_DIR:-.grimes/work}/flips.turn"

denied_name() {
    sed -n 's/^nothing at this anchor mentions \(.*\), so no path through it can depend on that name$/\1/p' <<<"$1"
}

while IFS= read -r -d '' ref &&
    IFS= read -r -d '' _category &&
    IFS= read -r -d '' _anchor &&
    IFS= read -r -d '' claim; do

    # The control is answered on its own terms whichever iteration this is, or
    # the pass is ungraded and nothing it said is recorded.
    term="$(denied_name "$claim")"
    if [[ -n "$term" ]]; then
        hit="$(grep -rhF --exclude-dir=.grimes -- "$term" . 2>/dev/null || true)"
        if [[ -n "$hit" ]]; then
            grimes-contract refute add --ref="$ref" --refuted \
                --action="grep -rF <name> ." --cwd="." --exit-code=0 --output="$hit"
            continue
        fi
    fi

    if [[ ! -f "$MARKER" ]]; then
        grimes-contract refute add --ref="$ref" --refuted \
            --action="sh -c false" --cwd="." --exit-code=1 \
            --output="the claim does not hold against the artifact"
    else
        grimes-contract refute add --ref="$ref" --upheld \
            --action="sh -c true" --cwd="." --exit-code=0 \
            --output="attacked the claim directly"
    fi
done < <(grimes-contract refute claims --print0)

: >"$MARKER"
grimes-contract refute seal
