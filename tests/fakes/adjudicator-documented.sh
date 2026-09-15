#!/usr/bin/env bash
# An independent reviewer wired the way the adapters document it: the tuple goes
# back through `grimes-contract adjudicate`, which reads the run's identity from
# the environment the engine exported rather than being told it.
set -euo pipefail

if [[ "${GRIMES_ROLE:-}" != "adjudicator" ]]; then
    echo "adjudicator was not addressed as one" >&2
    exit 1
fi

grimes-contract adjudicate \
    --decision=pass \
    --residual-risk=low \
    --review-confidence=high \
    --review-completeness=sufficient
