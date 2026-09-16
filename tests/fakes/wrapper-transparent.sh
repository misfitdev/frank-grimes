#!/usr/bin/env bash
# An operator-supplied wrapper that confines nothing. It is what the probe has
# to catch: without it, a run under this would look exactly like a confined one.
set -euo pipefail
exec "$@"
