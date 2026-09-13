#!/usr/bin/env bash
# Adds one cited finding, anchored in the kind of target under review.
#
# A finding lives in the artifact being reviewed: a code target anchors at a
# path, a document at a section, an argument at a step, an external source at
# its URI. The ledger refuses a mismatch, so a provider picks the anchor from
# the kind the engine told it.
set -euo pipefail

SEVERITY="$1"
BLAST="$2"
LIKELIHOOD="$3"
CLAIM="$4"
QUOTE="$5"

case "${GRIMES_TARGET_KIND:-code}" in
    document) ANCHOR=(--document="${GRIMES_TARGET_SCOPE}" --section="1") ;;
    idea) ANCHOR=(--argument="${GRIMES_TARGET_SCOPE}" --step=1) ;;
    external)
        ANCHOR=(--source="${GRIMES_TARGET_SCOPE}" --publisher="Example"
            --snapshot-sha256="$(printf 'a%.0s' $(seq 1 64))"
            --retrieved-at="2026-01-02T15:04:05Z")
        ;;
    *) ANCHOR=(--path=bad-script.sh) ;;
esac

grimes-contract report add \
    --category=SEC --severity="$SEVERITY" --blast="$BLAST" \
    --likelihood="$LIKELIHOOD" "${ANCHOR[@]}" \
    --tier=E2 --claim="$CLAIM" --quote="$QUOTE" >/dev/null
