# Frank Grimes standalone-repository plan

You are finishing the migration of Frank Grimes from the `misfitdev/claude-plugins` marketplace into this standalone repository.

## Goal

Ship a self-contained, provider-neutral Frank Grimes project. It should preserve the plugin’s “nervous-breakdown Grimey” personality—intensely competent, emotional about corner-cutting, and relentless about evidence—without being tied to Claude Code or any single model/provider.

## Starting state

- `README.md`, `LICENSE`, `assets/grimey.png`, and `.gitignore` have been seeded in this repository.
- The source plugin remains at `/Users/tucker/git/misfitdev/claude-plugins/plugins/frank-grimes` and is reference material only; do not change it unless explicitly asked.
- Preserve the existing user-facing Grimes Grind methodology, commands/flows where portable, report structure, and state-loop behavior.

## Work plan

1. Inspect the source plugin completely: manifest, commands, skills/prompts, hooks, scripts, assets, and documentation.
2. Establish a simple standalone layout and copy/adapt the necessary source files.
3. Replace platform-specific language and installation instructions with provider-neutral guidance. Do not say “Claude Code” unless documenting migration history.
4. Define a minimal, explicit integration surface for multiple agents/models/providers: the prompt/skill content must be portable, and any provider-specific adapters must be optional and isolated.
5. Update the README so a new user can understand Frank, install or integrate it, run a grind, interpret its verdicts, and find requirements.
6. Keep the personality sharp but useful: satire should never obscure instructions, safety constraints, or verification requirements.
7. Add only the tooling/tests that the project needs. Validate every manifest/configuration file and exercise any scripts or hooks you add.
8. Review `git diff` and report what changed, how to use it, validation results, and any remaining provider-specific decisions.

## Guardrails

- Do not commit, push, publish, delete source material, or modify the original marketplace repository without explicit approval.
- Do not include API keys, provider credentials, local model files, generated reports, or runtime state in version control.
- Prefer portable Markdown, JSON, and shell. Keep dependencies minimal.
- Never claim a grind is GREEN without evidence from the relevant checks.

## Definition of done

- The repository contains all required standalone functionality and assets.
- Documentation is provider-neutral and accurately reflects the implementation.
- The project validates cleanly with reproducible commands.
- The final handoff identifies any deliberately deferred adapter or distribution work.
