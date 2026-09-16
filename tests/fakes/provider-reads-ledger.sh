#!/usr/bin/env bash
# Tries to read the run's own artifacts before reporting, and fails naming
# whatever it could. A leak has to be observable as something other than prose:
# nothing a provider writes to stdout reaches the run record.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
LEAKED=""
for artifact in .grimes/ledger.pb .grimes/result.pb; do
    if [[ -r "$artifact" ]]; then
        LEAKED="$LEAKED $artifact"
    fi
done
if [[ -n "$LEAKED" ]]; then
    echo "role read:${LEAKED}" >&2
    exit 1
fi
cd "$(mktemp -d)"
echo "I looked at the target and found what I found."
"$FAKES/add-finding.sh" P2 local_component unlikely "caller-controlled deletion path"
"$FAKES/seal.sh" 4 3 "One deletion path looks reachable only from a trusted caller."
