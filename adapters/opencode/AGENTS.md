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

A `pass` verdict requires a second review that never saw the first. OpenCode has no bundled equivalent of the Claude adapter's `grimey-verifier` subagent, so a grind here records `Independent adjudication: not available`, sets `review_confidence=low`, and caps at `conditional`/YELLOW.

To reach a pass, run the grind a second time in a fresh session given only the target and the claimed verdict tuple — never the first report — and resolve the two by the stricter decision.
