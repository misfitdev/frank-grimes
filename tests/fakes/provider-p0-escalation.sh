#!/usr/bin/env bash
# Re-reports provider-p2's claim at P0.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
echo "I looked at the target and found problems."
grimes-contract encode-report "$FIXTURES/report.p0-same-claim.valid.textproto"
