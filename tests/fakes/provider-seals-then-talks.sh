#!/usr/bin/env bash
# Seals a real report, then describes it instead of emitting it.
#
# What an agent CLI does unprompted: the seal runs as a tool call, so its output
# lands in that CLI's own transcript, and what reaches the engine is whatever
# the model chose to say last. Delivery cannot depend on that choice.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

QUOTE="$(grep -m1 -E '[^[:space:]]' "${GRIMES_TARGET_ROOT:-.}/src/app.sh")"

"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path" "$QUOTE" >&2
"$FAKES/seal.sh" 4 3 "One deletion path." >&2

echo "I reviewed the target, found one issue, and sealed the report."
