# OpenCode Integration

OpenCode reads skills from multiple project and user paths, giving Frank Grimes portability for free.

## Skill Installation

OpenCode reads skills from (in order of precedence):
1. `.opencode/skills/<name>/`
2. `.claude/skills/<name>/`
3. `.agents/skills/<name>/`

User-level paths:
1. `~/.config/opencode/skills/<name>/`
2. `~/.claude/skills/<name>/`
3. `~/.agents/skills/<name>/`

### Project-Level (Recommended)

```bash
mkdir -p .opencode/skills
ln -s ../../skills/frank-grimes .opencode/skills/frank-grimes
```

This creates a symlink so the skill lives in one place (`skill/`) and OpenCode finds it.

### User-Level

```bash
mkdir -p ~/.config/opencode/skills
ln -s /path/to/frank-grimes/skill ~/.config/opencode/skills/frank-grimes
```

## Stop Hook Integration

OpenCode supports hook configuration in `AGENTS.md` or plugin manifests. Add a stop hook:

```yaml
# AGENTS.md or plugin manifest
hooks:
  stop:
    - command: hooks/stop.sh
      timeout: 600
```

Place `hooks/stop.sh` in your project root or adjust the path accordingly.

## Invocation

- **Explicit**: Use the `/skills` picker to select "frank-grimes", then invoke with the grind command
- **Implicit**: OpenCode matches the skill description against user requests. Phrases like "review this code", "find problems with", "red team", or "pre-mortem" will trigger the skill.

## Tool Restrictions

OpenCode does not support per-skill tool whitelists in frontmatter. If you need tool restrictions, configure them at the agent or project level. The skill functions without tool whitelists.

## Model Configuration

Model selection is configured at the agent or project level in OpenCode. The skill does not specify a model.

## Frontmatter Compatibility

OpenCode honors `name` and `description` in skill frontmatter. Other fields (`allowed-tools`, `model`, `arguments`, etc.) are silently ignored. The skill's `skills/frank-grimes/SKILL.md` uses only the portable subset.

## Independent Adjudication

Adjudication is engine-owned; the skill states the rule. This adapter supplies only the command that starts the second context:

```bash
grimes run --dir=. \
  --adjudicator-command="opencode" \
  --adjudicator-arg="run" \
  --adjudicator-arg="<the adjudicator prompt>" \
  --adjudicator-fresh \
  ...
```

`opencode run` is OpenCode's own non-interactive invocation; substitute whatever that is on your installation. `--adjudicator-fresh` is the operator asserting that the command begins a new context.

The engine exports the request to that process. The prompt tells it to review `$GRIMES_TARGET_CONTENT` under the skill and to end with the report:

```bash
grimes-contract adjudicate --decision=<d> --residual-risk=<r> \
    --review-confidence=<c> --review-completeness=<x>
```

Run identity and target fingerprint come from the exported environment, so the prompt carries neither.

## Refutation

Refutation is engine-owned; the skill states the rule. This adapter supplies only the command that starts the attacking context:

```bash
grimes run --dir=. \
  --refuter-command="opencode" \
  --refuter-arg="run" \
  --refuter-arg="<the refuter prompt>" \
  --refuter-fresh \
  ...
```

`--refuter-fresh` is the operator asserting that the command begins a new context.

The engine exports the claims to that process. The prompt tells it to read them, attack each one, and answer every ref exactly once:

```bash
grimes-contract refute claims
grimes-contract refute add --ref=<r> --refuted \
    --action=<cmd> --cwd=<dir> --exit-code=<n> --output=<excerpt>
grimes-contract refute add --ref=<r> --upheld ...
grimes-contract refute add --ref=<r> --unavailable="<what was missing>"
grimes-contract refute seal
```

Run identity and target fingerprint come from the exported environment, so the prompt carries neither. The engine plants its own control and maps each answer back to the finding it was issued for; the prompt carries no control and relays no outcome by hand.

## Running the roles on other CLIs

`adapters/roles.md` is where the per-role commands, the preflight a run needs
before it starts, and what to do when one fails are written down. It is the only
copy: what is here is this adapter's own wiring.
