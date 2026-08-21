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
    shellcheck scripts/*.sh benchmark/*.sh hooks/*.sh

# Format all shell scripts in place
fmt:
    shfmt -w -i 4 -ci scripts/*.sh benchmark/*.sh hooks/*.sh

# Check formatting without writing
fmt-check:
    shfmt -d -i 4 -ci scripts/*.sh benchmark/*.sh hooks/*.sh

# Lint, format-check, and validate
check: lint fmt-check validate
