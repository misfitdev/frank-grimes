#!/usr/bin/env bash
# Backgrounds a sleeper and waits, holding the stdout pipe open.
set -euo pipefail
sleep 300 &
echo "working"
wait
