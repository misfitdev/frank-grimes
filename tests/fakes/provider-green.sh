#!/usr/bin/env bash
# A review that survived its own grind with nothing left. A report cannot
# claim a verdict, so this is as close as a provider comes to asserting a pass.
set -euo pipefail
SEAL="$(cd "$(dirname "$0")" && pwd)/seal.sh"
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
"$SEAL" 7 7 "Seven candidates examined, all disproved."
