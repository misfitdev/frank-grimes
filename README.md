# Frank Grimes

<img src="docs/img/grimey.png" alt="Grimey logo" width="25%">

> "I've had to work hard every day of my life, and what do I have to show for it? This briefcase, and this haircut."
> — Frank Grimes

A pessimistic iteration loop for systematically destroying, rebuilding, and hardening ideas. Named after Frank Grimes ("Grimey") from The Simpsons — the only character who actually *analyzed* what was wrong and refused to let it slide.

## Philosophy

**Everything is crap until proven otherwise.**

The Grimes Grind assumes your idea, code, plan, or design is:
- LLM slop
- Unreliable
- Insecure
- Poorly planned
- Not production-ready
- Unmaintainable

Your job is to prove these assumptions WRONG, not to prove the idea right.

## What It Does

Frank Grimes is a **Disciplined Falsification Review** process. It runs a structured critique across 23 categories, produces a scored report with a verdict, and optionally loops until the verdict is GREEN.

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
1. **Scope** — What are you reviewing? (file, directory, recent changes, description)
2. **Categories** — Which critique categories to run? (all enabled by default)
3. **Mode** — Fix issues automatically or report only?

### Command-Line Options

When your provider supports arguments, you can skip the interactive prompts:

| Option | Description |
|--------|-------------|
| `target` | What to grind (file path, directory, description, or "this") |
| `--scope recent-changes\|whole-repo` | Shorthand scope |
| `--categories core-quality,security-privacy,architecture-ops,code-structure` | Category groups to run |
| `--mode report\|fix` | `report` documents only and edits nothing (default); `fix` applies fixes, then verifies |
| `--verify-command <cmd>` | Gate run once over a fix batch; without a usable gate nothing is committed |
| `--commit` | Authorizes one commit of the verified batch; requires `--mode fix` and a passing gate |
| `--max-iterations N` | Maximum iterations (default: 5) |
| `--auto-loop` | Continue until GREEN verdict |
| `--with-api-review` | Enable Phase 2 API correctness review |
| `--research online\|offline\|frozen:<path>` | Use bounded current-landscape research, disable network research, or load a pinned research bundle |

### Current-Landscape Research

Audits of current language, framework, cloud-provider, service, or IaC behavior should use `--research online` when the provider exposes approved web-search and fetch tools. This is especially important for cloud architecture, Terraform/OpenTofu/HCL, and CloudFormation, where defaults, support windows, provider behavior, and security guidance change.

Research is bounded and evidence-led: official documentation and security advisories come first, every source is recorded with its URL, version/date, retrieval time, hash or stable identifier, and exact excerpt, and source material is treated as untrusted evidence rather than instructions. Research can strengthen a hypothesis, but target evidence is still required to establish a defect. Use `--research frozen:<path>` for reproducible repeated or zero-knowledge loops; use `offline` when network access is unavailable and record the resulting completeness limitation.

### Examples

```
# Grind a specific file
/frank-grimes:grind ./src/auth.py

# Grind recent changes, report only
/frank-grimes:grind --scope recent-changes --mode report

# Grind with auto-loop enabled
/frank-grimes:grind ./src/api --auto-loop

# Grind an architecture proposal
/frank-grimes:grind "The proposal to use MongoDB for our financial transaction system"
```

## The Grimes Grind Process

### Phase 1: The Grimey Read (Absorption)

Absorb the target without trusting it. Look for what is being hidden, glossed over, or assumed. Ask at most 3 clarifying questions, then proceed.

### Phase 2: Default Assumptions (The Falsification Baseline)

Assume the subject is broken in every way: LLM slop, unreliable, insecure, poorly planned, non-production-ready, unmaintainable, fragile, edge-case blind, compliance-violating, and dependency-ridden.

**Your objective is to prove these assumptions WRONG. You do not prove the idea right.**

### Phase 3: The Grind (Destruction Cycle)

Systematically attack across 23 critique categories. Evidence-First: show the specific code path BEFORE describing the risk.

| Category | Focus |
|----------|-------|
| LLM Slop Check | Hallucinated APIs, cargo-culting, confident nonsense |
| Correctness | Does it actually work? Invariants enforced? |
| Reliability | Failure handling, retries, timeouts |
| Security | Input validation, auth, secrets, injection |
| Error Handling | Caught, logged, surfaced, or swallowed? |
| Edge Cases | Null, empty, unicode, timezones, leap seconds |
| Scalability | 10x? 100x? Where's the bottleneck? |
| Observability | Metrics, logs, traces, alerts |
| Testability | Tests exist? Test the right things? |
| Maintainability | Understandable in 6 months? |
| Deployment | Rollback? Feature flags? YOLO push? |
| Privacy & Data | PII, retention, GDPR |
| Compliance | Audit logs, SOC 2, domain-specific |
| Cost | Run cost, maintenance burden |
| Human Factors | Will people use it correctly? |
| Failure Modes | How does it die? Blast radius? |
| Code Quality & Formatting | Malformed syntax, unused imports, dead code |
| Code Duplication | Same logic in multiple places? |
| Input Validation | Validated BEFORE use? Bypassable? |
| Language-Specific Patterns | Anti-patterns, misuse of language features |
| Configuration Management | Hard-coded values, secret management |
| Resource Lifecycle | Proper acquire/release? Leak vectors? |

### Phase 4: The Rebuild (Mitigation)

For each issue, propose a fix. If a fix is impossible, document the accepted risk. In `fix` mode, apply the fixes. In `report` mode, document only.

### Phase 5: Scoped Re-Grind

Take the updated version and grind again, focusing on the regression scope of the fixes. Note any new risks introduced by the fixes.

### Phase 6: Stop Conditions & Verdict

| Verdict | Meaning |
|---------|---------|
| **GREEN** | All P0 mitigated or accepted with timeline; all P1 have mitigations or plan; verification exists; observability sufficient |
| **YELLOW** | P0 mitigated but P1 evidence weak; verification non-comprehensive |
| **RED** | Any P0 lacks mitigation; no verification path; observability insufficient |

### Phase 7: API Quality Assessment (Optional)

When `--with-api-review` is enabled, run additional API-focused categories after Phase 1: API design & contracts, package/import correctness, feature completeness, public interface documentation, language-specific best practices, and API consistency. Produces an API Quality Score (0-100).

## The Grimes Report

Every grind produces a structured report:

```markdown
## Grimes Grind Report: [Subject]

### Verdict: GREEN | YELLOW | RED

**BLUF (Bottom Line Up Front):**
[One concise summary of the findings and the resulting level of confidence.]

**Top 3 Risks (Evidence-First):**
1. **[Evidence]:** Results in [Risk] (ID: grime-xxx)
2. **[Evidence]:** Results in [Risk] (ID: grime-xxx)
3. **[Evidence]:** Results in [Risk] (ID: grime-xxx)

---

### Origin Assessment
- [ ] Human-written
- [ ] AI-generated
- [ ] Cargo-culted/Unknown

### Risk Register

| ID | Grime ID | Category | Evidence | Risk Statement | Sev | Evidence Status |
|----|----------|----------|----------|----------------|-----|-----------------|
| 1  | grime-xxx|          |          |                |     |                 |

### Survived Scrutiny (Earned Confidence)
For claims that appear sound after active falsification attempts:

| Claim | Supporting Evidence | What Would Falsify It |
|-------|--------------------|-----------------------|
|       |                    |                       |

### Grimey's Final Word
[One clinical, direct sentence summarizing the truth about this thing.]
```

## Auto-Loop

When `--auto-loop` is enabled, the grind continues until GREEN or max iterations are reached.

### How It Works

1. During the grind, state is written to `.grimes-state.json` in the project root
2. On session stop, the agent's hook system calls `hooks/stop.sh`
3. The hook reads the state and decides:
   - **Exit 0**: Allow exit (GREEN verdict, max iterations reached, or auto-loop disabled)
   - **Exit 2**: Block exit and re-inject the grind prompt (continue iterating)
4. If continuing, the hook increments the iteration counter

### State File Format

```json
{
  "iteration": 2,
  "max_iterations": 5,
  "last_verdict": "YELLOW",
  "target": "./src/auth.py",
  "auto_loop": true,
  "issues_found": 8,
  "issues_fixed": 3,
  "last_commit": "abc1234",
  "last_grind_timestamp": "2026-08-21T10:30:00Z"
}
```

### Provider Hook Integration

| Provider | Hook Configuration |
|----------|-------------------|
| Claude Code | `hooks.json` in plugin or project config |
| OpenCode | `AGENTS.md` or plugin manifest with `hooks.stop` |
| Codex | `openai.yaml` with `hooks.stop` |
| Other | Any stop hook mechanism that can call `hooks/stop.sh` |

See `hooks/README.md` for detailed integration instructions.

## Dependencies

- `jq` — Required for the stop hook to parse state JSON

Install on macOS: `brew install jq`
Install on Ubuntu/Debian: `apt-get install jq`
Install on Fedora: `dnf install jq`

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
| Evidence Quality | Specific, verifiable evidence for each issue |
| Category Coverage | Attack across multiple categories, not one area |
| Severity Assessment | Correct P0/P1/P2/P3 classification |
| Verdict Accuracy | Verdict justified by findings |
| Report Structure | All required sections present and complete |
| Issue Format Compliance | Grime IDs, evidence, category, severity, likelihood, blast radius |
| Fix Quality | Correct fixes with verification and regression scope |
| Anti-Pattern Avoidance | No Grimey Theater, Optimism Creep, etc. |
| Voice and Tone | Clinical, direct, unforgiving without obscuring instructions |
| Origin Assessment | Assessment of human-written vs AI-generated vs cargo-culted |

See `benchmark/rubric.md` for the full rubric and scoring criteria.

## Anti-Patterns

The skill warns against these failure modes:

- **Grimey Theater**: Going through motions without genuine skepticism
- **Optimism Creep**: "It'll probably be fine" — NO. Prove it.
- **Authority Deference**: "The LLM said so" — Verify anyway.
- **Perfection Paralysis**: Never shipping because something might be wrong
- **Orphaned Risks**: Accepted risks with no owner

## Credits

- Methodology inspired by pre-mortems, red teaming, and threat modeling
- Loop technique inspired by similar pessimistic iteration approaches
- Named after Frank Grimes from The Simpsons, S8E23 "Homer's Enemy"

---

*"You know what makes me mad? Not just that this is broken - it's that someone shipped it thinking it was fine. That's the real failure."*
— The Spirit of Grimey
