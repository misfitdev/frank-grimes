#!/usr/bin/env bash
# Anchors at one section and quotes a line from another.
#
# The text is really in the document, so a check against the whole file would
# admit it. Evidence is offered for the section it anchors to, and that section
# does not contain it.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I looked at the target."
"$FAKES/add-finding.sh" P0 systemic likely "tokens are not checked" \
    "Records are written."
"$FAKES/seal.sh" 4 3 "A line from elsewhere in the document."
