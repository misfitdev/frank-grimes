default:
    @just --list

# Validate repo structure, config files, and scripts
validate:
    ./scripts/validate.sh

# Run the benchmark against a single target
bench target:
    ./benchmark/runner.sh {{target}}

# Run the benchmark against all targets
bench-all *args:
    ./benchmark/runner.sh --all {{args}}

# Compare all benchmark results against the baseline
bench-compare:
    ./benchmark/runner.sh --all --compare

# Lint all shell scripts
lint:
    shellcheck scripts/*.sh benchmark/*.sh hooks/*.sh tests/*.sh tests/fakes/*.sh

# Format all shell scripts and Go sources in place
fmt:
    shfmt -w -i 4 -ci scripts/*.sh benchmark/*.sh hooks/*.sh tests/*.sh tests/fakes/*.sh
    gofmt -w ./cmd ./internal

# Check formatting without writing
fmt-check:
    shfmt -d -i 4 -ci scripts/*.sh benchmark/*.sh hooks/*.sh tests/*.sh tests/fakes/*.sh
    test -z "$(gofmt -l ./cmd ./internal)"

# Serve the GitHub Pages site locally for review
preview:
    cd docs && python3 -m http.server 8080

# Sync beads status into GitHub Project v2 #5 (manual/debug run)
sync-github:
    python3 scripts/sync-beads-github.py

# Lint, format-check, validate, and run contract tests
check: lint fmt-check proto-lint proto-breaking vet test-go validate test-fix-gate test-adjudication test-refutation test-stop-hook test-contracts test-collector test-orchestrator test-adapter-seam test-confinement test-fix

# A role runs inside a Seatbelt profile, and macOS refuses a nested
# sandbox_apply as soon as the outer profile contains one deny rule, which is
# what a boundary is made of. So the checks that create sandboxes cannot run
# from inside one: test-confinement, and the internal/confine tests within
# test-go. Everything they cover still runs in CI, unconfined.
#
# Tell the engine what this leaves out, so the record does not read as a full
# gate:
#
#   --verify-command='just check-confined' \
#     --verify-excludes='the confinement suite, which cannot create a sandbox inside one'

# The gate to use when Grimes is reviewing this repository
check-confined: lint fmt-check proto-lint proto-breaking vet test-go-unconfined validate test-fix-gate test-adjudication test-refutation test-stop-hook test-contracts test-collector test-orchestrator test-adapter-seam test-fix

# Run the Go tests a confined gate can reach
test-go-unconfined:
    go test $(go list ./... | grep -v /internal/confine)

# Vet the Go packages
#
# Both platforms, because the confinement backends are behind build tags: a
# darwin-only vet cannot see a linux file at all, and the first thing to notice
# would be CI.
vet:
    go vet ./...
    GOOS=linux go build ./...
    GOOS=darwin go build ./...

# Run the Go unit tests
test-go:
    go test ./...

# Run the fix-gate contract tests
test-fix-gate:
    ./tests/test-fix-gate.sh

# Run the independent adjudication contract tests
test-adjudication:
    ./tests/test-adjudication.sh

# Run the refutation contract tests
test-refutation:
    ./tests/test-refutation.sh

# Run the stop hook contract tests
test-stop-hook:
    ./tests/test-stop-hook.sh

# Run the execution-boundary tests
test-confinement:
    ./tests/test-confinement.sh

# Run the fix-mode supervisor tests
test-fix:
    ./tests/test-fix.sh

# Regenerate protobuf bindings from the contract
gen:
    buf generate

# Lint the protobuf contract
proto-lint:
    buf lint

# Refuse a wire-incompatible change to the contract.
#
# The baseline lives outside proto/ because that path is the buf module: an
# artifact inside it is read as part of the module by any tool that globs the
# directory. The baseline is the last wire state deliberately accepted. Breaking it is
# sometimes right, and `just proto-baseline` is how that is said out loud: the
# regenerated baseline lands in the same commit as the break and the schema
# major bump, so the acknowledgement is reviewable rather than silent.
proto-breaking:
    buf breaking --against .buf/baseline.binpb

# Accept the current contract as the baseline. See proto-breaking.
proto-baseline:
    buf build -o .buf/baseline.binpb

# Run the adapter/engine seam tests
test-adapter-seam:
    ./tests/test-adapter-seam.sh

# Run the orchestrator tests
# Collection resolves any target kind against real artifacts.
test-collector:
    ./tests/test-collector.sh

test-orchestrator:
    ./tests/test-orchestrator.sh

# Run the contract and ledger tests
test-contracts:
    ./tests/test-contracts.sh

# Build the contract codec
build:
    go build -o bin/grimes-contract ./cmd/grimes-contract
    go build -o bin/grimes ./cmd/grimes
