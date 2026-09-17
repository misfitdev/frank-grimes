#!/usr/bin/env bash
# Answers for a different run, over this run's target.
#
# The bytes match, so the fingerprint check admits it. What makes it not a
# second opinion on this review is that it was not reached for it: adjudication
# is what a pass turns on, and the engine stamps its own run onto the review it
# returns, so an answer from elsewhere would leave nothing to show it.
set -euo pipefail

# shellcheck disable=SC2001 # a prefix every two characters is not a parameter expansion
ESCAPED="$(echo "${GRIMES_TARGET_FINGERPRINT}" | sed 's/../\\x&/g')"

grimes-contract encode-report --type=AdjudicationReport /dev/stdin <<TEXTPROTO
schema_major: 2
run_id: "some-earlier-run"
reviewer_id: "fake-adjudicator"
target_fingerprint_sha256: "${ESCAPED}"
verdict {
  decision: DECISION_PASS
  residual_risk: RESIDUAL_RISK_LOW
  review_confidence: REVIEW_CONFIDENCE_HIGH
  review_completeness: REVIEW_COMPLETENESS_SUFFICIENT
}
completed_at { seconds: 1780000000 }
TEXTPROTO
