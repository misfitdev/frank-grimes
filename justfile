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
    shellcheck scripts/*.sh benchmark/*.sh hooks/*.sh tests/*.sh

# Format all shell scripts in place
fmt:
    shfmt -w -i 4 -ci scripts/*.sh benchmark/*.sh hooks/*.sh tests/*.sh

# Check formatting without writing
fmt-check:
    shfmt -d -i 4 -ci scripts/*.sh benchmark/*.sh hooks/*.sh tests/*.sh

# Serve the GitHub Pages site locally for review
preview:
    cd docs && python3 -m http.server 8080

# Lint, format-check, validate, and run contract tests
check: lint fmt-check proto-lint validate test-fix-gate test-adjudication test-stop-hook test-contracts

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

# Run the contract and ledger tests
test-contracts:
    ./tests/test-contracts.sh

# Build the contract codec
build:
    go build -o bin/grimes-contract ./cmd/grimes-contract
