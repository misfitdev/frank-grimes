#!/usr/bin/env bash
# Seals a real report and then dies.
#
# The report was delivered but never collected, and the pass token is the same
# for a repeated invocation of this run, role and iteration: left behind, it is
# what a retry would collect instead of its own work.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

QUOTE="$(grep -m1 -E '[^[:space:]]' "${GRIMES_TARGET_ROOT:-.}/src/app.sh")"
"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path" "$QUOTE" >&2
"$FAKES/seal.sh" 4 3 "One deletion path." >&2

exit 7
