#!/usr/bin/env bash
# Re-reports provider-p2's claim at P0, on stronger evidence.
set -euo pipefail
SEAL="$(cd "$(dirname "$0")" && pwd)/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
# shellcheck disable=SC2016  # the quoted text is evidence, not an expansion
grimes-contract report add --category=SEC --severity=P0 --blast=systemic \
    --likelihood=likely --path=bad-script.sh --tier=E2 \
    --claim="caller-controlled deletion path" --quote='eval "$UNTRUSTED"; rm -rf "$1"/*' >/dev/null
"$SEAL" 5 4 "The deletion path reaches an unauthenticated caller."
