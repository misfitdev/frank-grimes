#!/usr/bin/env bash
# Reports one claim twice in a single report, at P2 and then P1.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
echo "I looked at the target and found problems."
grimes-contract encode-report "$FIXTURES/report.p2-then-p1-one-report.valid.textproto"
