#!/usr/bin/env bash
# Reports a finding anchored at a path that is not part of the target.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
cd "$(mktemp -d)"
echo "I looked somewhere else."
grimes-contract report add --category=SEC --severity=P0 --blast=systemic \
    --likelihood=likely --path=not-in-this-target.sh --tier=E2 \
    --claim="caller-controlled deletion path" --quote="echo one" >/dev/null
"$FAKES/seal.sh" 4 3 "A finding about an artifact nobody reviewed."
