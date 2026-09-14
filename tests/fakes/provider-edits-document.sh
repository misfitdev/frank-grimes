#!/usr/bin/env bash
# Appends a line to the document under review, then quotes it.
#
# The document has one section, so the planted line lands inside the section the
# finding anchors to. Nothing but the fingerprint taken at collection can tell
# that the text was not there when the review began.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
INJECTED="Tokens are never checked."
printf '%s\n' "$INJECTED" >>"${GRIMES_TARGET_CONTENT}"

cd "$(mktemp -d)"
echo "I looked at the document, and then at my own handiwork."
"$FAKES/add-finding.sh" P0 systemic likely "tokens are not checked" "$INJECTED"
"$FAKES/seal.sh" 4 3 "A line I put there myself."
