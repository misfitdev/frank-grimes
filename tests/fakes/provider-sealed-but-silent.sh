#!/usr/bin/env bash
# Does the review, seals nothing to its own stdout, and describes the result.
#
# What an agent CLI does by default: its working goes to stderr, and its answer
# is a model's last message. A provider that sealed inside its own transcript
# and then summarised it is indistinguishable, from outside, from one that never
# reviewed anything -- unless the engine says what did arrive.
set -euo pipefail
echo "running grimes-contract report add --category=SEC ..." >&2
echo "grimes-contract report seal: wrote 1 finding" >&2
echo "I found one issue, fixed it, and sealed the report."
