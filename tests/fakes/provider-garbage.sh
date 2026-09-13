#!/usr/bin/env bash
# A well-formed report envelope wrapped around bytes that are not a report.
set -euo pipefail
echo "GRIMES_REPORT_PROTOBUF_V2_BEGIN"
echo "bm90IGEgcHJvdG9idWY="
echo "GRIMES_REPORT_PROTOBUF_V2_END"
