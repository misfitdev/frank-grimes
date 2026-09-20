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

## Before the run

An operator says one sentence. These are the parts of it nobody says, and every
one of them cost a failed run to learn.

**The tree must be clean.** A fix run builds its worktree from the commit you
are on, so uncommitted work means the bytes in front of the operator are not the
bytes under review, and anything lying there untracked is credited to the fixer
at the end. Do not commit or stash their work to get past it; say what is dirty.

**Tooling dirt is excluded, not committed.** Codex writes `.codex/` into the
repository, so a trial creates the state that refuses the next run:

```bash
echo '.codex/' >> "$(git rev-parse --path-format=absolute --git-common-dir)/info/exclude"
```

Through the common git directory rather than `.git/info/exclude`: in a linked
worktree `.git` is a file and the redirection fails with `not a directory`. This
form is correct in both. Do not reach for `GIT_CONFIG_*` — the engine strips it
deliberately, because the same mechanism sets `core.fileMode`, and an exclusion
passed that way is ignored without saying so.

**A revision range is a target, not a description.** Pass `b24a0e0^..HEAD`
directly as the target argument. Passing `.` and describing the range in prose
measures coverage against every file in the repository, which caps the verdict
for a reason that has nothing to do with the code.

**Build both binaries from one commit.** They are one contract in two halves and
a mismatched pair fails in ways that read as engine defects. `just build` stamps
them; `grimes --version` and `grimes-contract --version` must agree.

## When a run fails

Preserve and report. Do not retry, repair the runner, or clean up.

- the exact `grimes run` argv, including every role prompt in full
- the exact error
- `.grimes/work/<role>.log`, which the engine writes for every pass
- whether `.grimes/fix` exists, and `git -C .grimes/fix status --porcelain`
- `grimes state --show --dir=.`

Report **what happened** — the command, its output, the state — and do not
diagnose it. This applies to your own mistakes as much as the engine's: paste
the command, not an account of why it went wrong. A cause named without being
checked sends the reader to the wrong place, and a wrong one is worse than none.

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
    --provider-arg="$(cat adapters/prompts/primary.txt)" \
  --adjudicator-command="claude" \
    --adjudicator-arg="-p" \
    --adjudicator-arg="--model" --adjudicator-arg="opus" \
    --adjudicator-arg="$(cat adapters/prompts/adjudicator.txt)" \
  --adjudicator-fresh \
  HEAD^..HEAD
```

> Review with Codex, attack with a different model, and put it to two
> adjudicators that do not share a vendor.

```bash
grimes run --dir=. \
  --provider-command="codex exec" \
    --provider-arg="--dangerously-bypass-approvals-and-sandbox" \
    --provider-arg="$(cat adapters/prompts/primary.txt)" \
  --refuter-command="opencode run" \
    --refuter-arg="-m" --refuter-arg="qwen/qwen3.7-plus" \
    --refuter-arg="$(cat adapters/prompts/refuter.txt)" --refuter-fresh \
  --adjudicator-command="claude" \
    --adjudicator-arg="-p" --adjudicator-arg="$(cat adapters/prompts/adjudicator.txt)" \
  --adjudicator-command="opencode run" \
    --adjudicator-arg="-m" --adjudicator-arg="qwen/qwen3.7-plus" \
    --adjudicator-arg="$(cat adapters/prompts/adjudicator.txt)" \
  --adjudicator-fresh \
  .
```

`--adjudicator-fresh` is one assertion covering every reviewer the engine
spawns: you are saying each command begins a context that has not seen the
first review. It is your word, and the record keeps it as such.

> Grind this repo against the last three commits, fix mode, all three roles on
> Codex.

```bash
grimes run --dir="$PWD" --mode=fix \
  --provider-command="codex exec" \
    --provider-arg="--dangerously-bypass-approvals-and-sandbox" \
    --provider-arg="$(cat adapters/prompts/primary.txt)" \
  --refuter-command="codex exec" \
    --refuter-arg="--dangerously-bypass-approvals-and-sandbox" \
    --refuter-arg="$(cat adapters/prompts/refuter.txt)" --refuter-fresh \
  --adjudicator-command="codex exec" \
    --adjudicator-arg="--dangerously-bypass-approvals-and-sandbox" \
    --adjudicator-arg="$(cat adapters/prompts/adjudicator.txt)" --adjudicator-fresh \
  --provider-timeout=45m \
  --verify-command='just check' \
  HEAD~3..HEAD
```

The prompt is the whole file, read as-is. Do not extract part of one, append to
one, or compose one from more than one source. A pattern written to pull text
out of a file is how two runs lost their role prompts to an empty string.

No `--json`. It streams every tool call to stdout, and a long review crosses the
retention bound mid-pass; it buys nothing either, since the report arrives as a
sealed file and the engine records stdout regardless.

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

## Reviewing with your own context

The engine spawns a provider, so from the outside a role always looks fresh.
It is not, if the command you gave it reaches back into the session running the
review. An agent that drives Grimes and also answers as the provider is the
most contaminated context in the system: it has seen the target, the previous
iterations, and its own earlier conclusions, and it is now being asked whether
those conclusions hold.

Nothing can detect that. Say it instead:

```bash
grimes run --provider-is-coordinator ...
```

The run records `coordinator_authored`, reports `coordinator_separation` as an
unmet gate, and caps `review_confidence` at medium, which puts GREEN out of
reach. It does not stop the review: a contaminated context still finds real
defects. It stops the review from vouching for itself.

The alternative is to spawn the review on a CLI of its own, which is what the
per-role commands above are for. A separate process with a separate context is
the thing the declaration is admitting you do not have.

## What a long review needs

`--provider-timeout` defaults to ten minutes. A review of a multi-commit range
does not finish in ten, and a role killed mid-review surfaces as `no report
envelope`, which names the report rather than the timeout. Give it `45m` or more.

Every pass is recorded under `.grimes/work/`, one file per role, appended across
iterations: the command, how it exited, its stdout and its stderr. Written
whether the pass succeeded or failed. Nothing in a role prompt produces it, so
there is no logging instruction to compose and none to get wrong.

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
