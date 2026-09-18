#!/usr/bin/env bash
# Reports a finding in each of two files and repairs both.
#
# The batch a mixed refutation needs: one repair that earns its credit and one
# that does not, in a single set of edits.
set -euo pipefail
FAKES="$(cd "$(dirname "$0")" && pwd)"
ROOT="${GRIMES_TARGET_ROOT:-.}"

for unit in src/app.sh src/other.sh; do
    QUOTE="$(grep -m1 -E '[^[:space:]]' "$ROOT/$unit")"
    grimes-contract report add \
        --category=SEC --severity=P2 --blast=local_component --likelihood=unlikely \
        --path="$unit" --tier=E2 --claim="caller-controlled deletion path in $unit" \
        --quote="$QUOTE" \
        --disproof-action="tried to show the path is unreachable" \
        --disproof-exit=1 --disproof-output="still reachable" >/dev/null
done

"$FAKES/seal.sh" 4 3 "Two deletion paths, repaired."

# shellcheck disable=SC2016 # the text is the repaired target, not an expansion
printf 'rm -rf -- "${BUILD:?}"/*\n' >"$ROOT/src/app.sh"
printf 'echo repaired\n' >"$ROOT/src/other.sh"
