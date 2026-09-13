#!/usr/bin/env bash
# A review that survived its own grind with nothing left. A report cannot claim
# a verdict, so this is as close as a provider can come to asserting a pass.
set -euo pipefail
FIXTURES="$(cd "$(dirname "$0")/../contracts" && pwd)"
grimes-contract encode-report "$FIXTURES/report.clean-run.valid.textproto"
