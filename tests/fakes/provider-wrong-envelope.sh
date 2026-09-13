#!/usr/bin/env bash
# Sends a result envelope where a report belongs. The marker is what catches it.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
grimes-contract encode-result "$FIXTURES/result.report-complete.valid.textproto"
