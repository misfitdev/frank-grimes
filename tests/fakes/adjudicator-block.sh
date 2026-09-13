#!/usr/bin/env bash
# An independent reviewer that disagrees; its block must stand.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
"$HERE/adjudicate.sh" DECISION_BLOCK RESIDUAL_RISK_CRITICAL REVIEW_CONFIDENCE_HIGH REVIEW_COMPLETENESS_LIMITED
