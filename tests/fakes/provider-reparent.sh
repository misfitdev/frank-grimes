#!/usr/bin/env bash
# Leaves a descendant outside the provider's process group, holding stdout.
#
# The group kill and WaitDelay both miss it: the survivor is in another group
# and the blocked call is a read, not cmd.Wait. Every other fake cooperates by
# staying in the group, so this is the only one that exercises that path.
set -euo pipefail

python3 -c 'import os, time; os.setsid(); time.sleep(300)' &
echo "working"
