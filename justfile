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
check: lint fmt-check validate test-fix-gate test-adjudication

# Run the fix-gate contract tests
test-fix-gate:
    ./tests/test-fix-gate.sh

# Run the independent adjudication contract tests
test-adjudication:
    ./tests/test-adjudication.sh
