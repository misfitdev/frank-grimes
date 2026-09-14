#!/usr/bin/env bash
# fg-64y.18: collection resolves any target kind, not just a repository path.
#
# Black-box: every case drives the real grimes CLI against a real artifact, the
# way an adapter must. The unit inventory it records is asserted through the
# codec rather than by reaching into Go.
#
# Exit codes: 0 all passed, 1 a test failed, 2 toolchain unavailable.

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAKES="$PROJECT_ROOT/tests/fakes"

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

assert_eq() {
    if [[ "$1" == "$2" ]]; then pass "$3"; else fail "$3 (got '$1' vs '$2')"; fi
}

if ! command -v go &>/dev/null; then
    echo -e "${YELLOW}SKIP${NC}: go is not installed; collector tests require the toolchain"
    exit 2
fi

BINDIR="$(mktemp -d)"
trap 'rm -rf "$BINDIR"' EXIT
GRIMES="$BINDIR/grimes"
export PATH="$BINDIR:$PATH"

echo "--- Build ---"
if (cd "$PROJECT_ROOT" && go build -o "$GRIMES" ./cmd/grimes) 2>/dev/null &&
    (cd "$PROJECT_ROOT" && go build -o "$BINDIR/grimes-contract" ./cmd/grimes-contract) 2>/dev/null; then
    pass "grimes and grimes-contract build"
else
    fail "grimes and grimes-contract build"
    echo "Passed: $PASSED / Failed: $FAILED"
    exit 1
fi

# The inventory is a machine record, so it is read back through the codec.
inventory() {
    grimes-contract validate --type=TargetInventory "$1/.grimes/inventory.pb" 2>&1
}

show_inventory() {
    grimes-contract decode-report --type=TargetInventory "$1/.grimes/inventory.pb" 2>/dev/null
}

# A guard that fails leaves a read blocked on a FIFO, which would hang the suite
# rather than fail it. GNU timeout is not on every platform, so the bound is kept
# here: 124 on expiry, matching what timeout would have returned.
bounded() {
    local secs="$1"
    shift
    local out waited=0 pid code
    out="$(mktemp)"
    "$@" >"$out" 2>&1 &
    pid=$!
    while kill -0 "$pid" 2>/dev/null && [[ "$waited" -lt "$secs" ]]; do
        sleep 1
        waited=$((waited + 1))
    done
    if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" 2>/dev/null
        wait "$pid" 2>/dev/null
        code=124
    else
        wait "$pid"
        code=$?
    fi
    cat "$out"
    rm -f "$out"
    return "$code"
}

# prototext's spacing is randomized per binary build
# (google.golang.org/protobuf/internal/detrand), so match on content.
units_in() {
    show_inventory "$1" | grep -cE 'units:? +\{' || true
}

echo ""
echo "--- A document is reviewable with no repository present ---"

WS="$(mktemp -d)"
cat >"$WS/spec.md" <<'DOC'
# Overview
The service accepts requests.

## Authentication
Tokens are checked.

## Storage
Records are written.
DOC
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --kind=document \
    --provider-command="$FAKES/provider-red.sh" --format=prototext spec.md 2>&1)"
CODE=$?
set -e
# No git repository, no repository root: a document is named in full by itself.
if [[ "$CODE" != "1" ]]; then
    pass "a document target completes outside a repository"
else
    fail "a document target failed outside a repository: $OUT"
fi
if echo "$OUT" | grep -qE 'kind: +TARGET_KIND_DOCUMENT'; then
    pass "the record names the target as a document"
else
    fail "the record does not name the target as a document"
fi
if echo "$OUT" | grep -qE 'root: +"'; then
    fail "a document target recorded a repository root"
else
    pass "a document target records no repository root"
fi
if [[ -f "$WS/.grimes/inventory.pb" ]] && inventory "$WS" >/dev/null; then
    pass "collection persists an inventory the contract accepts"
else
    fail "collection persisted no valid inventory"
fi
# Three headings, so three units. A whole-file fallback would record one.
UNITS="$(units_in "$WS")"
assert_eq "$UNITS" "3" "each document heading is one unit"
rm -rf "$WS"

# A document with no heading is still wholly accountable, so it is one unit
# rather than none: the contract refuses an empty inventory.
WS="$(mktemp -d)"
printf 'Just prose, no headings at all.\n' >"$WS/notes.md"
set +e
(cd "$WS" && "$GRIMES" run --dir=. --kind=document \
    --provider-command="$FAKES/provider-red.sh" notes.md >/dev/null 2>&1)
set -e
UNITS="$(units_in "$WS")"
assert_eq "$UNITS" "1" "a document with no heading is one unit"
rm -rf "$WS"

echo ""
echo "--- An argument is reviewable from stdin ---"

WS="$(mktemp -d)"
set +e
OUT="$(cd "$WS" && printf 'First, costs fall.\n\nSecond, adoption rises.\n\nTherefore we should ship.\n' |
    "$GRIMES" run --dir=. --kind=idea \
        --provider-command="$FAKES/provider-red.sh" --format=prototext - 2>&1)"
set -e
if echo "$OUT" | grep -qE 'kind: +TARGET_KIND_IDEA'; then
    pass "a pasted argument is reviewable as an idea"
else
    fail "a pasted argument is not reviewable as an idea: $OUT"
fi
UNITS="$(units_in "$WS")"
assert_eq "$UNITS" "3" "each step of the argument is one unit"
# A pasted argument has no file and no repository, so the record names it as
# stdin and roots it nowhere.
if echo "$OUT" | grep -qE 'scope: +"stdin"'; then
    pass "a pasted argument is recorded as stdin"
else
    fail "a pasted argument is not recorded as stdin"
fi
if echo "$OUT" | grep -qE 'root: +"'; then
    fail "a pasted argument recorded a repository root"
else
    pass "a pasted argument records no repository root"
fi
rm -rf "$WS"

# An empty target names nothing. The CLI counts it as an argument, so the
# refusal has to come from collection.
WS="$(mktemp -d)"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" "" 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "1" ]] && grep -qi 'scope' <<<"$OUT"; then
    pass "an empty target is refused by name"
else
    fail "an empty target was not refused (exit $CODE)"
fi
rm -rf "$WS"

echo ""
echo "--- A finding is anchored in the target that was reviewed ---"

# The engine refuses a finding whose anchor is not a unit of the target, so a
# fake that anchors elsewhere would model a provider whose findings can never be
# admitted, and every test built on it would fail for that reason rather than
# its own.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
# A RED run exits non-zero by design, so the verdict is not what is under test.
(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" src >/dev/null 2>&1) || true
LEDGER="$(grimes-contract decode-report --type=Ledger "$WS/.grimes/ledger.pb" 2>/dev/null || true)"
if grep -qE 'value: +"src/a.sh"' <<<"$LEDGER"; then
    pass "a finding is anchored at a unit of the target"
else
    fail "the finding was anchored outside the target: $(grep -aoE 'value: +"[^"]*"' <<<"$LEDGER" | head -2 | tr '\n' ' ')"
fi
rm -rf "$WS"

echo ""
echo "--- Evidence has to point at the target that was reviewed ---"

# The contract sees that a citation carries a quote and an anchor. Only the
# engine holds the artifact, so only the engine can tell whether either names
# anything real.
for case in \
    "provider-foreign-anchor:not part of:a finding anchored outside the target is refused by name" \
    "provider-invented-quote:quoted text is not in:a quotation that is nowhere in the target is refused by name" \
    "provider-foreign-cwd:outside the target:a command that ran outside the target is refused by name"; do
    FAKE="${case%%:*}"
    REST="${case#*:}"
    NEEDLE="${REST%%:*}"
    WHAT="${REST#*:}"
    WS="$(mktemp -d)"
    mkdir -p "$WS/src"
    printf 'echo one\n' >"$WS/src/a.sh"
    # A relative name with no ".." that leaves the tree anyway. The contract
    # cannot see this; resolving it needs the filesystem.
    ln -s "$(mktemp -d)" "$WS/escape"
    set +e
    OUT="$(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/$FAKE.sh" src 2>&1)"
    CODE=$?
    set -e
    if [[ "$CODE" != "0" ]] && grep -qF "$NEEDLE" <<<"$OUT"; then
        pass "$WHAT"
    else
        fail "$WHAT (exit $CODE): $OUT"
    fi
    rm -rf "$WS"
done

# The provider runs before evidence is checked and can write to the artifact it
# was asked to review. Re-reading alone would admit the line it planted.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-edits-target.sh" src 2>&1)"
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -qF "changed during the review" <<<"$OUT"; then
    pass "a quote of text the provider planted is refused by name"
else
    fail "a provider quoted its own edit (exit $CODE): $OUT"
fi
rm -rf "$WS"

# The same, for a target fingerprinted over the whole of its content. One
# heading, so the planted line lands in the section the finding anchors to and
# only the fingerprint can refuse it.
WS="$(mktemp -d)"
printf '# Overview\nThe service accepts requests.\n' >"$WS/spec.md"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --kind=document \
    --provider-command="$FAKES/provider-edits-document.sh" spec.md 2>&1)"
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -qF "changed during the review" <<<"$OUT"; then
    pass "a document edited mid-review is refused by name"
else
    fail "a provider quoted its own edit to a document (exit $CODE): $OUT"
fi
rm -rf "$WS"

# A line that really is in the document, but not in the section the finding
# anchors to. Checking the whole file would admit it, and the citation claims
# the section rather than the document.
WS="$(mktemp -d)"
cat >"$WS/spec.md" <<'DOC'
# Overview
The service accepts requests.

## Storage
Records are written.
DOC
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --kind=document \
    --provider-command="$FAKES/provider-cross-section-quote.sh" spec.md 2>&1)"
CODE=$?
set -e
if [[ "$CODE" != "0" ]] && grep -qF "quoted text is not in" <<<"$OUT"; then
    pass "a quote from another section of the same document is refused by name"
else
    fail "a cross-section quote was admitted (exit $CODE): $OUT"
fi
rm -rf "$WS"

# A quote the reviewer really read, differing only in how the line ended. The
# refusal has to be about the text being absent, not about line endings.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\r\n' >"$WS/src/a.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" src 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "4" ]]; then
    pass "a quote is admitted across a line-ending difference"
else
    fail "a CRLF target refused its own content (exit $CODE): $OUT"
fi
rm -rf "$WS"

echo ""
echo "--- An external source needs a frozen snapshot ---"

WS="$(mktemp -d)"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --kind=external \
    --provider-command="$FAKES/provider-red.sh" "https://example.com/policy" 2>&1)"
CODE=$?
set -e
# Both halves matter: a run that fetched the source would not be repeatable, and
# one that silently reviewed nothing would report coverage it does not have.
if [[ "$CODE" != "0" ]] && grep -qi 'snapshot' <<<"$OUT"; then
    pass "an external target without a snapshot is refused by name"
else
    fail "an external target without a snapshot was not refused (exit $CODE)"
fi
rm -rf "$WS"

WS="$(mktemp -d)"
printf 'Clause 4: retention is 30 days.\n' >"$WS/frozen.txt"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --kind=external --snapshot=frozen.txt \
    --provider-command="$FAKES/provider-red.sh" --format=prototext "https://example.com/policy" 2>&1)"
set -e
if echo "$OUT" | grep -qE 'kind: +TARGET_KIND_EXTERNAL'; then
    pass "an external target with a snapshot is reviewable"
else
    fail "an external target with a snapshot is not reviewable: $OUT"
fi
if echo "$OUT" | grep -q 'example.com/policy'; then
    pass "the record names the source the snapshot was taken from"
else
    fail "the record does not name the source"
fi
rm -rf "$WS"

WS="$(mktemp -d)"
printf 'x\n' >"$WS/a.txt"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --snapshot=a.txt \
    --provider-command="$FAKES/provider-red.sh" a.txt 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "2" ]] || grep -qi 'only.*external' <<<"$OUT"; then
    pass "a snapshot without an external kind is refused"
else
    fail "a snapshot was accepted for a non-external kind (exit $CODE)"
fi
rm -rf "$WS"

echo ""
echo "--- A code target is fingerprinted by its content ---"

WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
printf 'echo two\n' >"$WS/src/b.sh"
(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" src >/dev/null 2>&1) || true
UNITS="$(units_in "$WS")"
assert_eq "$UNITS" "2" "each file under a code target is one unit"

# Editing the target mid-review changes what is under review. The ledger is
# raised against the fingerprint, so the second run must refuse rather than
# count the first target's findings toward the edited one.
printf 'echo one changed\n' >"$WS/src/a.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" src 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "1" ]] && grep -qi 'different target' <<<"$OUT"; then
    pass "an edited code target no longer matches its ledger"
else
    fail "an edited code target still matched its ledger (exit $CODE)"
fi
rm -rf "$WS"

# A target that is not there is not a target. Reviewing it would report a
# verdict over nothing.
WS="$(mktemp -d)"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" nowhere 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "1" ]] && grep -qi 'nowhere' <<<"$OUT"; then
    pass "a missing code target is refused by name"
else
    fail "a missing code target was not refused (exit $CODE)"
fi
rm -rf "$WS"

echo ""
echo "--- A code target cannot leave the repository root ---"

# A review names the repository it covers. Walking out of it would put files
# under review that the caller did not name, while the record still claims the
# named root.
WS="$(mktemp -d)"
OUTSIDE="$(mktemp -d)"
mkdir -p "$WS/repo/src"
printf 'echo inside\n' >"$WS/repo/src/a.sh"
printf 'secret\n' >"$OUTSIDE/secret.txt"
for escape in "../outside" "src/../../outside"; do
    set +e
    OUT="$(cd "$WS/repo" && "$GRIMES" run --dir=. \
        --provider-command="$FAKES/provider-red.sh" "$escape" 2>&1)"
    CODE=$?
    set -e
    if [[ "$CODE" == "1" ]] && grep -qi 'outside the repository root' <<<"$OUT"; then
        pass "a scope of $escape is refused by name"
    else
        fail "a scope of $escape was not refused (exit $CODE)"
    fi
done
# A symlink is the same escape wearing a different hat.
ln -s "$OUTSIDE" "$WS/repo/link"
set +e
OUT="$(cd "$WS/repo" && "$GRIMES" run --dir=. \
    --provider-command="$FAKES/provider-red.sh" link 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "1" ]] && grep -qi 'outside the repository root' <<<"$OUT"; then
    pass "a symlink out of the root is refused"
else
    fail "a symlink out of the root was not refused (exit $CODE)"
fi
rm -rf "$WS" "$OUTSIDE"

# A walk skips a non-regular file, so a directly named one must be refused
# rather than read: a FIFO blocks with nothing to cancel it.
WS="$(mktemp -d)"
mkfifo "$WS/pipe"
set +e
OUT="$(cd "$WS" && bounded 15 "$GRIMES" run --dir=. \
    --provider-command="$FAKES/provider-red.sh" pipe)"
CODE=$?
set -e
if [[ "$CODE" == "1" ]] && grep -qi 'not a regular file' <<<"$OUT"; then
    pass "a named pipe is refused rather than read"
else
    fail "a named pipe was not refused (exit $CODE)"
fi
rm -rf "$WS"

# Every file-backed kind reads a caller-named path, so each has to refuse a
# special file: a FIFO blocks inside the read with nothing to cancel it.
WS="$(mktemp -d)"
mkfifo "$WS/pipe"
printf 'x\n' >"$WS/real.txt"
for kind in document idea; do
    set +e
    OUT="$(cd "$WS" && bounded 15 "$GRIMES" run --dir=. --kind="$kind" \
        --provider-command="$FAKES/provider-red.sh" pipe)"
    CODE=$?
    set -e
    if [[ "$CODE" == "1" ]] && grep -qi 'not a regular file' <<<"$OUT"; then
        pass "the $kind target that is a named pipe is refused"
    else
        fail "the $kind named pipe was not refused (exit $CODE)"
    fi
done
set +e
OUT="$(cd "$WS" && bounded 15 "$GRIMES" run --dir=. --kind=external --snapshot=pipe \
    --provider-command="$FAKES/provider-red.sh" "https://example.com/p")"
CODE=$?
set -e
if [[ "$CODE" == "1" ]] && grep -qi 'not a regular file' <<<"$OUT"; then
    pass "an external snapshot that is a named pipe is refused"
else
    fail "an external named-pipe snapshot was not refused (exit $CODE)"
fi
rm -rf "$WS"

echo ""
echo "--- A rejected run leaves the inventory alone ---"

# The inventory describes the target the surviving state and ledger are about.
# A second target in the same directory is refused, and must not replace it.
WS="$(mktemp -d)"
mkdir -p "$WS/src" "$WS/other"
printf 'echo one\n' >"$WS/src/a.sh"
printf 'echo two\n' >"$WS/other/b.sh"
(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" src >/dev/null 2>&1) || true
FIRST="$(show_inventory "$WS")"
set +e
(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" other >/dev/null 2>&1)
set -e
if [[ "$FIRST" == "$(show_inventory "$WS")" ]]; then
    pass "a refused target does not overwrite the inventory"
else
    fail "a refused target overwrote the inventory"
fi
rm -rf "$WS"

echo ""
echo "--- The provider reads the target it was asked to review ---"

# provider-verify-content.sh digests what it was handed and refuses to report
# unless that digest is the target the engine named. A run that succeeds is
# therefore a run where the provider saw the artifact, which is the whole point:
# a finding about an artifact nobody could read is a finding about nothing.
run_verified() {
    local ws="$1"
    shift
    set +e
    VERIFY_OUT="$(cd "$ws" && "$GRIMES" run --dir=. \
        --provider-command="$FAKES/provider-verify-content.sh" "$@" 2>&1)"
    VERIFY_CODE=$?
    set -e
}

# A blocking verdict means the provider read the content and reported on it.
# Anything else is the fake refusing, and its reason is worth printing.
assert_verified() {
    if [[ "$VERIFY_CODE" == "4" ]]; then
        pass "$1"
    else
        fail "$1 (exit $VERIFY_CODE): $(tail -1 <<<"$VERIFY_OUT")"
    fi
}

# A pasted argument: consumed from stdin, so it exists nowhere until the engine
# writes it down.
WS="$(mktemp -d)"
set +e
VERIFY_OUT="$(cd "$WS" && printf 'First, costs fall.\n\nTherefore we should ship.\n' |
    "$GRIMES" run --dir=. --kind=idea \
        --provider-command="$FAKES/provider-verify-content.sh" - 2>&1)"
VERIFY_CODE=$?
set -e
assert_verified "a pasted argument is readable by the provider"
if [[ -f "$WS/.grimes/target.bin" ]]; then
    pass "a target with no path of its own is written down"
else
    fail "a pasted argument was never written down"
fi
rm -rf "$WS"

# An external target: the snapshot is the artifact, and it already has a path.
WS="$(mktemp -d)"
printf 'Clause 4: retention is 30 days.\n' >"$WS/frozen.txt"
run_verified "$WS" --kind=external --snapshot=frozen.txt "https://example.com/policy"
assert_verified "an external snapshot is readable by the provider"
if [[ -f "$WS/.grimes/target.bin" ]]; then
    fail "an external snapshot was copied when it already had a path"
else
    pass "a target that has a path is not copied"
fi
rm -rf "$WS"

WS="$(mktemp -d)"
printf '# One\n\ntext\n' >"$WS/spec.md"
run_verified "$WS" --kind=document spec.md
assert_verified "a document is readable by the provider"
rm -rf "$WS"

WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
run_verified "$WS" src
assert_verified "a code target is handed the tree under review"
rm -rf "$WS"

# A code target may name one file rather than a tree; collection supports it,
# so the content path is that file and nothing downstream may assume otherwise.
WS="$(mktemp -d)"
printf 'echo one\n' >"$WS/lone.sh"
run_verified "$WS" lone.sh
assert_verified "a single-file code target is handed that file"
rm -rf "$WS"

# Collection resolves a relative scope against its own working directory and the
# provider runs in the review directory. The same name in both is two different
# files, and the provider must be pointed at the one that was fingerprinted.
WS="$(mktemp -d)"
ELSEWHERE="$(mktemp -d)"
printf 'the one collection read\n' >"$ELSEWHERE/spec.md"
printf 'the one in the review directory\n' >"$WS/spec.md"
set +e
VERIFY_OUT="$(cd "$ELSEWHERE" && "$GRIMES" run --dir="$WS" --kind=document \
    --provider-command="$FAKES/provider-verify-content.sh" spec.md 2>&1)"
VERIFY_CODE=$?
set -e
assert_verified "a relative scope names one file to both halves of the run"
rm -rf "$WS" "$ELSEWHERE"

# The adjudicator judges the same artifact. Zero knowledge is about the first
# report, not about the target, and an adjudicator that cannot read a pasted
# argument cannot form an opinion of its own.
WS="$(mktemp -d)"
printf 'Clause 4: retention is 30 days.\n' >"$WS/frozen.txt"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --kind=external --snapshot=frozen.txt \
    --provider-command="$FAKES/provider-red.sh" \
    --adjudicator-command="$FAKES/adjudicator-reads-content.sh" \
    --format=prototext "https://example.com/policy" 2>&1)"
set -e
if echo "$OUT" | grep -qE 'context_origin: +CONTEXT_ORIGIN_'; then
    pass "an adjudicator that read the artifact still records an independent review"
else
    fail "the adjudicator could not read the artifact: $OUT"
fi
rm -rf "$WS"

echo ""
echo "--- A review answers for every unit of the target ---"

# The first GREEN the engine can produce. Until coverage was measured against
# the inventory, completeness could never reach sufficient and this tuple was
# unreachable by construction, whatever a review found.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --format=prototext \
    --provider-command="$FAKES/provider-green.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh src 2>&1)"
CODE=$?
set -e
assert_eq "$CODE" "0" "a fully accounted clean review passes"
if echo "$OUT" | grep -qE 'legacy_color: +LEGACY_COLOR_GREEN'; then
    pass "a fully accounted clean review reaches GREEN"
else
    fail "a fully accounted clean review did not reach GREEN: $(grep -aoE 'unmet_gates: +"[^"]*"' <<<"$OUT" | tr '\n' ' ')"
fi
rm -rf "$WS"

# The same review with the control removed. This is the shape that let PR #28
# ship: a check that passed while the defect it named was present, recorded as
# evidence the target was sound. Nothing else about the run changes.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --format=prototext \
    --provider-command="$FAKES/provider-uncontrolled.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh src 2>&1)"
CODE=$?
set -e
assert_eq "$CODE" "0" "an uncontrolled review completes"
if echo "$OUT" | grep -qE 'legacy_color: +LEGACY_COLOR_GREEN'; then
    fail "a probe never shown capable of failing still reached GREEN"
else
    pass "a probe never shown capable of failing does not reach GREEN"
fi
if echo "$OUT" | grep -qE 'review_completeness: +REVIEW_COMPLETENESS_INCONCLUSIVE'; then
    pass "an uncontrolled probe leaves completeness inconclusive"
else
    fail "completeness was not inconclusive: $(grep -aoE 'review_completeness: +[A-Z_]*' <<<"$OUT")"
fi
rm -rf "$WS"

# A unit nobody looked at is the difference between a verdict over the target
# and a verdict over part of it.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
printf 'echo two\n' >"$WS/src/unexamined.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --format=prototext \
    --provider-command="$FAKES/provider-partial-coverage.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh src 2>&1)"
CODE=$?
set -e
assert_eq "$CODE" "3" "an unaccounted unit holds the decision at conditional"
if echo "$OUT" | grep -qE 'unmet_gates: +"coverage"'; then
    pass "the record names coverage as the unmet gate"
else
    fail "the record does not name coverage"
fi
if echo "$OUT" | grep -qE 'legacy_color: +LEGACY_COLOR_GREEN'; then
    fail "a review that skipped a unit still reached GREEN"
else
    pass "a review that skipped a unit does not reach GREEN"
fi
rm -rf "$WS"

# Coverage naming something outside the target describes a review of a
# different artifact, which is an error rather than a finding.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. \
    --provider-command="$FAKES/provider-invented-coverage.sh" src 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "1" ]] && grep -qi 'not in the target' <<<"$OUT"; then
    pass "coverage of a unit outside the target is refused"
else
    fail "invented coverage was accepted (exit $CODE)"
fi
rm -rf "$WS"

# A material skip is an admission that part of the target went unreviewed. Two
# units, so the skip can be held out of the examined set and the two still cover
# the inventory between them: the run has to succeed for the assertion to mean
# anything, and a bare "not sufficient" would also be satisfied by a crash.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
printf 'echo two\n' >"$WS/src/b.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --format=prototext \
    --provider-command="$FAKES/provider-material-skip.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh src 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "0" ]]; then
    pass "a material skip completes the run"
else
    fail "material-skip run exited $CODE, wanted 0: $OUT"
fi
if echo "$OUT" | grep -qE 'unmet_gates: +"coverage"'; then
    fail "a skipped unit was counted as unaccounted rather than as a skip"
else
    pass "an explicitly skipped unit is accounted for"
fi
if echo "$OUT" | grep -qE 'review_completeness: +REVIEW_COMPLETENESS_LIMITED'; then
    pass "a material skip holds completeness at limited"
else
    fail "a material skip did not hold completeness at limited"
fi
rm -rf "$WS"

# A category that stopped for want of evidence did not finish its grind.
WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
set +e
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --format=prototext \
    --provider-command="$FAKES/provider-blocked-category.sh" \
    --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh src 2>&1)"
CODE=$?
set -e
if [[ "$CODE" == "0" ]]; then
    pass "a blocked category completes the run"
else
    fail "blocked-category run exited $CODE, wanted 0: $OUT"
fi
if echo "$OUT" | grep -qE 'legacy_color: +LEGACY_COLOR_GREEN'; then
    fail "a blocked category still reached GREEN"
else
    pass "a category blocked for want of evidence does not reach GREEN"
fi
rm -rf "$WS"

# Full accounting with no repository present, for the kinds that have none.
for kind in document idea; do
    WS="$(mktemp -d)"
    printf '# One\n\ntext\n' >"$WS/spec.md"
    set +e
    OUT="$(cd "$WS" && "$GRIMES" run --dir=. --kind="$kind" --format=prototext \
        --provider-command="$FAKES/provider-green.sh" \
        --adjudicator-command="$FAKES/adjudicator-pass.sh" --adjudicator-fresh spec.md 2>&1)"
    CODE=$?
    set -e
    if [[ "$CODE" == "0" ]]; then
        pass "a $kind target with no repository passes"
    else
        fail "$kind run exited $CODE, wanted 0: $OUT"
    fi
    if echo "$OUT" | grep -qE 'unmet_gates: +"coverage"'; then
        fail "a $kind target could not account for its units"
    else
        pass "a $kind target reaches full accounting with no repository"
    fi
    rm -rf "$WS"
done

echo ""
echo "--- The inventory belongs to the target it was taken from ---"

WS="$(mktemp -d)"
mkdir -p "$WS/src"
printf 'echo one\n' >"$WS/src/a.sh"
OUT="$(cd "$WS" && "$GRIMES" run --dir=. --provider-command="$FAKES/provider-red.sh" --format=prototext src 2>&1)" || true
TARGET_FP="$(echo "$OUT" | grep -m1 -oE 'fingerprint_sha256: +"[^"]*"' | sed 's/.*: *//')"
INV_FP="$(show_inventory "$WS" | grep -m1 -oE 'target_fingerprint_sha256: +"[^"]*"' | sed 's/.*: *//')"
if [[ -n "$TARGET_FP" && "$TARGET_FP" == "$INV_FP" ]]; then
    pass "the inventory carries the fingerprint of the target reviewed"
else
    fail "the inventory fingerprint does not match the target ($INV_FP vs $TARGET_FP)"
fi
rm -rf "$WS"

echo ""
echo "--- An unknown kind is refused rather than guessed ---"

WS="$(mktemp -d)"
set +e
(cd "$WS" && "$GRIMES" run --dir=. --kind=diagram \
    --provider-command="$FAKES/provider-red.sh" x >/dev/null 2>&1)
CODE=$?
set -e
assert_eq "$CODE" "1" "an unknown target kind is refused"
rm -rf "$WS"

echo ""
echo "========================================"
echo "Passed: $PASSED"
echo "Failed: $FAILED"
echo "========================================"

if [[ "$FAILED" -gt 0 ]]; then
    exit 1
fi
