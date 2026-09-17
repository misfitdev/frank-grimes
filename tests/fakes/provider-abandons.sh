#!/usr/bin/env bash
# Adds a finding and dies without sealing.
#
# What it leaves behind is a report the engine never received: the pass that
# opened it is gone, and the candidates in it were never delivered.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

"$FAKES/add-finding.sh" P0 systemic likely "a claim nobody sealed"
echo "this pass is not coming back" >&2
exit 1
