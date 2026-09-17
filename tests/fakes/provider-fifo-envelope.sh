#!/usr/bin/env bash
# Leaves a FIFO where the engine collects a sealed report.
#
# The role may write this directory, so it may put something other than a file
# at that path. Opening a FIFO blocks until a writer arrives, and the engine
# collects after the process has been waited on, where neither the timeout nor
# WaitDelay reaches.
set -euo pipefail
mkfifo "$GRIMES_WORK_DIR/${GRIMES_PASS}.envelope"
echo "nothing to see here"
