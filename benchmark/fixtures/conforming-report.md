# Fixture: a report that conforms to the current Grimes Report template

Scoring regression fixture. Every section, field name, and table header below is
copied from the report template in `skills/frank-grimes/SKILL.md`. A conforming
report must score at or near maximum; if this fixture starts scoring low, the
scorer has drifted from the template again.

---

## Grimes Grind Report: benchmark/targets/shell/bad-script.sh

### Verdict

- **Decision:** block
- **Residual risk:** critical
- **Review confidence:** high
- **Review completeness:** limited
- **Derived color:** RED
- **Unmet gates:** decision, residual_risk, review_completeness
- **Independent adjudication:** not available

**BLUF:** The script interpolates unvalidated positional arguments straight into a `rm -rf` and a `curl | sh`, so a caller controls both what gets deleted and what code runs; nothing here is salvageable without a rewrite of the argument handling.

### Review Contract and Routing

- Target and target type: `benchmark/targets/shell/bad-script.sh`; code
- Artifact set: single file, 84 lines, revision `f0c1cab`
- Claims under judgment: the script safely cleans a build directory and installs a pinned toolchain
- Critical invariants: deletion stays inside the build directory; installed code is the pinned release
- Environment: developer workstations and CI runners, invoked from a Makefile with caller-supplied arguments
- In scope: the script and its argument handling
- Out of scope: the Makefile that calls it, not supplied
- Evidence available: the file, `bash -n`, `shellcheck`
- Evidence unobtainable: runtime execution; the sandbox is read-only and the script deletes files
- Unknowns: whether CI passes a constrained argument set

Categories: COR included: deletion path is computed from input. INT included: arguments cross a trust boundary unvalidated. SEC included: remote code execution is reachable. REL included: failure mid-install leaves a partial toolchain. OPS included: no logging of what was deleted. PER excluded: no loop over unbounded input. VER excluded: no tests ship with the target. MNT excluded: a rewrite supersedes style concerns. DEP excluded: the single remote dependency is covered under SEC. HUM excluded: no interactive operator surface.

### Self-Grind Reconciliation

**9 candidates, 6 survived, 3 killed.**

| Killed Candidate | Disproof Probe | Recorded Result |
|------------------|----------------|-----------------|
| `set -u` missing, so unset vars expand empty | `rg -n '^set ' bad-script.sh` | line 3 is `set -euo pipefail`; the accusation was wrong |
| Temp file created world-readable | `rg -n 'mktemp' bad-script.sh` | line 41 passes `-m 600`; control is present |
| Exit code discarded after install | `rg -n 'install_toolchain' bad-script.sh` | line 70 checks `$?` and exits non-zero |

### Terminal Risks

1. `bad-script.sh:22`: `rm -rf "$1"/*` deletes whatever path the caller passes. E2. A caller passing `/` or an empty string escapes the build directory entirely. P0.
2. `bad-script.sh:58`: `curl -sSL "$TOOLCHAIN_URL" | sh` executes the response body with no checksum or signature check. E2. Anyone able to answer that host executes code as the invoking user. P0.
3. `bad-script.sh:31`: the trap that restores the previous directory is installed after the `cd`, so an early failure leaves the caller in the build directory. E1, reproduced with `bash -n` plus a traced dry run. P1.

### Risk Register

| Stable ID | State | Category | Evidence Tier and Record | Violated Invariant | Risk | Sev | Assumption Status | Self-Grind Result |
|-----------|-------|----------|--------------------------|--------------------|------|-----|-------------------|-------------------|
| grime-a1c | open | SEC | E2 `bad-script.sh:22` `rm -rf "$1"/*` | deletion stays inside the build directory | caller-controlled deletion path | P0 | none | probe attempted; no quoting or prefix check found |
| grime-b7f | open | SEC | E2 `bad-script.sh:58` `curl -sSL "$TOOLCHAIN_URL" \| sh` | installed code is the pinned release | remote code execution as invoking user | P0 | none | probe attempted; no checksum anywhere in file |
| grime-c04 | open | REL | E1 `bash -n` clean, traced dry run shows trap ordering | caller's working directory is restored | caller left in a deleted directory | P1 | none | probe attempted; reproduced |
| grime-d92 | open | INT | E2 `bad-script.sh:14` `TOOLCHAIN_URL="$2"` | arguments are validated before use | unvalidated URL reaches the network | P1 | none | probe attempted; no validation between read and use |
| grime-e55 | open | COR | E2 `bad-script.sh:22` unquoted glob after `"$1"` | deletion targets exactly the build directory | dotfiles survive the clean | P2 | none | probe attempted |
| grime-f18 | open | OPS | E3 no logging call in the deletion path | destructive actions are auditable | deletions cannot be reconstructed after the fact | P2 | confirmed | probe attempted; falsifier was a logging call, absent |

### Survived Scrutiny (Earned Acquittals)

| Claim or Invariant | Specific Probe Performed | Recorded Result | Scope of Acquittal |
|--------------------|--------------------------|-----------------|---------------------|
| Script fails fast on error | `rg -n '^set ' bad-script.sh` | line 3 `set -euo pipefail` | error handling only; does not cover the trap ordering defect |
| Temp files are not world-readable | `rg -n 'mktemp' bad-script.sh` | line 41 `mktemp -m 600` | temp file permissions only |
| Install failure surfaces to the caller | `rg -n -A3 'install_toolchain' bad-script.sh` | line 70 checks `$?`, exits non-zero | exit propagation only |

### Not Examined

| Area or Claim | Why Not Examined | Evidence Needed | Completeness Effect |
|---------------|------------------|-----------------|---------------------|
| Runtime behavior of the deletion path | Sandbox is read-only; the probe destroys files | A disposable container | caps review_completeness at limited |
| The Makefile that supplies arguments | Not in the artifact set | The calling Makefile | leaves the INT blast radius unbounded |
| Toolchain host response contents | No network access | An authorized fetch of the URL | SEC severity rests on the pattern, not the payload |

### Appendix: Remaining Findings

| Stable ID | State | Category | Evidence Tier and Record | Violated Invariant | Risk | Sev | Assumption Status | Self-Grind Result |
|-----------|-------|----------|--------------------------|--------------------|------|-----|-------------------|-------------------|
| grime-g23 | open | MNT | E2 `bad-script.sh:9` duplicated path literal | one authoritative build path | edits drift between the two copies | P3 | none | probe attempted |

### Grimey's Final Word

Two P0s that a caller triggers by accident and a third that hands them a shell; this does not get shipped behind a "works on my machine."
