#!/usr/bin/env bash
# Emits an AdjudicationReport over the target it was given.
#
# The fingerprint has to be the one the engine passed: a tuple over a different
# artifact is not a second opinion on this one, and the engine refuses it.
set -euo pipefail

TUPLE_DECISION="$1"
TUPLE_RISK="$2"
TUPLE_CONFIDENCE="$3"
TUPLE_COMPLETENESS="$4"

# Hex fingerprint to a textproto bytes literal. Parameter expansion cannot
# insert a prefix every two characters, so sed it is.
# shellcheck disable=SC2001
ESCAPED="$(echo "${GRIMES_TARGET_FINGERPRINT}" | sed 's/../\\x&/g')"

grimes-contract encode-report --type=AdjudicationReport /dev/stdin <<TEXTPROTO
schema_major: 2
run_id: "adj-${RANDOM:-0}"
reviewer_id: "fake-adjudicator"
target_fingerprint_sha256: "${ESCAPED}"
verdict {
  decision: ${TUPLE_DECISION}
  residual_risk: ${TUPLE_RISK}
  review_confidence: ${TUPLE_CONFIDENCE}
  review_completeness: ${TUPLE_COMPLETENESS}
}
completed_at { seconds: 1780000000 }
TEXTPROTO
