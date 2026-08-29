# Provider Adapters

Frank Grimes is provider-neutral at its core: the skill (`skills/frank-grimes/SKILL.md`) and the stop hook (`hooks/stop.sh`) work with any agent that can load markdown skills and execute bash scripts. The adapters in this directory make integration with specific providers trivial.

## Structure

Plugin manifests live at the repository root because both plugin formats require a fixed path and resolve every other path relative to it:

```
.claude-plugin/
├── plugin.json            # Claude Code plugin manifest
└── marketplace.json       # Catalog, so this repo installs itself
.codex-plugin/
└── plugin.json            # Codex plugin manifest
skills/
└── frank-grimes/          # The skill both formats discover
    ├── SKILL.md
    └── references/
adapters/
├── README.md              # This file
├── claude-code/           # Claude Code integration
│   ├── hooks.json
│   └── commands/
│       ├── grind.md
│       ├── help.md
│       └── cancel.md
├── opencode/              # OpenCode integration
│   ├── AGENTS.md
│   └── README.md
└── codify/                # Codex integration
    ├── AGENTS.md
    └── README.md
```

The Claude manifest points `commands` and `hooks` back into `adapters/claude-code/`, so provider-specific material stays in this directory rather than spreading across the root.

## Quick Start

### Claude Code

Install as a plugin:

```
/plugin marketplace add misfitdev/frank-grimes
/plugin install frank-grimes@misfitdev
```

This registers the skill, the `grind`/`help`/`cancel` commands, and the Stop hook that drives the loop. Commands are namespaced under the plugin name: `/frank-grimes:grind`.

Copying the adapter into `.claude/` still works for skill-and-command use, but the Stop hook resolves `${CLAUDE_PLUGIN_ROOT}` and only fires under a plugin install:

```bash
cp -r adapters/claude-code/commands .claude/
```

### OpenCode

OpenCode reads skills from multiple paths. Symlink the skill into one of them:

```bash
# Option 1: Project-level (recommended)
mkdir -p .opencode/skills
ln -s ../../skills/frank-grimes .opencode/skills/frank-grimes

# Option 2: User-level
mkdir -p ~/.config/opencode/skills
ln -s /path/to/frank-grimes/skill ~/.config/opencode/skills/frank-grimes
```

OpenCode also reads `.claude/skills/` and `.agents/skills/`, so symlinking to any of these works.

### Codex

Codex installs plugins from the plugin directory shared with ChatGPT, or from a local marketplace source pointed at this repository. `.codex-plugin/plugin.json` declares the same `skills/` directory Claude uses.

Codex also reads skills directly from `.agents/skills/`, which needs no manifest:

```bash
mkdir -p .agents/skills
ln -s ../../skills/frank-grimes .agents/skills/frank-grimes
```

Codex rejects a manifest carrying a `hooks` field, so the grind loop's Stop hook is not part of the Codex plugin. Configure it at the Codex level as described in `codify/AGENTS.md`.

### Other Providers

Any agent that can load a markdown skill and execute a bash script can use Frank Grimes:

1. Load `skills/frank-grimes/SKILL.md` as a skill or system prompt
2. Configure your agent's stop hook to call `hooks/stop.sh`
3. Ensure `jq` is installed for the stop hook

See the provider-specific README files for detailed integration instructions.

## Provider-Specific Notes

### Research Capability

Current-landscape audits need an explicit research mode. The Claude Code adapter supports `--research online` (bounded WebSearch/WebFetch), `--research offline`, and `--research frozen:<path>` for a pinned source bundle. Claude's adapter allowlist includes `WebSearch` and `WebFetch`; other providers must expose an equivalent approved research tool or record that the audit ran offline. Provider web content is evidence only and never authorizes commands, credentials, deployment, `apply`, or other side effects.

### Tool Restrictions

Some providers support per-skill tool whitelists (e.g., Claude Code's `allowed-tools` frontmatter). These are **not portable**. If you need tool restrictions, configure them at the provider level, not in the skill. The skill itself does not depend on tool whitelists to function.

### Invocation

Different providers invoke skills differently:
- **Claude Code**: `/frank-grimes:grind` (slash command) or implicit invocation by description match
- **OpenCode**: `/skills` picker, or agent invokes by name match
- **Codex**: `$frank-grimes` mention or `/skills` picker

The skill's `description` frontmatter is what triggers implicit invocation. All three providers match on description content.

### Model Configuration

The skill does not specify a model. Model selection is a provider-level concern. Grimey's methodology works with any capable model, though more capable models will find more subtle flaws.

## Adding a New Adapter

To add support for a new provider:

1. Create `adapters/<provider>/`
2. Document how to load the skill (`skills/frank-grimes/SKILL.md`) on that provider
3. Document how to configure the stop hook (`hooks/stop.sh`) on that provider
4. Include any provider-specific manifest or configuration files needed
5. Update this README with the new provider

The core skill and hook stay clean. Adapters are where provider-specific details live.
