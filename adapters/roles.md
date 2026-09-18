# Running a role on another CLI

The engine spawns each role as its own process, and the three role commands are
independent. So the reviewer, the attacker and the adjudicator can each be a
different agent, on a different CLI, running a different model.

```bash
--provider-command      the primary review, and the repair in fix mode
--refuter-command       the pass that attacks what the primary claimed
--adjudicator-command   the independent verdict; may be given more than once
```

That is what makes an adjudication independent rather than nominal: a separate
process, a separate context, and — if you want — a separate model that has
never seen the first review. A `pass` needs one.

## What each CLI needs

Verified 2026-09-18. Each of these runs one shot, takes the prompt as the last
argument, and prints the agent's final message to stdout, which is where the
engine reads a report from.

| CLI | Command | Model | The part that is not obvious |
|---|---|---|---|
| Claude Code | `claude` + `-p` | `--model` | — |
| Codex | `codex exec` + `--dangerously-bypass-approvals-and-sandbox` | `-m` | Without the bypass it dies at `sandbox_apply: Operation not permitted`. See "A provider that sandboxes itself" in the README: two Seatbelt profiles cannot nest. |
| OpenCode | `opencode run` | `-m provider/model` | Prints its model banner to stderr and the answer to stdout. Harmless — the engine reads stdout — but it makes a plain terminal run look odd. |

`--provider-command` is split on whitespace, so `codex exec` and `opencode run`
work as written. Everything after that goes in its own `--provider-arg`, which
is not split: a prompt, or any value containing a space, must be one argument.

## Saying which model does which stage

> Use Qwen through OpenCode for the review, and Opus for the final word.

```bash
grimes run --dir=. --mode=fix \
  --provider-command="opencode run" \
    --provider-arg="-m" --provider-arg="qwen/qwen3.7-plus" \
    --provider-arg="$(cat prompts/primary.txt)" \
  --adjudicator-command="claude" \
    --adjudicator-arg="-p" \
    --adjudicator-arg="--model" --adjudicator-arg="opus" \
    --adjudicator-arg="$(cat prompts/adjudicator.txt)" \
  --adjudicator-fresh \
  HEAD^..HEAD
```

> Review with Codex, attack with a different model, and put it to two
> adjudicators that do not share a vendor.

```bash
grimes run --dir=. \
  --provider-command="codex exec" \
    --provider-arg="--dangerously-bypass-approvals-and-sandbox" \
    --provider-arg="$(cat prompts/primary.txt)" \
  --refuter-command="opencode run" \
    --refuter-arg="-m" --refuter-arg="qwen/qwen3.7-plus" \
    --refuter-arg="$(cat prompts/refuter.txt)" --refuter-fresh \
  --adjudicator-command="claude" \
    --adjudicator-arg="-p" --adjudicator-arg="$(cat prompts/adjudicator.txt)" \
  --adjudicator-command="opencode run" \
    --adjudicator-arg="-m" --adjudicator-arg="qwen/qwen3.7-plus" \
    --adjudicator-arg="$(cat prompts/adjudicator.txt)" \
  --adjudicator-fresh \
  .
```

`--adjudicator-fresh` is one assertion covering every reviewer the engine
spawns: you are saying each command begins a context that has not seen the
first review. It is your word, and the record keeps it as such.

## What to put in a role prompt

Not the methodology. `skills/frank-grimes/SKILL.md` is the normative procedure,
and a role prompt that restates it is a second copy that will drift from it.

Ready-made templates are in `adapters/prompts/`:

```text
primary.txt       the review, and the repair in fix mode
refuter.txt       the attack on what the primary claimed
adjudicator.txt   the independent verdict
```

Each says who the role is, points at the skill, names the command that ends its
turn, and carries the two rules an agent gets wrong unprompted: paths given to
`grimes-contract` are relative to the target root, and a validation error means
correct the arguments and try again rather than abandon the review.

## Where a report comes from

`report seal` writes the envelope to the work directory **and** prints it. The
engine collects the file first and falls back to stdout, so a role that seals
and then describes its work in prose is not penalised for it. That is the
ordinary behaviour of an agent CLI, and it needs no prompting against.

## What the engine gives a role

Every role is told what it is reviewing and where it may write:

```text
GRIMES_TARGET_ROOT         where unit ids resolve
GRIMES_TARGET_CONTENT      the bytes under review
GRIMES_TARGET_FINGERPRINT  the target's identity
GRIMES_TARGET_KIND         code, document, idea, external
GRIMES_ROLE                primary, refuter, or adjudicator
GRIMES_RUN_ID              the run this answer belongs to
GRIMES_WORK_DIR            the only directory a role may write, besides the fix worktree
```

The rest is given by role, and what is withheld is deliberate:

```text
primary      GRIMES_TARGET_INVENTORY   what its coverage is measured against
             GRIMES_MODE, GRIMES_ITERATION, GRIMES_CATEGORIES, GRIMES_RESEARCH
refuter      GRIMES_CLAIMS             the claims to attack, and nothing else
adjudicator  nothing further
```

A refuter gets no inventory and no categories: it is not reviewing the target,
and a coverage denominator would invite it to. An adjudicator gets neither
those nor the verdict it is meant to reach independently.

Use them in the prompt as words, not as shell variables inside commands the
agent composes: a subshell may not carry them, and an empty expansion writes to
`/`, which is read-only and fails.
