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

# Lint, format-check, validate, and run contract tests
check: lint fmt-check proto-lint proto-breaking vet test-go validate test-fix-gate test-adjudication test-stop-hook test-contracts test-collector test-orchestrator test-adapter-seam

# Vet the Go packages
vet:
    go vet ./...

# Run the Go unit tests
test-go:
    go test ./...

# Run the fix-gate contract tests
test-fix-gate:
    ./tests/test-fix-gate.sh

# Run the independent adjudication contract tests
test-adjudication:
    ./tests/test-adjudication.sh

# Run the stop hook contract tests
test-stop-hook:
    ./tests/test-stop-hook.sh

# Regenerate protobuf bindings from the contract
gen:
    buf generate

# Lint the protobuf contract
proto-lint:
    buf lint

# Refuse a wire-incompatible change to the contract.
#
# The baseline is the last wire state deliberately accepted. Breaking it is
# sometimes right, and `just proto-baseline` is how that is said out loud: the
# regenerated baseline lands in the same commit as the break and the schema
# major bump, so the acknowledgement is reviewable rather than silent.
proto-breaking:
    buf breaking --against proto/baseline.binpb

# Accept the current contract as the baseline. See proto-breaking.
proto-baseline:
    buf build -o proto/baseline.binpb

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
