# Frank Grimes Benchmark Framework

The benchmark framework validates that the Grimes Grind skill produces high-quality critiques across different target types. It's designed for A/B testing: swap the skill content, run the benchmark, compare scores.

## Structure

```
benchmark/
├── README.md              # Benchmark documentation
├── rubric.md              # Scoring rubric (the standard)
├── runner.sh              # Executes a grind against a target and scores the output
├── targets/
│   ├── shell/
│   │   ├── bad-script.sh      # A deliberately bad shell script
│   │   └── expected-issues.md # What a good grind should find
│   ├── go/
│   │   ├── bad-service/       # A deliberately bad Go service
│   │   └── expected-issues.md
│   └── web/
│       ├── bad-endpoint/     # A deliberately bad web endpoint
│       └── expected-issues.md
└── results/
    └── .gitkeep              # Results go here (gitignored)
```

## The Rubric

The rubric defines what "good" looks like for a Grimes Grind output. It's the standard against which all grinds are scored. See `rubric.md` for the full rubric.

The fixture harness is intentionally offline. It is useful for controlled A/B checks of evidence discipline, report validity, recall, precision, false positives, and verdict consistency, but it is not proof of production readiness. For current-language, framework, cloud, Terraform/OpenTofu/HCL, or CloudFormation work, add a separate live-research matrix or a frozen research bundle and score citation quality, freshness, and whether the audit missed any material baseline finding.

## Running a Benchmark

```bash
# Run against a specific target
./benchmark/runner.sh ./benchmark/targets/shell/bad-script.sh

# Run against all targets
./benchmark/runner.sh --all

# Compare results (after making skill changes)
./benchmark/runner.sh --all --compare
```

## A/B Testing

1. Run the benchmark against the current skill: `./benchmark/runner.sh --all`
2. Save the results: `cp -r benchmark/results benchmark/results-baseline`
3. Modify `skill/SKILL.md`
4. Run the benchmark again: `./benchmark/runner.sh --all`
5. Compare: `./benchmark/runner.sh --all --compare`

The runner outputs a score for each target and dimension, making it easy to see if your changes improved or degraded the skill.

## Target Design Principles

Each target is deliberately bad in specific ways that the rubric checks for. A good grind should:

1. **Find the obvious flaws** (syntax errors, missing error handling, hardcoded secrets)
2. **Find the subtle flaws** (edge cases, scalability issues, maintainability problems)
3. **Not hallucinate flaws** (every issue should have evidence)
4. **Produce a proper report** (verdict, risk register, survived scrutiny, final word)
5. **Use the correct format** (Grime IDs, evidence-first, severity ratings)

Targets are designed to be small enough to grind quickly but complex enough to exercise multiple critique categories.

## Scoring

Each dimension is scored 0-5:

| Score | Meaning |
|-------|---------|
| 0 | Not present |
| 1 | Present but severely deficient |
| 2 | Present with significant gaps |
| 3 | Present, meets minimum standard |
| 4 | Present, above average |
| 5 | Excellent, exceeds expectations |

Total score is the sum across all dimensions, normalized to a percentage.

Record operational measurements alongside scores: valid-report rate, input/output tokens, cache creation and reads, latency, and cost. A change is an improvement only when it preserves material finding recall and report validity while improving evidence quality/precision or reducing cost; a decline includes missed P0/P1 findings, unsupported findings, malformed reports, or materially worse cost/latency.

## Adding New Targets

To add a new target type:

1. Create `benchmark/targets/<type>/`
2. Add a deliberately bad example in that target type
3. Document what flaws the target contains in `expected-issues.md`
4. Update `runner.sh` to handle the new target type if needed
5. Run the benchmark to verify the target exercises the rubric appropriately

The target should be bad enough that a good grind finds at least 5-10 issues across multiple categories, but not so complex that grinding takes more than a few minutes.
