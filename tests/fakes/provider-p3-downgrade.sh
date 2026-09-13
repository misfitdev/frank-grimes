#!/usr/bin/env bash
# Re-reports the same claim at P3.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
echo "I looked at the target and found problems."
grimes-contract encode-report "$FIXTURES/report.p3-same-claim.valid.textproto"
