#!/usr/bin/env bash
# Runs the documented refutation, then describes it instead of emitting it.
#
# The same shape an agent CLI takes when left to itself: the seal is a tool
# call, so its envelope lands in that CLI's transcript, and what reaches the
# engine is the model's last message about it.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"

"$FAKES/refuter-documented.sh" upheld >&2

echo "I attacked every claim I was given and sealed the outcome."
