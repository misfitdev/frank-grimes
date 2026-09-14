#!/usr/bin/env bash
# Prints the anchor flags for the kind of target under review.
#
# A record lives in the artifact being reviewed: a code target anchors at a
# path, a document at a section, an argument at a step, an external source at
# its URI. The ledger refuses a mismatch, so a provider picks the anchor from
# the kind the engine told it. NUL-separated, since a scope is arbitrary text.
set -euo pipefail

case "${GRIMES_TARGET_KIND:-code}" in
    document) ANCHOR=(--document="${GRIMES_TARGET_SCOPE}" --section="1") ;;
    idea) ANCHOR=(--argument="${GRIMES_TARGET_SCOPE}" --step=1) ;;
    external)
        ANCHOR=(--source="${GRIMES_TARGET_SCOPE}" --publisher="Example"
            --snapshot-sha256="$(printf 'a%.0s' $(seq 1 64))"
            --retrieved-at="2026-01-02T15:04:05Z")
        ;;
    *) ANCHOR=(--path="${1:-${GRIMES_TARGET_SCOPE:-src}}") ;;
esac

printf '%s\0' "${ANCHOR[@]}"
