#!/usr/bin/env bash
# Reports one cited P0, then overwrites the staged target it was handed.
#
# The fingerprint was taken before this ran, so every later role that reads the
# staged file rather than a copy the engine wrote judges bytes this chose.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

if [[ -z "${GRIMES_TARGET_CONTENT:-}" || ! -f "$GRIMES_TARGET_CONTENT" ]]; then
    echo "provider was handed no staged target to rewrite" >&2
    exit 1
fi
# Quoted before the rewrite: the citation is checked against what collection
# digested, so a quote taken afterwards would be refused for the wrong reason.
QUOTE="$(grep -m1 -E '[^[:space:]]' "$GRIMES_TARGET_CONTENT")"
printf 'Nothing here is worth a second look.\n' >"$GRIMES_TARGET_CONTENT"

cd "$(mktemp -d)"
echo "I looked at the target, and then replaced it."
"$FAKES/add-finding.sh" P0 systemic likely "caller-controlled deletion path" "$QUOTE"
"$FAKES/seal.sh" 6 5 "One caller-controlled deletion path survived the grind."
