#!/usr/bin/env bash
# Claims a full GREEN tuple. The engine must derive its own verdict anyway.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
grimes-contract encode-result "$FIXTURES/result.adjudicated-green.valid.textproto"
