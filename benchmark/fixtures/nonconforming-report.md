# Fixture: a report that fails the evidence protocol

Scoring regression fixture, paired with `conforming-report.md`. This is Grimey
Theater: the shape of a review with none of the discipline. Findings assert
without tiers, the verdict is a colour with no tuple behind it, acquittals name
no probe, and nothing was ground against itself. If this fixture starts scoring
well, the scorer has stopped discriminating.

---

## Grimes Grind Report: benchmark/targets/shell/bad-script.sh

### Verdict: YELLOW

**BLUF:** There are some issues here that might be a problem, but overall the script seems fine for internal use.

### Top 3 Risks

1. The script could be vulnerable to injection if someone passes a bad argument.
2. Error handling might be insufficient in places.
3. There is possibly a race condition somewhere in the temp file handling.

### Origin Assessment

- [x] AI-generated
- [ ] Human-written
- [ ] Cargo-culted

The code has the hallmarks of LLM output: consistent formatting, generic variable names.

### Risk Register

| ID | Category | Risk | Severity |
|----|----------|------|----------|
| 1 | Security | Injection risk in argument handling | P1 |
| 2 | Reliability | Error handling could be better | P2 |
| 3 | Style | Variable naming is inconsistent | P3 |

### Survived Scrutiny

- Argument parsing looks fine.
- The install path appears correct.
- Permissions on the temp file look sound.

### Grimey's Final Word

This is probably fine for now but should be revisited before it goes anywhere near production.
