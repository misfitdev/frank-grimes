#!/usr/bin/env bash
# A refuter that answers every claim twice, vouching for it and then breaking
# it. Without a uniqueness rule the later answer wins, which would let a report
# that contradicts itself manufacture a passed control.
set -euo pipefail

# shellcheck disable=SC2001  # a per-pair substitution is what sed is for here
ESCAPED="$(echo "${GRIMES_TARGET_FINGERPRINT}" | sed 's/../\\x&/g')"
REFS="$(grimes-contract decode --type=RefutationTask "${GRIMES_CLAIMS}" |
    sed -n 's/^[[:space:]]*ref:[[:space:]]*"\(.*\)"[[:space:]]*$/\1/p')"

{
    echo 'schema_major: 2'
    echo "run_id: \"${GRIMES_RUN_ID}\""
    echo 'refuter_id: "fake-refuter"'
    echo "target_fingerprint_sha256: \"${ESCAPED}\""
    for ref in $REFS; do
        for verdict in upheld refuted; do
            echo "outcomes {"
            echo "  ref: \"${ref}\""
            echo "  ${verdict} {"
            echo '    executed_command {'
            echo '      action: "sh -c true"'
            echo '      cwd { value: "." }'
            echo '      exit_code: 0'
            echo '      output_excerpt: "two answers for one claim"'
            echo '    }'
            echo '    completed_at { seconds: 1780000000 }'
            echo '  }'
            echo "}"
        done
    done
    echo 'completed_at { seconds: 1780000000 }'
} | grimes-contract encode-report --type=RefutationReport /dev/stdin
