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
