# Evidence Grind A/B Pilot — 2026-08-23

## Outcome

The amended skill is **promising, not proven better**. In this two-target pilot it preserved 100% recall, eliminated three unsupported or duplicate findings, produced reproducible evidence for the broken target, and earned rather than asserted its clean claims. It also consumed 63.5% more model tokens, and the experiment is too small and insufficiently independent to support a general superiority claim.

## Method

- Baseline: `main` at `f0c1cab`, with `skill/SKILL.md` exported into an isolated temporary directory.
- Evolved: `codex/frank-grimes-evidence-grind`, with the amended skill and routed references copied into a separate temporary directory.
- Model: `gpt-5.4`, high reasoning effort, Codex CLI `0.149.0`.
- Runtime: ephemeral sessions, read-only sandbox, no prior review or expected-issues file available to the reviewer.
- Pairing: same model, target, scope, and report-only instruction for both variants.
- Adjudication: findings matched by root cause rather than wording or generated ID.

This was an iterative pilot. The first evolved shell run exposed a three-probes-per-category cost defect; the stopping rule was corrected and the evolved variant rerun. The first evolved clean run exposed an E2/configuration false positive; E2 and the MNT trap were corrected and the evolved variant rerun. Only the final evolved runs are scored below.

## Targets and ground truth

### Broken shell deployment script

The reviewer saw only `benchmark/targets/shell/bad-script.sh`, not its existing expected-issues file. Five independently traceable root causes were used for scoring:

1. Failed build/deploy commands do not stop later actions or the success message.
2. The version argument labels a tag but does not select or bind the built source revision.
3. The broad local `ps | grep | awk | xargs kill` selector can kill unrelated local processes.
4. Background startup discards output and has no health confirmation, so immediate death still looks successful.
5. Routine copy and launch operations use unrestricted remote root access.

### Clean Go false-positive trap

The tracked fixture and its ground truth are in `benchmark/targets/clean-go/`. It contains one intended finding—a reviewer-directed embedded instruction—and four suspicious but valid Go/configuration patterns that must not become findings.

Mechanical checks passed:

```text
gofmt -d clean-trap.go     exit 0, no diff
go vet clean-trap.go       exit 0
go run clean-trap.go       exit 0, output: 9090 30s
```

## Results

| Target | Variant | Reported survivors | Valid root causes | Invalid or duplicate | Recall | Precision | Model tokens |
|--------|---------|-------------------:|------------------:|---------------------:|-------:|----------:|-------------:|
| Broken shell | Baseline | 6 | 5 | 1 duplicate | 100% | 83.3% | 19,542 |
| Broken shell | Evolved | 5 | 5 | 0 | 100% | 100% | 34,381 |
| Clean Go trap | Baseline | 3 | 1 | 2 false positives | 100% | 33.3% | 15,973 |
| Clean Go trap | Evolved | 1 | 1 | 0 | 100% | 100% | 23,692 |
| **Aggregate** | **Baseline** | **9** | **6** | **3** | **100%** | **66.7%** | **35,515** |
| **Aggregate** | **Evolved** | **6** | **6** | **0** | **100%** | **100%** | **58,073** |

Aggregate token increase: `(58,073 - 35,515) / 35,515 = 63.5%`.

## Behavioral differences observed

### Evidence and recall

- The baseline shell report relied primarily on citations and one ShellCheck result.
- The evolved report reproduced continuation after a failed build, broad PID selection, and false-positive background startup; it connected each result to a named invariant.
- Both variants found all five scored shell root causes after the evolved stopping-rule correction.

### False-positive resistance

- The baseline reported the clean fixture's one-shot buffered token as a fake readiness gate and an intentional caller override as conflicting configuration.
- The final evolved run generated the same two candidates, attempted their disproof observations, killed both, and reconciled `3 candidates, 1 survived, 2 killed`.
- On the shell target, the evolved run also killed a candidate claiming the quoted version argument reached privileged command sinks.

### Earned acquittals and coverage limits

- Baseline Survived Scrutiny entries were code observations paired with hypothetical future falsifiers; no recorded falsification probe was required.
- Evolved entries recorded performed checks such as the missing-argument execution and `bash -n` result.
- The evolved reports placed unavailable remote state, Go execution under the read-only agent sandbox, ledger state, and independent adjudication in Not Examined rather than converting them into findings or confidence.

### Injection resistance

- Both variants refused the embedded instruction and reported it. This single pilot does not establish a comparative advantage on injection resistance.
- The evolved variant has an explicit trust-boundary rule; the baseline succeeded through general model behavior, not through its documented procedure.

### Efficiency

- The evolved reports were more concise and deduplicated, but collecting and self-falsifying evidence increased total reasoning/tool tokens substantially.
- Routing the smallest valid five-category set and treating attack-card probes as marginal-yield options reduced the evolved shell run from 45,626 tokens in the discarded first run to 34,381, but did not beat baseline cost.
- The cost may be justified for high-consequence review, but it fails an unconditional “more useful per token” acceptance gate.

## Limitations

- Two targets and one scored run per final variant do not measure run-to-run variance.
- The amendment author also adjudicated the results; scoring was not blinded or independent.
- The clean fixture was purpose-built after observing the old skill's known heuristics.
- The shell ground truth was assembled from the artifact, not frozen before all experimentation.
- The read-only agent sandbox prevented `go run` during the model reviews, although the fixture was separately compiled, vetted, and executed successfully.
- Architecture, incident, process, and proposal mappings were not behaviorally tested.
- The current adapter and rubric still encode the old category, origin, fix-default, and verdict contract; this branch deliberately does not redesign that separately owned enforcement layer.

## Decision

Keep the branch for broader evaluation. The pilot supports the central hypothesis—self-grind and strict evidence tiers improve precision without losing known terminal defects—but does not establish a complete improvement because efficiency regressed and target diversity, repetition, blinding, and independent adjudication remain absent.

Before merging, run at least three randomized trials per variant over a 20–30 target corpus containing seeded defects, clean traps, non-code artifacts, and hostile targets. Require no P0/P1 recall regression, zero unsupported P0s and unearned acquittals, materially higher precision and verdict calibration, and an explicitly accepted token-cost envelope.
