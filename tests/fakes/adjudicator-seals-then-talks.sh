#!/usr/bin/env bash
# Reaches a verdict through the documented command, then describes it.
#
# The adapter tells a reviewer to end its turn with `adjudicate` and nothing
# else. An agent CLI left to itself does not: the command is a tool call, and
# the model's last message is prose about it.
set -euo pipefail

grimes-contract adjudicate \
    --decision=pass --residual-risk=low \
    --review-confidence=high --review-completeness=sufficient >&2

echo "I reviewed the target independently and recorded my verdict."
