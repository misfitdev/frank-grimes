#!/usr/bin/env bash
# A refutation pass wired the way the adapters document it: claims come out of
# `refute claims`, each answer goes back through `refute add`, and `refute seal`
# emits the report. Nothing here parses prototext or escapes a fingerprint.
#
# The engine plants a control claim about an identifier that is not in the
# target. This fake breaks it the way an honest refuter would, by looking.
#
# Usage: refuter-documented.sh refuted|upheld|unavailable
set -euo pipefail

OUTCOME="$1"

if [[ "${GRIMES_ROLE:-}" != "refuter" ]]; then
    echo "refuter invoked as role ${GRIMES_ROLE:-none}" >&2
    exit 1
fi

case "${OUTCOME}" in
    probe)
        # Refusals a refuter has to be able to act on. The engine buffers this
        # child's stderr, so they are captured where a test can read them.
        : >./refute.err
        grimes-contract refute add --ref="not-a-ref" --upheld \
            --action="sh -c true" --cwd="." --exit-code=0 --output="x" 2>>./refute.err || true
        first="$(grimes-contract refute claims | head -1 | cut -f1)"
        grimes-contract refute add --ref="$first" --upheld \
            --action="sh -c true" --cwd="." --exit-code=0 --output="x"
        grimes-contract refute add --ref="$first" --upheld \
            --action="sh -c true" --cwd="." --exit-code=0 --output="x" 2>>./refute.err || true
        # The answers above stand in for a pass that died before sealing. The loop
        # below answers every claim again, which only succeeds if they were dropped.
        OUTCOME=upheld
        ;;
esac

while IFS= read -r -d '' ref &&
    IFS= read -r -d '' _category &&
    IFS= read -r -d '' _anchor &&
    IFS= read -r -d '' claim; do

    verdict="$OUTCOME"
    token="$(grep -oE 'fgq[0-9a-f]{13}' <<<"$claim" || true)"
    if [[ -n "$token" ]] && ! grep -rqF "$token" --exclude-dir=.grimes . 2>/dev/null; then
        verdict=refuted
    fi

    case "$verdict" in
        unavailable)
            grimes-contract refute add --ref="$ref" \
                --unavailable="no runtime available to exercise this claim"
            ;;
        refuted)
            grimes-contract refute add --ref="$ref" --refuted \
                --action="grep -rF <identifier> ." --cwd="." --exit-code=1 \
                --output="no such identifier in the target"
            ;;
        *)
            grimes-contract refute add --ref="$ref" --upheld \
                --action="sh -c true" --cwd="." --exit-code=0 \
                --output="attacked the claim directly"
            ;;
    esac
done < <(grimes-contract refute claims --print0)

grimes-contract refute seal
