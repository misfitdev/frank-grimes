#!/usr/bin/env bash
# Re-reports the same claim at P3. A re-report must not defuse a finding.
set -euo pipefail
SEAL="$(cd "$(dirname "$0")" && pwd)/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
# shellcheck disable=SC2016  # the quoted text is evidence, not an expansion
grimes-contract report add --category=SEC --severity=P3 --blast=local_component \
    --likelihood=unlikely --path=bad-script.sh --tier=E2 \
    --claim="caller-controlled deletion path" --quote='rm -rf "$1"/*' >/dev/null
"$SEAL" 5 4 "On another look the deletion path seems cosmetic."
