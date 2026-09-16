#!/bin/bash
#
# Contract and ledger tests (audit FG-101).
#
# Black-box: everything here goes through the grimes-contract CLI, the same way
# the hook and adapters must. Nothing reaches into the Go packages directly.
#
# Usage: ./tests/test-contracts.sh
#
# Exit codes:
#   0 - All checks passed
#   1 - One or more checks failed
#   2 - Toolchain unavailable (Go not installed)
#

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FIXTURES="$PROJECT_ROOT/tests/contracts"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASSED=0
FAILED=0

pass() {
    echo -e "${GREEN}PASS${NC}: $*"
    PASSED=$((PASSED + 1))
}

fail() {
    echo -e "${RED}FAIL${NC}: $*"
    FAILED=$((FAILED + 1))
}

# `cond && pass || fail` misreports when pass itself returns non-zero, so the
# assertions go through helpers instead.
assert_eq() {
    if [[ "$1" == "$2" ]]; then pass "$3"; else fail "$3 (got '$1' vs '$2')"; fi
}

assert_ne() {
    if [[ "$1" != "$2" ]]; then pass "$3"; else fail "$3 (both '$1')"; fi
}

if ! command -v go &>/dev/null; then
    echo -e "${YELLOW}SKIP${NC}: go is not installed; contract tests require the toolchain"
    exit 2
fi

BIN="$(mktemp -d)/grimes-contract"
trap 'rm -rf "$(dirname "$BIN")"' EXIT

echo "========================================"
echo "Contract and Ledger Tests"
echo "========================================"
echo ""

echo "--- The codec builds ---"
if (cd "$PROJECT_ROOT" && go build -o "$BIN" ./cmd/grimes-contract) 2>/dev/null; then
    pass "grimes-contract builds"
else
    fail "grimes-contract builds"
    echo "Passed: $PASSED / Failed: $FAILED"
    exit 1
fi

echo ""
echo "--- Identity is content-addressed and stable ---"
# Evidence strings below contain literal $1/$2 shell text, not expansions.
# shellcheck disable=SC2016
ID_A=$("$BIN" id --category=SEC --path=bad-script.sh --evidence='rm -rf "$1"/*' | grep '^id:')
# shellcheck disable=SC2016
ID_B=$("$BIN" id --category=SEC --path=./bad-script.sh --evidence='rm -rf "$1"/*' | grep '^id:')
# shellcheck disable=SC2016
ID_C=$("$BIN" id --category=SEC --path=bad-script.sh --evidence='rm -rf "$2"/*' | grep '^id:')
# shellcheck disable=SC2016
ID_D=$("$BIN" id --category=COR --path=bad-script.sh --evidence='rm -rf "$1"/*' | grep '^id:')

assert_eq "$ID_A" "$ID_B" "path normalization does not change identity"
assert_ne "$ID_A" "$ID_C" "different evidence yields a different identity"
assert_ne "$ID_A" "$ID_D" "category participates in identity"

# A finding can live in a document, an argument, or a retrieved source as well
# as at a path. The anchor kind is part of identity, so two kinds reading the
# same cannot be the same finding.
# shellcheck disable=SC2016
ID_DOC=$("$BIN" id --category=SEC --document=bad-script.sh --section=1 --evidence='rm -rf "$1"/*' | grep '^id:')
# shellcheck disable=SC2016
ID_ARG=$("$BIN" id --category=SEC --argument=bad-script.sh --step=1 --evidence='rm -rf "$1"/*' | grep '^id:')
# shellcheck disable=SC2016
ID_SRC=$("$BIN" id --category=SEC --source=bad-script.sh --evidence='rm -rf "$1"/*' | grep '^id:')

assert_ne "$ID_A" "$ID_DOC" "a document clause is not the same place as a path"
assert_ne "$ID_DOC" "$ID_ARG" "an argument step is not the same place as a document clause"
assert_ne "$ID_ARG" "$ID_SRC" "a retrieved source is not the same place as an argument step"

# shellcheck disable=SC2016
ID_DOC_CASE=$("$BIN" id --category=SEC --document=bad-script.sh --section="  1  " --evidence='rm -rf "$1"/*' | grep '^id:')
assert_eq "$ID_DOC" "$ID_DOC_CASE" "section spacing does not change identity"

# shellcheck disable=SC2016
ID_DOC_S2=$("$BIN" id --category=SEC --document=bad-script.sh --section=2 --evidence='rm -rf "$1"/*' | grep '^id:')
assert_ne "$ID_DOC" "$ID_DOC_S2" "a different clause is a different finding"

# flag.Uint accepts more than the contract's uint32 holds, and the conversion
# would truncate 4294967297 to 1 and fingerprint a different claim.
if "$BIN" id --category=SEC --argument=plan --step=4294967297 --evidence=x 2>/dev/null; then
    fail "a step above uint32 was accepted"
else
    pass "a step above uint32 is refused rather than truncated"
fi

# An anchor is required, and only one of them.
if "$BIN" id --category=SEC --evidence=x 2>/dev/null; then
    fail "an anchorless id was accepted"
else
    pass "an id requires an anchor"
fi
if "$BIN" id --category=SEC --path=a --document=b --section=1 --evidence=x 2>/dev/null; then
    fail "two anchors were accepted at once"
else
    pass "anchors are exclusive"
fi
if [[ "$ID_A" =~ ^id:\ FG-SEC-[0-9a-f]{12}$ ]]; then
    pass "ID matches the schema pattern"
else
    fail "ID does not match the schema pattern: $ID_A"
fi

# Line numbers are excluded on purpose: code moving down a file is not a new
# finding, and re-reporting it as one is how a register fills with duplicates.
EV_FILE="$(mktemp)"
# shellcheck disable=SC2016  # literal evidence text, not an expansion
printf '\n\nrm -rf "$1"/*   \n\n' >"$EV_FILE"
ID_WS=$("$BIN" id --category=SEC --path=bad-script.sh --evidence-file="$EV_FILE" | grep '^id:')
rm -f "$EV_FILE"
assert_eq "$ID_A" "$ID_WS" "surrounding blank lines and trailing space do not change identity"

echo ""
echo "--- Fixtures classify as declared ---"
for fixture in "$FIXTURES"/*.textproto; do
    name="$(basename "$fixture")"
    case "$name" in
        finding.*) msg="Finding" ;;
        result.*) msg="GrimesResult" ;;
        ledger.*) msg="Ledger" ;;
        state.*) msg="LoopState" ;;
        report.*) msg="ProviderReport" ;;
        adjudication.*) msg="AdjudicationReport" ;;
        inventory.*) msg="TargetInventory" ;;
        verification.*) msg="Verification" ;;
        *)
            fail "fixture $name has no recognized type prefix"
            continue
            ;;
    esac

    if "$BIN" validate --type="$msg" --format=textproto "$fixture" &>/dev/null; then
        got="valid"
    else
        got="invalid"
    fi

    if [[ "$name" == *.invalid.* ]]; then
        want="invalid"
    else
        want="valid"
    fi

    assert_eq "$got" "$want" "$name is $want"
done

echo ""
echo "--- Canonical encoding round-trips and rejects tampering ---"
VALID_RESULT="$FIXTURES/result.report-complete.valid.textproto"
BIN_OUT="$(mktemp)"
"$BIN" encode-result --raw "$VALID_RESULT" >"$BIN_OUT"

if "$BIN" validate --type=GrimesResult --format=binary "$BIN_OUT" &>/dev/null; then
    pass "canonical binary re-validates"
else
    fail "canonical binary failed validation"
fi

# Encoding twice must produce identical bytes, or no digest over a result means
# anything and the ledger reference cannot be checked.
BIN_OUT2="$(mktemp)"
"$BIN" encode-result --raw "$VALID_RESULT" >"$BIN_OUT2"
if cmp -s "$BIN_OUT" "$BIN_OUT2"; then
    pass "encoding is deterministic across runs"
else
    fail "encoding is not deterministic"
fi

# Appending a field the schema does not define must be rejected, not ignored.
TAMPERED="$(mktemp)"
cat "$BIN_OUT" >"$TAMPERED"
printf '\xf8\x7f\x01' >>"$TAMPERED"
if "$BIN" validate --type=GrimesResult --format=binary "$TAMPERED" &>/dev/null; then
    fail "unknown trailing field was accepted"
else
    pass "unknown trailing field is rejected"
fi

# The same message re-encoded with a duplicated field decodes to the same value
# but is not the canonical form; round-trip equality is what catches it.
DUPED="$(mktemp)"
cat "$BIN_OUT" "$BIN_OUT" >"$DUPED"
if "$BIN" validate --type=GrimesResult --format=binary "$DUPED" &>/dev/null; then
    fail "non-canonical concatenated encoding was accepted"
else
    pass "non-canonical encoding is rejected"
fi

echo ""
echo "--- A result recorded before provenance existed still reads ---"
# Recorded by an earlier build of schema major 2, which had no field 6 on
# FindingSnapshot. The bytes cannot be produced by any current writer, so the
# fixture is committed rather than generated here.
ARCHIVED="$FIXTURES/result.pre-provenance.bin"

if "$BIN" validate --type=GrimesResult --format=binary "$ARCHIVED" &>/dev/null; then
    pass "an archived result validates"
else
    fail "an archived result no longer validates"
fi

if "$BIN" decode --type=GrimesResult "$ARCHIVED" | grep -qE 'provenance: *FINDING_PROVENANCE_UNATTACKED'; then
    pass "a finding nobody attacked reads as unattacked"
else
    fail "an absent provenance did not read as unattacked"
fi

# Filling the field on read must not let a writer omit it: silence from a
# current producer is a defect, not a record of when it was written.
NO_PROVENANCE="$(mktemp)"
grep -v 'provenance:' "$FIXTURES/result.snapshot-provenance.valid.textproto" >"$NO_PROVENANCE"
if "$BIN" encode-result --raw "$NO_PROVENANCE" &>/dev/null; then
    fail "a result written without provenance was encoded"
else
    pass "a result written without provenance is rejected"
fi
rm -f "$NO_PROVENANCE"

echo ""
echo "--- Report and result envelopes stay distinct ---"
VALID_REPORT="$FIXTURES/report.p0-with-citation.valid.textproto"
REPORT_ENV="$(mktemp)"
"$BIN" encode-report "$VALID_REPORT" >"$REPORT_ENV"

if grep -q 'GRIMES_REPORT_PROTOBUF_V2_BEGIN' "$REPORT_ENV"; then
    pass "a report is wrapped in the report envelope"
else
    fail "a report is not wrapped in the report envelope"
fi

if "$BIN" decode-report "$REPORT_ENV" | grep -qE 'run_id: *"run-001"'; then
    pass "a report envelope round-trips"
else
    fail "a report envelope does not round-trip"
fi

# A payload sent the wrong way must fail on the marker, not deep in validation.
if "$BIN" decode-result "$REPORT_ENV" &>/dev/null; then
    fail "a report envelope was accepted as a result"
else
    pass "a report envelope is not accepted as a result"
fi
rm -f "$REPORT_ENV"

echo ""
echo "--- Envelope extraction ---"
ENVELOPE="$(mktemp)"
{
    echo "Here is some assistant prose."
    echo "An earlier truncated block: GRIMES_RESULT_PROTOBUF_V2_BEGIN"
    echo ""
    "$BIN" encode-result "$VALID_RESULT"
} >"$ENVELOPE"

# prototext's whitespace is intentionally randomized per binary build
# (google.golang.org/protobuf/internal/detrand), so match on content, not
# exact spacing.
if "$BIN" decode-result "$ENVELOPE" | grep -qE 'run_id: *"run-001"'; then
    pass "last complete envelope is extracted from surrounding prose"
else
    fail "envelope extraction failed"
fi

# A stream cut mid-retry leaves a BEGIN after the good block. Scanning back from
# the final marker alone would find that one and report an unclosed envelope.
TRAILING="$(mktemp)"
{
    "$BIN" encode-result "$VALID_RESULT"
    echo "GRIMES_RESULT_PROTOBUF_V2_BEGIN"
    echo "dHJ1bmNhdGVk"
} >"$TRAILING"

if "$BIN" decode-result "$TRAILING" | grep -qE 'run_id: *"run-001"'; then
    pass "trailing truncated block does not shadow the complete envelope"
else
    fail "trailing truncated block shadows the complete envelope"
fi

rm -f "$BIN_OUT" "$BIN_OUT2" "$TAMPERED" "$DUPED" "$ENVELOPE" "$TRAILING"

echo ""
echo "--- A count the contract cannot hold is refused, not wrapped ---"

# The contract carries these counters as uint32 and the exit code as int32. A
# wider value narrowed silently would seal a report recording a number nobody
# supplied, and it would validate, because the wrapped value is in range.
WORK="$(mktemp -d)"
(
    cd "$WORK" || exit 1
    mkdir -p .grimes
    "$BIN" report add --category=SEC --severity=P2 --blast=local_component \
        --likelihood=unlikely --path=a.sh --tier=E2 --claim="c" --quote="q" >/dev/null
) || fail "report add rejected the seed candidate"

for flag in examined disproved; do
    set +e
    OUT="$(cd "$WORK" && "$BIN" report seal --target-root=/repo --target-scope=a.sh \
        --iteration=1 --routed=SEC --"$flag"=4294967296 --summary="s" 2>&1)"
    CODE=$?
    set -e
    if [[ "$CODE" != "0" ]] && grep -q -- "--$flag" <<<"$OUT"; then
        pass "--$flag above uint32 is refused by name"
    else
        fail "--$flag above uint32 was accepted (exit $CODE)"
    fi
done

set +e
OUT="$(cd "$WORK" && "$BIN" report add --category=SEC --severity=P2 \
    --blast=local_component --likelihood=unlikely --path=b.sh \
    --tier=E1 --claim="c" --action="go test" --cwd=. --exit-code=2147483648 --output="x" 2>&1)"
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -q -- '--exit-code' <<<"$OUT"; then
    pass "--exit-code above int32 is refused by name"
else
    fail "--exit-code above int32 was accepted (exit $CODE)"
fi
rm -rf "$WORK"

echo ""
echo "--- Sealing ends the report it sealed ---"

# The next seal stamps the current run's identity onto whatever candidates it
# finds. Leaving them would let a later request adopt findings its own pass
# never made, and they would pass the engine's binding check because the
# identity is current.
WORK="$(mktemp -d)"
mkdir -p "$WORK/.grimes"
(
    cd "$WORK" || exit 1
    "$BIN" report add --category=SEC --severity=P2 --blast=local_component \
        --likelihood=unlikely --path=a.sh --tier=E2 --claim="c" --quote="q" >/dev/null
    "$BIN" report stop --category=SEC --condition=marginal-yield --probes=2 >/dev/null
    "$BIN" report seal --target-root=/repo --target-scope=a.sh --iteration=1 \
        --routed=SEC --examined=1 --disproved=0 --summary="s" >/dev/null
) || fail "the seal sequence failed"

if [[ -f "$WORK/.grimes/report.textproto" ]]; then
    fail "the working report survived its own sealing"
else
    pass "sealing clears the candidates it sealed"
fi

SECOND="$(cd "$WORK" && "$BIN" report seal --target-root=/repo --target-scope=a.sh \
    --iteration=2 --examined=0 --disproved=0 --summary="s" |
    "$BIN" decode-report)"
if grep -q 'candidates' <<<"$SECOND"; then
    fail "a later seal readopted the previous report's candidates"
else
    pass "a later seal carries no candidate it was not given"
fi
rm -rf "$WORK"

echo ""
echo "--- A unit id survives whatever characters it holds ---"

# An external target's unit id is its URI, so a skip that split its argument on
# a delimiter would record a fragment. The engine measures coverage against the
# inventory and refuses a fragment as a unit outside the target, which makes the
# unit impossible to skip and denies an otherwise complete review.
WORK="$(mktemp -d)"
mkdir -p "$WORK/.grimes"
HOSTILE='https://example.com/policy,v2
second line'
SKIPPED="$(cd "$WORK" && "$BIN" report cover --skip="$HOSTILE" \
    --skip-reason="the publisher withdrew it" --skip-material >/dev/null &&
    "$BIN" report show --file="$WORK/.grimes/report.textproto")"
if [[ "$(grep -c 'unit_id' <<<"$SKIPPED")" == "1" ]] &&
    grep -qF 'https://example.com/policy,v2' <<<"$SKIPPED"; then
    pass "a unit id holding a colon, a comma, and a newline is skipped whole"
else
    fail "the skipped unit id was split: $SKIPPED"
fi

# The reason is what separates a deliberate skip from an oversight.
set +e
OUT=$(cd "$WORK" && "$BIN" report cover --skip="a.sh" 2>&1)
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -q 'skip-reason' <<<"$OUT"; then
    pass "a skip with no reason is refused by name"
else
    fail "an unexplained skip was accepted (exit $CODE)"
fi
rm -rf "$WORK"

echo ""
echo "--- An unavailable disproof cannot carry the attempt it did not make ---"

WORK="$(mktemp -d)"
mkdir -p "$WORK/.grimes"
ADD=(report add --category=SEC --severity=P2 --blast=local_component
    --likelihood=likely --path=a.sh --tier=E2 --claim=c --quote=q
    --disproof-unavailable="the service is not reachable")

# Every performed-side flag, including the ones carrying a default and the
# contradiction flag the contract would reject on its own.
for extra in --disproof-action=x --disproof-cwd=. --disproof-exit=1 \
    --disproof-output=y --disproof-output-sha256=aa --disproof-contradicts; do
    set +e
    OUT=$(cd "$WORK" && "$BIN" "${ADD[@]}" "$extra" 2>&1)
    CODE=$?
    set -e
    if [[ "$CODE" != "0" ]] && grep -qF -- "--disproof-unavailable" <<<"$OUT"; then
        pass "an unavailable disproof refuses $extra rather than dropping it"
    else
        fail "$extra was accepted alongside --disproof-unavailable (exit $CODE): $OUT"
    fi
done

# The bare form still works, or the refusal above would be refusing everything.
if (cd "$WORK" && "$BIN" "${ADD[@]}" >/dev/null 2>&1); then
    pass "an unavailable disproof on its own is recorded"
else
    fail "an unavailable disproof was refused on its own"
fi
rm -rf "$WORK"

echo ""
echo "--- A refusal names a flag the command actually defines ---"

# Every prefixed evidence family shortens the exit flag: --probe-exit, not
# --probe-exit-code. A message built by concatenation sends the caller looking
# for a flag nobody defined.
WORK="$(mktemp -d)"
mkdir -p "$WORK/.grimes"
BIG=2147483648
for case in \
    "disproof-exit:report add --category=SEC --severity=P2 --blast=local_component --likelihood=likely --path=a.sh --tier=E2 --claim=c --quote=q --disproof-action=x --disproof-output=y" \
    "probe-exit:report acquit --category=SEC --path=a.sh --claim=c --scope=s --probe-action=x --probe-output=y" \
    "control-exit:report acquit --category=SEC --path=a.sh --claim=c --scope=s --probe-action=x --probe-exit=0 --probe-output=y --control-mutation=m --control-action=x --control-output=z"; do
    FLAG="${case%%:*}"
    ARGS="${case#*:}"
    set +e
    # shellcheck disable=SC2086  # the argument list is the case under test
    OUT=$(cd "$WORK" && "$BIN" $ARGS "--$FLAG=$BIG" 2>&1)
    CODE=$?
    set -e
    if [[ "$CODE" != "0" ]] && grep -qF -- "--$FLAG must be between" <<<"$OUT"; then
        pass "an out-of-range --$FLAG is refused by its own name"
    else
        fail "--$FLAG was refused under another name (exit $CODE): $OUT"
    fi
done
rm -rf "$WORK"

echo ""
echo "--- An acquittal carries the control that showed its probe can fail ---"

WORK="$(mktemp -d)"
mkdir -p "$WORK/.grimes"
ACQUIT=(report acquit --category=SEC --path=a.sh
    --claim="the target rejects an unsigned request" --scope="the request path"
    --probe-action="run the suite" --probe-exit=0 --probe-output="all held")
CONTROL=(--control-mutation="drop the signature check" --control-action="run the suite"
    --control-output="the unsigned request was accepted")

# Whether the control failed is read from its exit status. A caller that could
# declare it would be writing down the outcome it wanted.
set +e
OUT=$(cd "$WORK" && "$BIN" "${ACQUIT[@]}" "${CONTROL[@]}" --control-exit=0 2>&1)
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -qi 'capable of failing' <<<"$OUT"; then
    pass "a control the probe survived is refused by name"
else
    fail "a probe that passed against the mutated target was accepted (exit $CODE): $OUT"
fi

# A mutation nobody probed and a probe with nothing mutated are both records of
# something that did not happen. The last case would otherwise reach the
# contract and be refused by field name rather than by flag.
PARTIALS=(--control-mutation=x --control-action=y --control-output=z)
for partial in "${PARTIALS[@]}"; do
    set +e
    OUT=$(cd "$WORK" && "$BIN" "${ACQUIT[@]}" "$partial" 2>&1)
    CODE=$?
    set -e
    if [[ "$CODE" != "0" ]] && grep -q -- '--control-' <<<"$OUT"; then
        pass "a partial negative control is refused by name ($partial)"
    else
        fail "$partial alone was accepted (exit $CODE): $OUT"
    fi
done

set +e
OUT=$(cd "$WORK" && "$BIN" "${ACQUIT[@]}" --control-action=y --control-output=z --control-exit=1 2>&1)
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -q -- '--control-mutation' <<<"$OUT"; then
    pass "a control with nothing mutated is refused by name"
else
    fail "a control with no mutation was accepted or refused by field (exit $CODE): $OUT"
fi

set +e
OUT=$(cd "$WORK" && "$BIN" "${ACQUIT[@]}" --control-exit=0 2>&1)
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -q -- '--control-mutation' <<<"$OUT"; then
    pass "--control-exit=0 alone is a partial control, not the absence of one"
else
    fail "--control-exit=0 alone was read as no control at all (exit $CODE): $OUT"
fi

OUT=$(cd "$WORK" && "$BIN" "${ACQUIT[@]}" "${CONTROL[@]}" --control-exit=1 2>&1)
if grep -q 'failed as required' <<<"$OUT"; then
    pass "a control the probe failed against is admitted"
else
    fail "a controlled acquittal was refused: $OUT"
fi

# An acquittal with no control is a record of what was done. It earns nothing
# downstream, which is the engine's business rather than this command's.
if (cd "$WORK" && "$BIN" "${ACQUIT[@]}" >/dev/null 2>&1); then
    pass "an acquittal with no control is recorded"
else
    fail "an acquittal without a control was refused"
fi
rm -rf "$WORK"

echo ""
echo "--- A retrieved source carries its retrieval identity ---"

# Every --source candidate was rejected by validation, because the anchor was
# built with only the URI while the contract also requires a publisher, a
# snapshot digest, and a retrieval time.
WORK="$(mktemp -d)"
mkdir -p "$WORK/.grimes"
DIGEST="$(printf 'a%.0s' $(seq 1 64))"
set +e
OUT="$(cd "$WORK" && "$BIN" report add --category=SEC --severity=P2 \
    --blast=local_component --likelihood=unlikely --source="https://example.com/p" \
    --tier=E2 --claim="c" --quote="q" 2>&1)"
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -q -- '--publisher' <<<"$OUT"; then
    pass "a source without its retrieval identity is refused by name"
else
    fail "a source without its retrieval identity was accepted (exit $CODE)"
fi

if (cd "$WORK" && "$BIN" report add --category=SEC --severity=P2 \
    --blast=local_component --likelihood=unlikely --source="https://example.com/p" \
    --publisher="Example" --snapshot-sha256="$DIGEST" \
    --retrieved-at="2026-01-02T15:04:05Z" --source-section="Clause 4" \
    --tier=E2 --claim="c" --quote="q" >/dev/null 2>&1); then
    pass "a source with its retrieval identity is admitted"
else
    fail "a complete source candidate was still rejected"
fi

for bad in "--snapshot-sha256=zz --retrieved-at=2026-01-02T15:04:05Z" \
    "--snapshot-sha256=$DIGEST --retrieved-at=yesterday"; do
    set +e
    # shellcheck disable=SC2086  # the pair under test is two flags, not one word
    OUT="$(cd "$WORK" && "$BIN" report add --category=SEC --severity=P3 \
        --blast=local_component --likelihood=unlikely --source="https://example.com/q" \
        --publisher="Example" $bad --tier=E2 --claim="c2" --quote="q" 2>&1)"
    CODE=$?
    set -e
    if [[ "$CODE" != "0" ]]; then
        pass "a malformed source identity is refused ($bad)"
    else
        fail "a malformed source identity was accepted ($bad)"
    fi
done
rm -rf "$WORK"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
