# Frank Grimes

[![Build](https://github.com/misfitdev/frank-grimes/actions/workflows/release.yml/badge.svg?job=Lint%2C+format%2C+and+test)](https://github.com/misfitdev/frank-grimes/actions/workflows/release.yml)
[![SLSA 3](https://slsa.dev/images/gh-badge-level3.svg)](https://slsa.dev)

<img src="docs/img/grimey.png" alt="Grimey logo" width="25%">

> "I've had to work hard every day of my life, and what do I have to show for it? This briefcase, and this haircut."
> Frank Grimes

A pessimistic iteration loop for systematically destroying, rebuilding, and hardening ideas. Named after Frank Grimes ("Grimey") from The Simpsons, the only character who actually *analyzed* what was wrong and refused to let it slide.

## Philosophy

**Everything is crap until proven otherwise.**

The Grimes Grind assumes your idea, code, plan, or design is:
- LLM slop
- Unreliable
- Insecure
- Poorly planned
- Not production-ready
- Unmaintainable

The burden of proof sits with the target, never the reviewer. A clean verdict is earned by surviving probes that were actually run, not granted because nothing turned up.

## What It Does

Frank Grimes is a **Disciplined Falsification Review** process. It routes 5-8 categories from ten, attacks each with the cheapest decisive probes, grinds its own findings to kill false positives, and produces an evidence-first report with a derived verdict.

The core deliverable is a **Grimes Report** that tells you:
- What's wrong (with evidence)
- How wrong it is (severity, likelihood, blast radius)
- What survived scrutiny (claims that withstood falsification)
- What the verdict is (GREEN/YELLOW/RED)

## Installation

Frank Grimes is provider-neutral. The core skill (`skills/frank-grimes/SKILL.md`) and stop hook (`hooks/stop.sh`) work with any agent that can load markdown skills and execute bash scripts. Providers with a plugin system can install the whole thing in one step; the rest load the skill directly.

`jq` is required for the stop hook in every setup below.

### Quick Start (Any Provider)

1. Load the skill: `skills/frank-grimes/SKILL.md`
2. Ensure `jq` is installed
3. Invoke the grind on your target

### Provider-Specific Setup

#### Claude Code

Install as a plugin. This repository is also its own marketplace, so it ships the slash commands and the stop hook alongside the skill:

```
/plugin marketplace add misfitdev/frank-grimes
/plugin install frank-grimes@misfitdev
```

Then invoke with `/frank-grimes:grind <target>`. Plugin components are namespaced, which is where the `frank-grimes:` prefix comes from.

To load only the skill, without the commands or the loop hook, symlink it instead and invoke with `/grind`:

```bash
mkdir -p .claude/skills
ln -s ../../skills/frank-grimes .claude/skills/frank-grimes
```

#### Codex

Install as a plugin from the plugin directory, or point Codex at this repository as a local marketplace source. Skill-only setup:

```bash
mkdir -p .agents/skills
ln -s ../../skills/frank-grimes .agents/skills/frank-grimes
```

Invoke with `$frank-grimes`, or let the description match your request.

#### OpenCode

OpenCode has no plugin format for bundling a skill, so symlink it:

```bash
mkdir -p .opencode/skills
ln -s ../../skills/frank-grimes .opencode/skills/frank-grimes
```

OpenCode reads `.opencode/skills/`, `.claude/skills/`, and `.agents/skills/`. Any of these works.

#### Other Providers

Any agent that supports markdown skills and bash hooks can use Frank Grimes:

1. Load `skills/frank-grimes/SKILL.md` as a skill or system prompt
2. Configure your agent's stop hook to call `hooks/stop.sh`
3. Ensure `jq` is installed

See `adapters/README.md` for detailed integration instructions.

## Running a Grind

### Basic Usage

```
Load the Frank Grimes skill and run a grind on your target.
```

The skill will guide you through:
1. **Scope**: What are you reviewing? (file, directory, recent changes, description)
2. **Routing**: Which categories the target's shape actually warrants (the skill decides, and records why for all ten)
3. **Mode**: Report only (default), or fix behind a verification gate

### Command-Line Options

When your provider supports arguments, you can skip the interactive prompts:

| Option | Description |
|--------|-------------|
| `target` | What to grind: a file, a directory, `.` for the whole repository, a revision range such as `HEAD^..HEAD`, a description, or "this". Written last: it ends the options |
| `--categories COR,SEC,REL` | Restrict routing to these canonical categories |
| `--mode report\|fix` | `report` documents only and edits nothing (default); `fix` applies fixes, then verifies |
| `--verify-command <cmd>` | Gate run once over a fix batch; without a usable gate nothing is committed |
| `--commit` | Authorizes one commit of the verified batch; requires `--mode fix` and a passing gate |
| `--repository-check` | Authorizes running the repository's own `check` recipe as the gate |
| `--adjudicator-command` | A command that performs an independent review; repeatable for a panel of reviewers |
| `--max-iterations N` | Maximum iterations (default: 5) |
| `--auto-loop` | Continue while iterations still change the verdict |
| `--research online\|offline\|frozen:<path>` | Use bounded current-landscape research, disable network research, or load a pinned research bundle |
| `--sandbox-command <cmd>` | Confine each role with your own wrapper instead of the built-in mechanism |
| `--unsafe` | Run the roles unconfined; recorded in the result and caps confidence |

### Fix Mode

`--mode fix` reviews and repairs in one pass, inside a git worktree the engine creates off HEAD. Your own working tree is never written, and it has to be clean to start: a worktree off HEAD would otherwise review something other than what you are looking at.

A finding whose file the batch touched is recorded as `fixed`. It becomes `verified` only when the gate passed after the edit. The gate is `--verify-command`, or — with `--repository-check` — a `check` recipe in a checked-in `justfile` or `Makefile`. That second one is a command from inside the target, so the flag is you saying you have read it; without the flag it is found and not run. A gate that fails or cannot run verifies nothing, and the worktree is left on disk with its path in the run record so you can read or cherry-pick it.

The gate runs inside the same boundary the fixer does: it may build in the worktree, and it cannot reach your working tree, the review's own record, or the repository's history. Under `--unsafe` there is no boundary, for the gate as for every role, and a check recipe can reach all three. A check that stages or commits is refused — that history belongs to the supervisor and to the one commit it is authorized to make.

`--commit` is authorized separately from `--mode fix`, and makes at most one commit per verified batch, on the worktree's own branch, naming the findings it closes. A batch that edited outside the reviewed scope is refused whole.

### Independent Review

A pass needs a second review that reached the same conclusion without seeing the first. `--adjudicator-command` is that reviewer, and it is repeatable: each occurrence is another reviewer, and the `--adjudicator-arg` flags that follow it belong to it.

```bash
--adjudicator-command="claude" --adjudicator-arg="-p" --adjudicator-arg="<prompt>" \
--adjudicator-command="codex"  --adjudicator-arg="exec" --adjudicator-arg="<prompt>" \
--adjudicator-fresh
```

Asking for several is asking for all of them. The run records how many were asked for and how many answered, and a reviewer that did not answer leaves the adjudication gate unmet rather than quietly shrinking the panel. The strictest verdict among them stands.

The engine counts reviewers; it does not classify them. What makes two reviewers independent is that you chose two different commands — there is no table of provider names here, and nothing to configure to use one model rather than another.

### Confinement

Every role runs inside a boundary the engine builds from the run itself: it may not write anywhere in the review directory except its own scratch path, and it may not read the ledger, the recorded verdict, or a copy staged for another role. Everything else, including `$HOME` and the network, is left alone — this bounds what a role can do to the review, not what it can do to your machine.

The built-in mechanism is `sandbox-exec` on macOS and `bubblewrap` on Linux, and it needs no configuration. Elsewhere, supply one with `--sandbox-command` (`srt`, `firejail`, a container runner) or waive it with `--unsafe`. Whatever is in force is proved before the first role spawns: a canary attempts exactly the reads and writes the boundary forbids, and a mechanism that lets either through refuses the run rather than reporting findings gathered under a boundary that was not there.


### Current-Landscape Research

Audits of current language, framework, cloud-provider, service, or IaC behavior should use `--research online` when the provider exposes approved web-search and fetch tools. This is especially important for cloud architecture, Terraform/OpenTofu/HCL, and CloudFormation, where defaults, support windows, provider behavior, and security guidance change.

Research is bounded and evidence-led: official documentation and security advisories come first, every source is recorded with its URL, version/date, retrieval time, hash or stable identifier, and exact excerpt, and source material is treated as untrusted evidence rather than instructions. Research can strengthen a hypothesis, but target evidence is still required to establish a defect. Use `--research frozen:<path>` for reproducible repeated or zero-knowledge loops; use `offline` when network access is unavailable and record the resulting completeness limitation.

### Examples

```
# Grind a specific file
/frank-grimes:grind ./src/auth.py

# Grind the whole repository, report only
/frank-grimes:grind --mode report .

# Grind what the last commit changed
/frank-grimes:grind HEAD^..HEAD

# Grind what a branch contributed since it diverged
/frank-grimes:grind main...topic

# Grind with auto-loop enabled
/frank-grimes:grind ./src/api --auto-loop

# Grind an architecture proposal
/frank-grimes:grind "The proposal to use MongoDB for our financial transaction system"
```

### Reviewing a range of commits

A target may be a revision range instead of a path. Both of git's spellings work:
`A..B` is what B has that A does not, and `A...B` is what B has since the two
diverged.

The range decides **which files** are under review. The bytes are the ones in the
tree, not the versions at either end of the range, because a fix run edits the
tree and a gate runs over it. Two consequences worth knowing:

- The inventory is the files the range changed, so coverage is measured against
  what the review was asked about. A path target over the same repository counts
  every file in it, which is why a review of recent changes used to cap at
  limited completeness.
- A file the range deleted is not a unit. There are no bytes to review and no
  line for a finding to anchor to. Nor is a file that something later removed.

The run record names the commits the range resolved to, not the spelling. A
review of `HEAD^..HEAD` is a review of two particular commits, and the record has
to say which.

A scope that is both a path on disk and a valid range is refused rather than
guessed at. Write `./that-path` for the path.

## The Grimes Grind Process

The methodology is defined in [`skills/frank-grimes/SKILL.md`](skills/frank-grimes/SKILL.md), which is the only normative source. This is a summary; where the two differ, the skill is correct.

A grind absorbs the target and writes a review contract, routes 5-8 categories from the ten canonical ones, attacks each with the cheapest decisive probes, then grinds its own findings to kill the false positives before reporting. Every finding carries one evidence tier:

| Tier | Means | Requires |
|------|-------|----------|
| **E1** | reproduced | an executed action, its exit status, and the result |
| **E2** | cited | `path:line` and a quote in which the defect is visible |
| **E3** | inferred | a stated assumption and a named falsifier |

P0 requires E1 or E2. E3 caps at P1. A finding whose falsifier could not be attempted caps at P2.

The verdict is a tuple, `{decision, residual_risk, review_confidence, review_completeness}`, and the colour is derived from it, never asserted. GREEN additionally requires an independent adjudicator, running in a context that never saw the first report, to reach the same conclusion. Where no adjudicator is available, a grind caps at YELLOW.

Fixing is opt-in and reporting is the default. A commit requires fix mode, explicit `--commit`, and a verification gate that exited zero: all three.

## The Grimes Report

Every grind produces a structured report. The full template lives in the skill; this is its shape:

```markdown
## Grimes Grind Report: [Subject]

### Verdict

- **Decision:** block | conditional | pass
- **Residual risk:** critical | high | moderate | low | unknown
- **Review confidence:** high | medium | low
- **Review completeness:** sufficient | limited | inconclusive
- **Derived color:** RED | YELLOW | GREEN
- **Unmet gates:** [exact gate names, or `none`]
- **Independent adjudication:** confirmed | pending | not available

**BLUF:** [One concise, direct summary grounded in the tuple.]

### Review Contract and Routing
### Self-Grind Reconciliation      <- N candidates, M survived, K killed
### Terminal Risks                 <- at most three, evidence first
### Risk Register                  <- at most 12 survivors, stable IDs
### Survived Scrutiny              <- probe-backed acquittals only
### Not Examined                   <- coverage limits, never a pass
### Grimey's Final Word
```

Two sections carry most of the weight. **Self-Grind Reconciliation** reports what the review killed in its own findings, and the arithmetic has to close: a candidate absent from `N = M + K` cannot appear in the report. **Survived Scrutiny** admits an entry only when a specific probe was performed and its result recorded; a claim nobody tried to falsify goes to **Not Examined** instead, which is a confession of coverage limits rather than a pass.

## Auto-Loop

When `--auto-loop` is enabled, the grind keeps iterating while each pass still surfaces new P0/P1 findings. It stops on a confirmed pass, on the first pass that finds nothing new, or at the iteration cap.

### How It Works

1. The engine writes the run record to `.grimes/` as it reviews: loop state, the derived result, and the ledger
2. On session stop, the agent's hook system calls `hooks/stop.sh`
3. The hook invokes `grimes loop`, which verifies the record and decides:
   - **Exit 0**: Allow exit (pass confirmed, no new P0/P1 findings, iteration cap reached, or the record could not be verified)
   - **Exit 2**: Block exit and re-inject the grind prompt (continue iterating)

### The Run Record

The record is protobuf and the engine writes it. A review cannot report its own verdict: the result is accepted only when its digest is the one the loop state recorded, its run identity and target fingerprint match, and it declares orchestrator authorship. A GREEN result additionally has to carry an independent review of the same target with no oscillation, which the contract enforces rather than the hook.

A record that fails any of those checks is quarantined under `.grimes/quarantine/` and the session ends with no pass recorded. Inspect it with `grimes state --show`.

### Provider Hook Integration

| Provider | Hook Configuration |
|----------|-------------------|
| Claude Code | `hooks.json` in plugin or project config |
| OpenCode | `AGENTS.md` or plugin manifest with `hooks.stop` |
| Codex | `openai.yaml` with `hooks.stop` |
| Other | Any stop hook mechanism that can call `hooks/stop.sh` |

See `hooks/README.md` for detailed integration instructions.

## Dependencies

- `jq`: required for the stop hook to parse state JSON
- `bubblewrap`: the built-in confinement mechanism on Linux. On Ubuntu 24.04 and later it needs the AppArmor profile the package ships to be loaded, or `kernel.apparmor_restrict_unprivileged_userns=0`; without either, `bwrap` reports `setting up uid map: Permission denied`. macOS uses `sandbox-exec`, which ships with the system. Without it every role is refused; see [Confinement](#confinement) for `--sandbox-command` and `--unsafe`.

Install on macOS: `brew install jq`
Install on Ubuntu/Debian: `apt-get install jq bubblewrap`
Install on Fedora: `dnf install jq bubblewrap`

## Quality Checklist

Before shipping, verify your grind meets these standards:

- [ ] Every issue has specific evidence (code path, line number, scenario)
- [ ] Issues span multiple critique categories, not clustered in one
- [ ] Severity ratings are correct (P0 for blocking, P1 for significant, P2 for friction, P3 for debt)
- [ ] Verdict matches the findings (GREEN only when P0 are mitigated)
- [ ] Report has all required sections (verdict, BLUF, top 3 risks, risk register, survived scrutiny, final word)
- [ ] Issues use the correct format (Grime ID, evidence, category, severity, likelihood, blast radius)
- [ ] No anti-patterns (Grimey Theater, Optimism Creep, Authority Deference, Perfection Paralysis, Orphaned Risks)
- [ ] Voice is clinical and direct, not obscured by satire

## Benchmarking

The `benchmark/` directory contains a framework for validating grind quality across different target types.

### Running the Benchmark

```bash
# Run against all targets
./benchmark/runner.sh --all

# Run against a specific target
./benchmark/runner.sh ./benchmark/targets/shell/bad-script.sh

# A/B test: compare current results with baseline
./benchmark/runner.sh --all --compare
```

### What the Benchmark Measures

| Dimension | What It Checks |
|-----------|---------------|
| Evidence Quality | One evidence tier per finding, with the record that tier requires |
| Routing Discipline | 5-8 categories routed, with a reason for every inclusion and exclusion |
| Severity Assessment | Correct P0/P1/P2/P3 classification |
| Verdict Accuracy | Complete tuple, colour derived rather than asserted |
| Report Structure | All required sections present and complete |
| Finding Format Compliance | Register rows carry ID, category, tier, invariant, severity, assumption status |
| Fix Quality | Correct fixes, verification, regression scope |
| Anti-Pattern Avoidance | No Grimey Theater, Optimism Creep, or unearned acquittals |
| Self-Grind and Earned Acquittal | Reconciliation closes; acquittals name a performed probe |

Grimey's voice is deliberately not scored: it exists because it is fun to read, and paying points for persona buys theater at the expense of findings.

See `benchmark/rubric.md` for the full rubric and scoring criteria.

## Anti-Patterns

The skill warns against these failure modes:

- **Grimey Theater**: Going through motions without genuine skepticism
- **Optimism Creep**: "It'll probably be fine." No. Prove it.
- **Authority Deference**: "The LLM said so." Verify anyway.
- **Perfection Paralysis**: Never shipping because something might be wrong
- **Orphaned Risks**: Accepted risks with no owner

## Credits

- Methodology inspired by pre-mortems, red teaming, and threat modeling
- Loop technique inspired by similar pessimistic iteration approaches
- Named after Frank Grimes from The Simpsons, S8E23 "Homer's Enemy"

---

*"You know what makes me mad? Not just that this is broken - it's that someone shipped it thinking it was fine. That's the real failure."*
The Spirit of Grimey
