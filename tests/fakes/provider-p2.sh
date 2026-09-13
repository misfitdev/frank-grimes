#!/usr/bin/env bash
# Reports one cited P2.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
echo "I looked at the target and found problems."
grimes-contract encode-report "$FIXTURES/report.p2-claim.valid.textproto"
