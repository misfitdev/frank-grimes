#!/usr/bin/env bash
# Reports one cited P0 as a provider report.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
echo "I looked at the target and found problems."
grimes-contract encode-report "$FIXTURES/report.p0-with-citation.valid.textproto"
