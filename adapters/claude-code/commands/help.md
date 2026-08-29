---
description: Display Frank Grimes plugin documentation and usage
allowed-tools: []
---

# Frank Grimes: Disciplined Falsification Review

The Frank Grimes plugin implements a clinical, pessimistic iteration loop for systematically destroying, rebuilding, and hardening ideas. It assumes all input is flawed until proven otherwise through survival.

## Available Commands

- `/frank-grimes:grind <target>` - Initiates a Disciplined Falsification Review on code, architecture, or plans
- `/frank-grimes:cancel` - Terminates an active grind loop and removes state data
- `/frank-grimes:help` - Displays this documentation

## Methodology: Earned Confidence

Confidence is not assumed; it is earned by surviving a relentless, adversarial critique.

1. **Assume Failure:** Every draft is broken, insecure, and unreliable by default
2. **Active Falsification:** We actively seek evidence to prove the draft wrong
3. **Evidence-First:** All identified risks must be preceded by technical evidence (code paths, logic flaws)
4. **Iterative Hardening:** We fix identified flaws and re-grind until we reach a GREEN verdict

## Quick Start

```bash
# Interactive — prompts for scope, categories, and mode
/frank-grimes:grind

# Direct — skip prompts by specifying the target
/frank-grimes:grind ./src/auth.ts --auto-loop
```

When invoked without arguments, Grimes will ask three setup questions before grinding:
1. **Scope** — Recent changes, whole repo, or a specific target
2. **Categories** — Which of the 23 critique categories to evaluate (all enabled by default)
3. **Mode** — Fix issues automatically or report only

## Verdicts

- **GREEN:** Confidence earned. Terminal flaws mitigated or risks explicitly accepted
- **YELLOW:** Conditional confidence. Mitigation evidence is weak or incomplete
- **RED:** Failure. Critical flaws exist without mitigation

## Command Reference

### `/frank-grimes:grind [target] [options]`

Starts a Grimes Grind. If invoked without arguments, prompts interactively for scope, categories, and mode.

**Arguments:**
- `target` (optional) - What to grind: file path, directory, code snippet, or description. Skips the scope question.
- `--scope recent-changes|whole-repo` (optional) - Shorthand scope. Skips the scope question.
- `--categories core-quality,security-privacy,architecture-ops,code-structure` (optional) - Comma-separated category groups. Skips the category question. Default: all groups.
- `--mode fix|report` (optional) - `fix` applies fixes automatically (default); `report` documents findings only without editing files. Skips the mode question.
- `--max-iterations N` (optional) - Maximum iterations before stopping (default: 5)
- `--auto-loop` (optional) - Automatically continue until GREEN verdict or max iterations reached
- `--with-api-review` (optional) - Enable Phase 2 API Correctness & Completeness review

**Examples:**
```bash
/frank-grimes:grind ./src/auth.ts
/frank-grimes:grind --scope recent-changes --mode report
/frank-grimes:grind "Review this architecture" --max-iterations 3 --auto-loop
/frank-grimes:grind ./src/api --with-api-review --mode report
/frank-grimes:grind this --auto-loop
```

### `/frank-grimes:cancel`

Terminates an active grind session and removes state data.

**When to use:**
- To stop an active grind loop without waiting for completion
- To reset state after a session interruption
- To start fresh on a new target

## Version

Frank Grimes v1.0.0 with enforced auto-loop contract validation.

For the full technical methodology, see `skills/frank-grimes/SKILL.md` in the plugin directory.
