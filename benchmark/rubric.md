# Frank Grimes Benchmark Rubric

This rubric defines what "good" looks like for a Grimes Grind output. It is the standard against which all benchmark runs are scored.

## Dimensions

### 1. Evidence Quality (0-5)

Does the grind present specific, verifiable evidence for each issue?

| Score | Criteria |
|-------|----------|
| 0 | No evidence presented; issues are vague assertions |
| 1 | Evidence is generic ("this could be a problem") without specific code paths |
| 2 | Some evidence present but incomplete; missing line numbers, file paths, or concrete examples |
| 3 | Evidence is specific and verifiable; code paths shown; issues are grounded in the target |
| 4 | Evidence is thorough; includes context around the flaw; explains why it matters |
| 5 | Evidence is exemplary; every issue has concrete code paths, scenarios, and clear linkage to the risk |

**What to look for:**
- Specific file paths and line numbers
- Code snippets showing the problematic pattern
- Clear explanation of why the evidence proves the risk
- No issues without evidence

---

### 2. Category Coverage (0-5)

Does the grind attack across multiple critique categories, or does it focus on one area?

| Score | Criteria |
|-------|----------|
| 0 | Only one category addressed, or no categories clearly identified |
| 1 | Two categories addressed superficially |
| 2 | Three to four categories addressed |
| 3 | Five to seven categories addressed with substance |
| 4 | Eight to twelve categories addressed |
| 5 | Comprehensive coverage across 13+ categories; all major failure modes explored |

**What to look for:**
- Issues spread across different categories (security, reliability, correctness, maintainability, etc.)
- Not all issues clustered in one category
- Categories match the target's actual risk profile (a shell script won't have goroutine leaks, but it might have injection vulnerabilities)

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

Is the final verdict (GREEN/YELLOW/RED) justified by the findings?

| Score | Criteria |
|-------|----------|
| 0 | No verdict, or verdict contradicts the findings |
| 1 | Verdict present but unjustified; GREEN with unmitigated P0 issues |
| 2 | Verdict is plausible but the reasoning is thin |
| 3 | Verdict matches the findings; stop conditions are referenced |
| 4 | Verdict is well-justified; explicitly ties to P0/P1 mitigation status |
| 5 | Verdict is clinically precise; explains exactly which stop conditions are met or not met |

**What to look for:**
- GREEN only when all P0 mitigated or accepted with timeline, P1 have plans, verification exists
- YELLOW when P0 mitigated but P1 weak, verification non-comprehensive
- RED when any P0 lacks mitigation, no verification path, observability insufficient
- Verdict is not GREEN unless the evidence supports it

---

### 5. Report Structure (0-5)

Does the output follow the Grimes Report format?

| Score | Criteria |
|-------|----------|
| 0 | No structured report; output is unstructured text |
| 1 | Some structure but missing key sections |
| 2 | Most sections present but incomplete |
| 3 | All required sections present: verdict, BLUF, top 3 risks, risk register, survived scrutiny, final word |
| 4 | Structure is complete and well-organized; sections are substantive |
| 5 | Structure is exemplary; every section is complete, formatted correctly, and adds value |

**Required sections:**
- Verdict (GREEN/YELLOW/RED)
- BLUF (one concise summary)
- Top 3 Risks (evidence-first)
- Origin Assessment (human-written, AI-generated, or cargo-culted)
- Risk Register (table with ID, Grime ID, Category, Evidence, Risk Statement, Severity, Evidence Status)
- Survived Scrutiny (claims that withstood falsification attempts)
- Grimey's Final Word (one clinical, direct sentence)

---

### 6. Issue Format Compliance (0-5)

Are issues formatted correctly with Grime IDs, evidence, category, severity, likelihood, and blast radius?

| Score | Criteria |
|-------|----------|
| 0 | No consistent issue format |
| 1 | Issues present but missing most required fields |
| 2 | Some fields present; Grime IDs missing or inconsistent |
| 3 | All required fields present for most issues; Grime IDs follow the pattern |
| 4 | Format is consistent across all issues; enhanced prefixes used where appropriate |
| 5 | Format is exemplary; every issue has all fields, enhanced prefixes used correctly, evidence is specific |

**Required fields per issue:**
- Grime ID (grime-[a-z0-9]{3} or enhanced prefix)
- Evidence (specific code path, scenario, or logic flaw)
- Category (from the 23 critique categories)
- Severity (P0/P1/P2/P3)
- Likelihood (High/Medium/Low)
- Blast Radius (what gets affected)
- Description of Risk (impact derived from evidence)

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

### 9. Voice and Tone (0-5)

Does the output sound like Grimey—clinical, direct, unforgiving—without obscuring the instructions?

| Score | Criteria |
|-------|----------|
| 0 | No distinct voice; reads like generic code review |
| 1 | Voice is present but flat; lacks the clinical pessimism |
| 2 | Voice is recognizable but inconsistent; sometimes too soft, sometimes too aggressive |
| 3 | Voice is consistent; clinical, direct, and unforgiving |
| 4 | Voice is strong; the satire is present but never obscures the findings |
| 5 | Voice is exemplary; every section carries the Grimey tone without sacrificing clarity |

**What to look for:**
- Clinical, direct language
- No softening of findings to spare feelings
- Anger at indifference to incompetence, not at incompetence itself
- Satire present but never obscuring instructions, safety constraints, or verification requirements

---

### 10. Origin Assessment (0-5)

Does the grind assess whether the target is human-written, AI-generated, or cargo-culted?

| Score | Criteria |
|-------|----------|
| 0 | No origin assessment |
| 1 | Origin mentioned but not assessed |
| 2 | Origin assessment present but superficial |
| 3 | Origin assessment is present with some evidence |
| 4 | Origin assessment is well-supported; indicators are identified |
| 5 | Origin assessment is clinically precise; specific indicators are cited |

**What to look for:**
- Assessment of whether the target is human-written, AI-generated, or cargo-culted
- Evidence for the assessment (patterns, inconsistencies, hallmarks)
- Not just a checkbox; the assessment informs the critique

---

## Scoring Summary

| Dimension | Max Score |
|-----------|-----------|
| Evidence Quality | 5 |
| Category Coverage | 5 |
| Severity Assessment | 5 |
| Verdict Accuracy | 5 |
| Report Structure | 5 |
| Issue Format Compliance | 5 |
| Fix Quality | 5 |
| Anti-Pattern Avoidance | 5 |
| Voice and Tone | 5 |
| Origin Assessment | 5 |
| **Total** | **50** |

**Normalized score:** (Total / 50) × 100 = Percentage

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
3. Look for patterns: did one version improve evidence quality but degrade voice?
4. The total score is a summary, but dimension-level comparison is where the insights are

A "better" skill is one that improves the total score without degrading any single dimension by more than 1 point. A skill that improves evidence quality by 2 points but drops voice by 2 points is a trade-off that needs evaluation.
