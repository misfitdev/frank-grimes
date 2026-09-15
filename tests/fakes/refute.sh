#!/usr/bin/env bash
# A refutation pass with a fixed outcome for every claim it is handed.
#
# Fails loudly if it was told anything that argues for a claim. A refuter shown
# the severity, the reporter, or the evidence narrative is checking the case for
# a claim rather than the claim, which is the hole the pass exists to close.
#
# The engine plants a control claim that denies a name the target does carry.
# This fake breaks it the way an honest refuter would, by looking: it searches
# for the name and exhibits what came back. Pass `rubberstamp` to get the
# failure mode where the fixed outcome is applied to the control too, or
# `fabricate` to get the one where the control is recognised and answered with a
# well-formed disproof that rests on nothing.
#
# Usage: refute.sh refuted|upheld|unavailable [rubberstamp|fabricate]
set -euo pipefail

OUTCOME="$1"
STAMP="${2:-}"

for leak in GRIMES_FINDING GRIMES_EVIDENCE GRIMES_LEDGER GRIMES_SEVERITY GRIMES_SUMMARY GRIMES_CLAIMED GRIMES_TARGET_INVENTORY GRIMES_CATEGORIES; do
    if [[ -n "${!leak+x}" ]]; then
        echo "refuter was told ${leak}" >&2
        exit 1
    fi
done

if [[ "${GRIMES_ROLE:-}" != "refuter" ]]; then
    echo "refuter invoked as role ${GRIMES_ROLE:-none}" >&2
    exit 1
fi

# The name a claim denies, when it denies one. Everything else is a real claim.
denied_name() {
    sed -n 's/^nothing at this anchor mentions \(.*\), so no path through it can depend on that name$/\1/p' <<<"$1"
}

while IFS= read -r -d '' ref &&
    IFS= read -r -d '' _category &&
    IFS= read -r -d '' _anchor &&
    IFS= read -r -d '' claim; do

    term="$(denied_name "$claim")"

    if [[ -n "$term" && "$STAMP" == "fabricate" ]]; then
        grimes-contract refute add --ref="$ref" --refuted \
            --action="grep -rF <name> ." --cwd="." --exit-code=1 \
            --output="that name is carried nowhere at this anchor"
        continue
    fi

    if [[ -n "$term" && "$STAMP" != "rubberstamp" ]]; then
        hit="$(grep -rhF --exclude-dir=.grimes -- "$term" . 2>/dev/null || true)"
        if [[ -n "$hit" ]]; then
            grimes-contract refute add --ref="$ref" --refuted \
                --action="grep -rF <name> ." --cwd="." --exit-code=0 \
                --output="$hit"
            continue
        fi
    fi

    case "$OUTCOME" in
        unavailable)
            grimes-contract refute add --ref="$ref" \
                --unavailable="no runtime available to exercise this claim"
            ;;
        refuted)
            grimes-contract refute add --ref="$ref" --refuted \
                --action="sh -c false" --cwd="." --exit-code=1 \
                --output="the claim does not hold against the artifact"
            ;;
        *)
            grimes-contract refute add --ref="$ref" --upheld \
                --action="sh -c true" --cwd="." --exit-code=0 \
                --output="attacked the claim directly"
            ;;
    esac
done < <(grimes-contract refute claims --print0)

grimes-contract refute seal
