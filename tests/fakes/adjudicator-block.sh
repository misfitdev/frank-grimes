#!/usr/bin/env bash
# An independent reviewer that disagrees; its block must stand.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
grimes-contract encode-result "$FIXTURES/result.report-complete.valid.textproto"
