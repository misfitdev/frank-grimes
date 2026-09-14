#!/usr/bin/env bash
# Adds one cited finding, anchored in the kind of target under review.
#
# The quote is taken from the artifact rather than supplied: the engine checks a
# citation against the target, so invented text is refused. A caller may still
# pass one to exercise that refusal.
set -euo pipefail

SEVERITY="$1"
BLAST="$2"
LIKELIHOOD="$3"
CLAIM="$4"
QUOTE="${5:-}"

FAKES="$(cd "$(dirname "$0")" && pwd)"

ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$FAKES/anchor-flags.sh")

# The first line that carries something, from whatever the anchor names. A code
# unit id is relative to the repository root; every other kind has its content
# in one file.
if [[ -z "$QUOTE" ]]; then
    SOURCE="${GRIMES_TARGET_CONTENT:-}"
    if [[ "${GRIMES_TARGET_KIND:-code}" == "code" ]]; then
        SOURCE="${GRIMES_TARGET_ROOT:-.}/${ANCHOR[0]#--path=}"
    fi
    QUOTE="$(grep -m1 -E '[^[:space:]]' "$SOURCE" 2>/dev/null || true)"
fi

# The disproof this finding survived. A finding nobody attacked carries no
# verdict weight, so a fake reporting a severity has to record the attack that
# earned it. DISPROOF=unavailable models the reviewer who could not try.
DISPROOF=("--disproof-action=tried to show the path is unreachable"
    "--disproof-exit=1" "--disproof-output=still reachable")
if [[ "${DISPROOF_UNAVAILABLE:-}" != "" ]]; then
    DISPROOF=("--disproof-unavailable=$DISPROOF_UNAVAILABLE")
elif [[ "${DISPROOF_NONE:-}" != "" ]]; then
    DISPROOF=()
fi

grimes-contract report add \
    --category=SEC --severity="$SEVERITY" --blast="$BLAST" \
    --likelihood="$LIKELIHOOD" "${ANCHOR[@]}" \
    --tier=E2 --claim="$CLAIM" --quote="$QUOTE" "${DISPROOF[@]}" >/dev/null
