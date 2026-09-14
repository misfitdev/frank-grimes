#!/usr/bin/env bash
# Prints the anchor flags for the kind of target under review.
#
# A record lives in the artifact being reviewed: a code target anchors at a
# path, a document at a section, an argument at a step, an external source at
# its URI. The engine refuses an anchor that names nothing in the target, so the
# anchor is taken from the inventory rather than guessed from the scope: the
# scope of a code target can be a directory, and a directory is not one of its
# units. NUL-separated, since a unit id is arbitrary text.
set -euo pipefail

first_unit() {
    if [[ -n "${GRIMES_TARGET_INVENTORY:-}" && -f "${GRIMES_TARGET_INVENTORY}" ]]; then
        local first
        IFS= read -r -d '' first < <(grimes-contract report units --print0) || true
        printf '%s' "$first"
    fi
}

UNIT="${1:-$(first_unit)}"

case "${GRIMES_TARGET_KIND:-code}" in
    document)
        ANCHOR=(--document="${GRIMES_TARGET_SCOPE}" --section="${UNIT:-1}")
        ;;
    idea)
        ANCHOR=(--argument="${GRIMES_TARGET_SCOPE}" --step="${UNIT#step-}")
        ;;
    external)
        ANCHOR=(--source="${UNIT:-${GRIMES_TARGET_SCOPE}}" --publisher="Example"
            --snapshot-sha256="$(printf 'a%.0s' $(seq 1 64))"
            --retrieved-at="2026-01-02T15:04:05Z")
        ;;
    *)
        ANCHOR=(--path="${UNIT:-${GRIMES_TARGET_SCOPE:-src}}")
        ;;
esac

printf '%s\0' "${ANCHOR[@]}"
