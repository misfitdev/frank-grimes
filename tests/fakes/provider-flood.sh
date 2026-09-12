#!/usr/bin/env bash
# Unbounded output. The engine must stop reading rather than buffer it all.
set -euo pipefail
exec yes "0123456789abcdef0123456789abcdef"
