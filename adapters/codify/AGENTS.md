# Codex Integration

Codex reads skills from `.agents/skills/<name>/` in the project directory (walking up to repo root) or from `~/.agents/skills/<name>/` user-wide.

## Skill Installation

### Project-Level (Recommended)

```bash
mkdir -p .agents/skills
ln -s ../../skills/frank-grimes .agents/skills/frank-grimes
```

This creates a symlink so the skill lives in one place (`skill/`) and Codex finds it.

### User-Level

```bash
mkdir -p ~/.agents/skills
ln -s /path/to/frank-grimes/skill ~/.agents/skills/frank-grimes
```

## Stop Hook Integration

Codex supports hook configuration in `openai.yaml` or project settings. Add a stop hook:

```yaml
# openai.yaml or equivalent config
hooks:
  stop:
    - command: hooks/stop.sh
      timeout: 600
```

Place `hooks/stop.sh` in your project root or adjust the path accordingly.

## Invocation

- **Explicit**: Use the `/skills` picker to select "frank-grimes", or mention `$frank-grimes` in your prompt
- **Implicit**: Codex matches the skill description against user requests. Phrases like "review this code", "find problems with", "red team", or "pre-mortem" will trigger the skill.

## Tool Restrictions

Codex does not support per-skill tool whitelists in skill frontmatter. Tool restrictions are configured at the server level via `enabled_tools` in `openai.yaml`. The skill functions without tool whitelists.

## Model Configuration

Model selection is configured at the config level in Codex. The skill does not specify a model.

## Frontmatter Compatibility

Codex honors `name` and `description` in skill frontmatter. Other fields are silently ignored. The skill's `skills/frank-grimes/SKILL.md` uses only the portable subset.

## MCP Tool Naming

If you use MCP tools with Frank Grimes, note that Codex uses per-server `enabled_tools` lists by tool name, not prefixed names. Tools should be referenced by their conceptual name in the skill body, not by platform-specific prefixes.

## Independent Adjudication

Adjudication is engine-owned; the skill states the rule. This adapter supplies only the command that starts the second context:

```bash
grimes run --dir=. \
  --adjudicator-command="codex" \
  --adjudicator-arg="exec" \
  --adjudicator-arg="<the adjudicator prompt>" \
  --adjudicator-fresh \
  ...
```

`codex exec` is Codex's own non-interactive invocation; substitute whatever that is on your installation. `--adjudicator-fresh` is the operator asserting that the command begins a new context.

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
  --refuter-command="codex" \
  --refuter-arg="exec" \
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
