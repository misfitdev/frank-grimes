#!/usr/bin/env bash
# A reviewer that does not answer.
#
# Not a run failure: absence of a second opinion is not agreement, and the
# engine counts it as one of the panel that never arrived.
set -euo pipefail
echo "this reviewer could not be reached" >&2
exit 1
