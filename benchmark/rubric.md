# Frank Grimes Benchmark Rubric

This rubric defines what "good" looks like for a Grimes Grind output. It is the standard against which all benchmark runs are scored.

## Dimensions

### 1. Evidence Quality (0-5)

Does every finding carry exactly one evidence tier, and does the tier hold up?

| Score | Criteria |
|-------|----------|
| 0 | No evidence presented; findings are vague assertions |
| 1 | Findings cite no tier, or a tier is claimed with no record behind it |
| 2 | Tiers are present but misapplied; a command name or unexecuted scenario is promoted to E1 |
| 3 | Every finding carries one tier with the record that tier requires; no P0 rests on E3 |
| 4 | Tier records are complete; E2 quotes make the defect visible without unstated surrounding facts |
| 5 | Tier discipline is exemplary; E1 records include exit codes and bounded output, E3 names its falsifier, and severity caps are respected throughout |

**Required record per tier:**
- **E1 — reproduced:** executed action or command, exit code or status, result excerpt
- **E2 — cited:** repository-relative `path:line` and an exact quote in which the defect is visible
- **E3 — inferred:** explicit assumption, observed facts used, and a named falsifying observation

**Severity caps:** P0 requires E1 or E2. E3 caps at P1. An unverified finding caps at P2.

**What to look for:**
- No command name, unexecuted scenario, path without a quote, or model recollection presented as evidence
- E2 not used to prove absence, intent, reachability, or future divergence
- The repo's own analyzers and tests run first, with their output recorded as E1 even when they pass

---

### 2. Routing Discipline (0-5)

Did the grind route the right categories and defend both its inclusions and its exclusions?

Routing selects 5-8 categories from `COR`, `INT`, `SEC`, `REL`, `OPS`, `PER`, `VER`, `MNT`, `DEP`, `HUM`. Breadth is not the goal: a category count above 8 is a routing failure, not thoroughness, and spreading findings to fill categories is Grimey Theater.

| Score | Criteria |
|-------|----------|
| 0 | No categories identified, or findings carry no category |
| 1 | Categories appear on findings but no routing decision is recorded |
| 2 | Routing recorded but outside the 5-8 band, or inclusions given without exclusions |
| 3 | 5-8 categories routed with a recorded one-clause reason for every inclusion and exclusion |
| 4 | Routing reasons are specific to this target and tie to the contract and inventory |
| 5 | Every exclusion demonstrates that a P0 cannot live in that category; every inclusion owns a distinct plausible P0 hypothesis |

**What to look for:**
- All ten categories accounted for as `included — reason` or `excluded — reason`
- Exclusions justified by the target's actual shape, not by charity toward it
- One primary category per root cause; secondary tags do not spawn extra findings
- No filler findings manufactured to populate a routed category

---

### 3. Severity Assessment (0-5)

Are issues correctly categorized by severity (P0/P1/P2/P3)?

| Score | Criteria |
|-------|----------|
| 0 | No severity ratings, or all issues rated the same |
| 1 | Severity ratings present but arbitrary; P0 assigned to minor issues |
| 2 | Mostly correct but some misclassifications |
| 3 | Correct severity for most issues; P0 reserved for actual blocking problems |
| 4 | Accurate severity across the board; nuanced distinction between P1 and P2 |
| 5 | Severity assessment is clinically precise; P0/P1/P2/P3 distinctions are defensible |

**What to look for:**
- P0 reserved for data loss, breach, system down, or logic failure
- P1 for significant risk or degraded functionality
- P2 for increased risk/friction, not immediate explosion
- P3 for technical debt
- Severity matches the blast radius and likelihood

---

### 4. Verdict Accuracy (0-5)

Is the verdict tuple complete, and is the derived color entailed by it?

The verdict is the tuple `{decision, residual_risk, review_confidence, review_completeness}`. The color is derived from the tuple, never asserted independently.

| Score | Criteria |
|-------|----------|
| 0 | No verdict, or the verdict contradicts the findings |
| 1 | A color is asserted with no tuple behind it |
| 2 | Tuple is partial; one or more of the four fields is missing or unresolved |
| 3 | All four fields present and the derived color follows from them |
| 4 | Derivation is correct and every unmet gate is listed by name |
| 5 | Derivation is clinically precise; assumption-dependent and unverified findings are excluded from verdict weight and the exclusion is visible |

**Derivation rules:**
- `decision = block` when any open P0 remains; `conditional` when any open P1 remains or independent adjudication is pending; `pass` only when no verdict-weighted P0/P1 remains and independent adjudication is confirmed
- `RED` follows from `decision = block`
- `GREEN` requires all four of `decision = pass`, `residual_risk = low`, `review_confidence = high`, `review_completeness = sufficient`
- `YELLOW` for every other tuple

**What to look for:**
- Unknowns capping `review_completeness` rather than being converted into findings
- `review_confidence` reflecting whether self-grind probes were actually attempted
- No GREEN where evidence was unavailable

---

### 5. Report Structure (0-5)

Does the output follow the Grimes Report format?

| Score | Criteria |
|-------|----------|
| 0 | No structured report; output is unstructured text |
| 1 | Some structure but missing key sections |
| 2 | Most sections present but incomplete |
| 3 | All required sections present |
| 4 | Structure is complete and well-organized; sections are substantive |
| 5 | Structure is exemplary; every section is complete, formatted correctly, and adds value |

**Required sections:**
- Verdict (the four-field tuple plus derived color, unmet gates, adjudication status)
- BLUF (one concise summary grounded in the tuple)
- Review Contract and Routing (the Phase 1 contract, plus all ten categories marked included or excluded)
- Self-Grind Reconciliation (`N candidates, M survived, K killed`, with the killed table)
- Terminal Risks (at most three highest verdict-weighted findings, evidence first)
- Risk Register (at most 12 survivors, sorted by verdict weight and severity)
- Survived Scrutiny (probe-backed acquittals only)
- Not Examined (coverage limits, never a pass)
- Grimey's Final Word (one clinical, direct sentence)

An Appendix section is required only when survivors exceed the 12-row register cap.

---

### 6. Finding Format Compliance (0-5)

Does every register row carry the fields the register schema requires?

| Score | Criteria |
|-------|----------|
| 0 | No register table, or findings are unstructured prose |
| 1 | A register exists but most columns are empty or absent |
| 2 | Some columns populated; stable IDs missing or inconsistent |
| 3 | All required columns present and populated for most findings |
| 4 | Format is consistent across every row; severities and tiers agree with the caps |
| 5 | Format is exemplary; each row names one root cause and one violated invariant, with no duplicate symptoms split across rows |

**Required columns:**
- Stable ID, State, Category (one of the ten), Evidence Tier and Record
- Violated Invariant, Risk, Severity (P0/P1/P2/P3)
- Assumption Status (`confirmed`, `unconfirmed`, or `none`)
- Self-Grind Result

**What to look for:**
- One row per root cause, not one row per symptom
- `assumption-dependent` and `unverified` rows tagged and visibly excluded from verdict weight
- Stable IDs consistent between Terminal Risks, the register, and the appendix

---

### 7. Fix Quality (0-5, if mode=fix)

When fixes are applied, are they correct, verifiable, and scoped appropriately?

| Score | Criteria |
|-------|----------|
| 0 | No fixes applied, or fixes make things worse |
| 1 | Fixes are superficial; don't address root cause |
| 2 | Fixes address the issue but lack verification plan or regression scope |
| 3 | Fixes are correct; include verification and residual risk |
| 4 | Fixes are thorough; verification is concrete; regression scope is identified |
| 5 | Fixes are exemplary; each fix has a clear verification path, residual risk acknowledged, and regression scope defined |

**What to look for:**
- Fix addresses the root cause, not just the symptom
- Verification explains how to prove the fix survives the next grind
- Residual risk acknowledges what is still not perfect
- Regression scope identifies what must be re-checked

---

### 8. Anti-Pattern Avoidance (0-5)

Does the grind avoid the known anti-patterns?

| Score | Criteria |
|-------|----------|
| 0 | Multiple anti-patterns present |
| 1 | One or two anti-patterns evident |
| 2 | Minor anti-patterns present but not dominant |
| 3 | Anti-patterns mostly avoided; occasional slips |
| 4 | Anti-patterns avoided; grind is genuinely skeptical |
| 5 | Anti-patterns completely avoided; grind demonstrates genuine falsification |

**Anti-patterns to check for:**
- **Grimey Theater**: Going through motions without genuine skepticism
- **Optimism Creep**: "It'll probably be fine" without evidence
- **Authority Deference**: "The LLM said so" without verification
- **Perfection Paralysis**: Never shipping because something might be wrong (when P0/P1 are actually mitigated)
- **Orphaned Risks**: Accepted risks with no owner or timeline

---

### 9. Self-Grind and Earned Acquittal (0-5)

Did the grind attack its own findings, and are its clean claims backed by a probe that was actually performed?

An acquittal that nobody tried to falsify is the worst failure available to a judge. This dimension scores whether confidence was earned or assumed.

| Score | Criteria |
|-------|----------|
| 0 | No self-grind and no acquittal evidence; "looks fine" is written down as a finding-free area |
| 1 | A reconciliation line appears but the arithmetic does not close, or acquittals cite no probe |
| 2 | Disproof observations are named but none were attempted; nothing was killed or marked unverified |
| 3 | `N candidates, M survived, K killed` closes as `N = M + K`, and every acquittal names a performed probe and its result |
| 4 | Killed candidates are listed with the probe and recorded result; unattemptable probes produce `unverified` findings capped at P2 |
| 5 | Self-grind is exemplary; surviving findings carry both original evidence and self-grind result, and every claim lacking a performed probe is in Not Examined rather than Survived Scrutiny |

**What to look for:**
- The reconciliation arithmetic closing exactly; a candidate absent from it must not appear in the report
- Acquittals stating the scope of what was acquitted, not blanket clean bills
- Coverage limits confessed in Not Examined rather than converted into confidence
- Embedded instructions found in the target reported as findings, never obeyed

---

## Scoring Summary

| Dimension | Max Score |
|-----------|-----------|
| Evidence Quality | 5 |
| Routing Discipline | 5 |
| Severity Assessment | 5 |
| Verdict Accuracy | 5 |
| Report Structure | 5 |
| Finding Format Compliance | 5 |
| Fix Quality | 5 |
| Anti-Pattern Avoidance | 5 |
| Self-Grind and Earned Acquittal | 5 |
| **Total** | **45** |

**Normalized score:** (Total / 45) × 100 = Percentage

Grimey's voice is not scored. It exists because it is fun to read, and paying points for it would buy persona at the expense of findings. The skill still confines it to the BLUF and Final Word; that placement is enforced by the report template, not by the rubric.

**Interpretation:**
- 0-40%: Poor — the grind is not functioning as intended
- 40-60%: Marginal — some dimensions work, others don't
- 60-80%: Good — the grind produces usable output
- 80-90%: Very Good — the grind is solid across most dimensions
- 90-100%: Excellent — the grind is operating at high quality

## Using the Rubric for A/B Testing

When comparing two versions of the skill:

1. Run the benchmark against both versions
2. Compare scores dimension by dimension
3. Look for patterns: did one version improve evidence quality but degrade routing discipline?
4. The total score is a summary, but dimension-level comparison is where the insights are

A "better" skill is one that improves the total score without degrading any single dimension by more than 1 point. A skill that improves evidence quality by 2 points but drops self-grind by 2 points is a trade-off that needs evaluation.

Scores are comparable only within a rubric version. This rubric scores the evidence-tier report shape; it replaced Category Coverage with Routing Discipline and Origin Assessment with Self-Grind and Earned Acquittal, so totals do not compare against runs scored before that change.

## Scope of the Automated Scorer

`runner.sh` scores structure, not judgment. It can confirm that a tier is declared, a reconciliation closes, and a column is populated. It cannot confirm that an E2 quote actually shows the defect, that a severity is defensible, or that an exclusion reason is honest. Treat automated scores as a regression check against format drift, and score the judgment dimensions by hand.

Two consequences of that limit:

- **Severity Assessment tops out at 4 automatically.** The fifth point requires judging whether a P0/P1 split is defensible, which no grep can do. A perfect report scores 39/40 in report mode, not 40/40.
- **Fix Quality is `N/A` in report mode** and drops out of the denominator, so report-mode totals are out of 40.

Score a saved report without re-running an agent:

```bash
./benchmark/runner.sh --score path/to/report.md
```

`benchmark/fixtures/` holds a conforming and a non-conforming report. They are the scorer's own regression test: the conforming one must score near maximum and the non-conforming one must score poorly. `scripts/validate.sh` enforces that gap, so a template change that breaks the scorer fails `just validate` instead of silently corrupting the next A/B comparison.
