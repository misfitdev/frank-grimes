#!/usr/bin/env bash
# Reports one cited P2.
set -euo pipefail
SEAL="$(cd "$(dirname "$0")" && pwd)/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
# shellcheck disable=SC2016  # the quoted text is evidence, not an expansion
grimes-contract report add --category=SEC --severity=P2 --blast=local_component \
    --likelihood=unlikely --path=bad-script.sh --tier=E2 \
    --claim="caller-controlled deletion path" --quote='rm -rf "$1"/*' >/dev/null
"$SEAL" 4 3 "One deletion path looks reachable only from a trusted caller."
