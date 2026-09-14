#!/usr/bin/env bash
# Adds one cited finding, anchored in the kind of target under review.
set -euo pipefail

SEVERITY="$1"
BLAST="$2"
LIKELIHOOD="$3"
CLAIM="$4"
QUOTE="$5"

ANCHOR=()
while IFS= read -r -d '' flag; do
    ANCHOR+=("$flag")
done < <("$(cd "$(dirname "$0")" && pwd)/anchor-flags.sh")

grimes-contract report add \
    --category=SEC --severity="$SEVERITY" --blast="$BLAST" \
    --likelihood="$LIKELIHOOD" "${ANCHOR[@]}" \
    --tier=E2 --claim="$CLAIM" --quote="$QUOTE" >/dev/null
