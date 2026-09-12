#!/usr/bin/env bash
# Emits the committed RED fixture as a result envelope.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
echo "I looked at the target and found problems."
grimes-contract encode-result "$FIXTURES/result.report-complete.valid.textproto"
