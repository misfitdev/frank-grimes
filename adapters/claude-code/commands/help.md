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
# Interactive — prompts for scope
/frank-grimes:grind

# Direct — skip prompts by specifying the target
/frank-grimes:grind ./src/auth.ts --auto-loop
```

When invoked without arguments, Grimes asks for the scope and defaults to report mode. Category routing is not a question — the skill selects 5-8 of the ten categories from the target's own shape and records why for all ten.

## Verdicts

The colour is derived from the verdict tuple, never asserted. See the skill for the derivation.

- **RED:** `decision=block` — an open P0 remains
- **YELLOW:** anything unconfirmed — including a carried accepted P0, or no available adjudicator
- **GREEN:** the full tuple plus an independent adjudicator that reached the same conclusion without seeing the report

## Command Reference

### `/frank-grimes:grind [target] [options]`

Starts a Grimes Grind. If invoked without arguments, prompts for scope. Reports by default.

**Arguments:**
- `target` (optional) - What to grind: file path, directory, code snippet, or description. Skips the scope question.
- `--scope recent-changes|whole-repo` (optional) - Shorthand scope. Skips the scope question.
- `--categories COR,SEC,REL` (optional) - Restrict routing to these canonical categories. Default: the skill routes 5-8 itself.
- `--mode report|fix` (optional) - `report` documents findings and edits nothing (default); `fix` applies fixes and then runs a verification gate. Skips the mode question.
- `--verify-command <cmd>` (optional) - Command used to verify a fix batch. Without a usable gate, fix mode edits but never commits.
- `--commit` (optional) - Authorize one commit of the verified batch. Requires `--mode fix` and a gate that exited zero. Off by default.
- `--max-iterations N` (optional) - Maximum iterations before stopping (default: 5)
- `--auto-loop` (optional) - Continue while iterations still change the verdict, up to the maximum

**Examples:**
```bash
/frank-grimes:grind ./src/auth.ts
/frank-grimes:grind --scope recent-changes --mode report
/frank-grimes:grind "Review this architecture" --max-iterations 3 --auto-loop
/frank-grimes:grind ./src/api --mode fix --verify-command "just check"
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
