---
description: Start a Grimes Grind - a clinical, pessimistic iteration loop to find everything wrong with an idea, code, or design
arguments:
  - name: target
    description: What to grind (file path, directory, description, or "this" for current context). If omitted, scope will be asked interactively.
    required: false
  - name: scope
    description: "Shorthand scope: 'recent-changes' (git diff), 'whole-repo', or a custom path/description. Skips the scope question."
    required: false
  - name: categories
    description: "Restrict routing to specific canonical categories (COR, INT, SEC, REL, OPS, PER, VER, MNT, DEP, HUM). Default: the skill routes 5-8 itself."
    required: false
  - name: mode
    description: "Output mode: 'report' (report findings only, no edits; default) or 'fix' (apply fixes; requires a verification gate before any commit)"
    required: false
  - name: verify-command
    description: "Command used to verify fixes before committing. Without a usable gate, fix mode edits but never commits."
    required: false
  - name: commit
    description: "Authorize a single commit of the verified fix batch (default false). Requires mode=fix and a gate that exited zero."
    required: false
  - name: max-iterations
    description: Maximum grind iterations before stopping (default 5)
    required: false
  - name: auto-loop
    description: Iterate while each pass still surfaces new P0/P1 findings, up to max-iterations (default false)
    required: false
  - name: research
    description: "Current-landscape research: online (default), offline, or frozen:<path>"
    required: false
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Edit
  - Write
  - Agent
  - AskUserQuestion
  - WebSearch
  - WebFetch
---

# Grimes Grind Command

Execute a Grimes Grind on the target using the grimey methodology.

**Arguments:**
- `target` / `scope`: What to review: file path, directory, `recent-changes`, `whole-repo`, or a description
- `categories`: Restrict routing to canonical categories from COR, INT, SEC, REL, OPS, PER, VER, MNT, DEP, HUM (default: the skill routes 5-8 itself)
- `mode`: `report` (default) or `fix`. `fix` also requires a verification gate, and `--commit` before anything is committed
- `max-iterations`: Stop after N iterations (default 5)
- `auto-loop`: Iterate while passes still yield new P0/P1 findings (default false)
- `research`: `online` (default when available), `offline`, or `frozen:<path>` for a pinned research bundle

**Return format:** a GRIMES_RESULT block carrying the verdict tuple, findings, and verification status

**Examples:**

```bash
# Interactive setup (asks for scope; reports by default)
/frank-grimes:grind

# Report on a single file
/frank-grimes:grind ./src/auth.go

# Recent changes, report only
/frank-grimes:grind --scope recent-changes --mode report

# Fix mode with an explicit gate, committing only if it passes
/frank-grimes:grind ./src/api --mode fix --verify-command "just check" --commit

# Auto-loop, reporting only
/frank-grimes:grind ./proto-mcp --auto-loop
```

---

## 0.0 SESSION SETUP

**PROTOCOL: Determine session configuration before starting. Skip any question whose answer was already provided in `$ARGUMENTS`.**

### 0.1 Scope

- **If `$ARGUMENTS` contains a `target`, `scope`, or a file/directory path:** Use it directly. Skip this step.
- **Otherwise**, use the **AskUserQuestion** tool:
  - question: "What should I grind?"
  - header: "Scope"
  - options:
    ```
    [
      { label: "Recent changes", description: "Review files changed in the last commit or currently staged/unstaged (git diff)" },
      { label: "Whole repo", description: "Scan the entire repository" },
      { label: "Something else", description: "I'll describe the target (file path, code snippet, architecture, etc.)" }
    ]
    ```
  - If "Something else" is selected (or the user writes a custom answer), ask them to describe the target and use their response as the scope.
  - **Scope resolution:**
    - "Recent changes" → run `git diff HEAD` and `git diff --staged` to identify changed files; target those files
    - "Whole repo" → use the repository root as the target
    - Custom → use the provided path/description as the target

### 0.2 Evaluation Categories

- **If `$ARGUMENTS` contains `--categories` or `categories`:** treat the value as a restriction on what the skill may route, using the canonical codes `COR`, `INT`, `SEC`, `REL`, `OPS`, `PER`, `VER`, `MNT`, `DEP`, `HUM`. Record that the user narrowed the routing, since it caps coverage.
- **Otherwise:** pass no restriction and let the skill route from the contract and inventory.

Routing is the skill's job: it selects 5-8 categories from the ten and records a reason for every inclusion and exclusion. Do not ask the user to pre-enable a fixed set: a list chosen before the target is read is not routing, and padding findings to fill it is the anti-pattern the skill exists to prevent.

### 0.3 Mode

- **If `$ARGUMENTS` contains `--mode fix`, `--mode report`, or `mode=`:** Use the provided value. Skip this step.
- **Otherwise** default to `mode=report`. Only ask when the invocation is ambiguous, using the **AskUserQuestion** tool:
  - question: "Should I report findings only, or also apply fixes?"
  - header: "Mode"
  - options:
    ```
    [
      { label: "Report Only (Default)", description: "Identify and document all issues but make no file edits" },
      { label: "Fix", description: "Apply fixes, then run a verification gate. Commits only with explicit --commit and a passing gate." }
    ]
    ```
  - Map "Report Only (Default)" → `mode=report`, "Fix" → `mode=fix`.

### 0.4 Verification Gate (only when `mode=fix`)

Select the gate before making any edit, using the order in the skill's "Fix Mode and the Commit Gate" section:

1. `--verify-command` when supplied.
2. The repository's checked-in aggregate check (for example `just check`).
3. A single documented test or CI command.
4. Otherwise `verification=unavailable`.

Inspect the command before running it. Record which rule selected it. If the gate is `unavailable`, continue in fix mode but state up front that no commit will occur.

---

## 1.0 EXECUTE GRIND

Once scope, categories, and mode are determined, **execute the Grimes Grind inline in this session.** Adopt the grimey persona and run the full methodology defined in SKILL.md with the resolved configuration.

**Resolved configuration:**
- **Target / Scope:** (from step 0.1)
- **Category restriction:** (from step 0.2, or none)
- **Mode:** (from step 0.3): `fix` = apply edits; `report` = document only, no edits
- **Max iterations:** from `$ARGUMENTS` or default 5
- **Auto-loop:** from `$ARGUMENTS` or default false
- **Research mode:** from `$ARGUMENTS` or default `online` when WebSearch/WebFetch are available; otherwise `offline` with the limitation recorded
- **Current iteration:** 1
- **Previous findings:** none (first iteration)

---

## 2.0 EXECUTE THE METHODOLOGY

The methodology is defined once, in `skills/frank-grimes/SKILL.md`. Read it and follow it. Do not restate its phases, categories, evidence tiers, register schema, or verdict rules here; a second copy drifts from the first, and the copy is always the one that goes stale.

This section covers only what this adapter is responsible for: invoking the skill with the resolved configuration, and the two places where a provider capability is required.

### Fix mode and the commit gate

Applies only when `mode=fix`. The rules are in the skill's "Fix Mode and the Commit Gate" section. This adapter supplies:

- The gate command resolved in 0.4, run once over the whole batch via Bash.
- `--commit` as the separate authorization the skill requires before any commit.

Record the command, working directory, exit code, and bounded output. Commit only when fix mode, `--commit`, and a zero exit all hold; otherwise leave edits uncommitted and say so.

### Refutation

Required before a finding can raise confidence, per the skill's Phase 6. This adapter supplies the attacking context: one delegation to the **grimey-refuter** subagent via the Agent tool, carrying every surviving claim and the control the skill requires in a single batch, each as exactly this and nothing else:

```text
Claim: <handle>
Category: <three-letter code>
Anchor: <repository-relative path, document section, argument step, or source URI>
Claim text: <the claim, as written>
Artifact: <repository-relative path or scope>
```

Do NOT include the severity, the grime ID, the evidence you cited, your disproof attempt, the reporter, or your verdict tuple.

Build the control with Grep: choose an identifier of at least twelve characters, confirm it appears nowhere in the artifact, and write it into a claim at the anchor of one of the real ones. Vary where in the batch it goes.

Relay each outcome to the engine against the handle it was issued under. Never relay the control as a finding.

### Independent adjudication

Required before any `pass`, per the skill's "Independent Adjudication" section. The engine runs the pass and resolves the two verdicts. This adapter supplies only the command that starts the second context, on the same `grimes run` invocation that seals the report:

```bash
  --adjudicator-command="claude" \
  --adjudicator-arg="-p" \
  --adjudicator-arg="<the adjudicator prompt>" \
  --adjudicator-fresh \
```

`claude -p` is Claude Code's own non-interactive invocation. `--adjudicator-fresh` is you asserting that it begins a new context.

The prompt is the body of `agents/grimey-verifier.md`. The engine exports the request to that process, so the prompt names no target and carries no digest:

```text
You are adjudicating a target another reviewer has already judged.
Review $GRIMES_TARGET_CONTENT under the skill and reach your own tuple.
End your turn by running, and nothing else:

grimes-contract adjudicate --decision=<d> --residual-risk=<r> \
    --review-confidence=<c> --review-completeness=<x>
```

Do NOT include your verdict tuple, findings, evidence, severities, grime IDs,
proposed fixes, the report, or your reasoning. The verifier's value is that it
has not seen them; contaminating the prompt destroys the only thing it provides.
Your tuple least of all — it is the answer it was asked to reach on its own.

If the adjudicator cannot run, the engine records the absence itself and the verdict caps accordingly. Do not write that outcome by hand.

---

## 3.0 STRUCTURED RETURN

The engine owns the verdict, the iteration, and the ledger. Your job is to hand
it what you found; it decides what that means. Report each surviving finding,
then run the engine over the report.

### Record each survivor

One call per finding that survived the self-grind. The contract validates each
one as it is added, so an error names the finding at fault rather than failing
the whole report:

```bash
grimes-contract report add \
  --category=SEC --severity=P0 --blast=systemic --likelihood=likely \
  --path=<repo-relative path> \
  --tier=E2 --claim="<what the evidence establishes>" \
  --quote="<exact quote in which the defect is visible>"
```

The anchor is one of `--path`, `--document` with `--section`, `--argument` with
`--step`, or `--source` with `--publisher`, `--snapshot-sha256`,
`--retrieved-at`, and optionally `--source-section`. Evidence follows the tier: `--tier=E1` takes `--action`,
`--cwd`, `--exit-code`, and `--output`; `--tier=E2` takes `--quote`; `--tier=E3`
takes `--assumption`, `--reasoning`, and `--falsifier`. Add `--suggested-fix` to
record a fix as text without applying it.

Every finding records what was done to disprove it, or why that could not be
done:

```bash
  --disproof-action="<command>" --disproof-exit=<n> --disproof-output="<excerpt>" \
  [--disproof-contradicts]
  # or, when it could not be attempted:
  --disproof-unavailable="<what was missing>"
```

The two forms are exclusive. The skill governs when a disproof is required and
what follows from its result.

A rejected call prints the contract rule it failed. The skill governs what to do
about it.

### Account for the target

`$GRIMES_TARGET_INVENTORY` holds this run's units and `report units` prints their
ids exactly. A unit id is arbitrary text, so read it from there rather than
parsing any rendering of it. The skill says what to account for; these are the
calls that record it.

```bash
grimes-contract report units --print0 |
  grimes-contract report cover --examined-stdin0

grimes-contract report cover --skip="<unit id>" --skip-reason="<why>" [--skip-material]
grimes-contract report stop --category=SEC \
  --condition=<marginal-yield|probes-exhausted|evidence-unavailable> --probes=<n>
```

`--examined-stdin0` is the only form safe for an id containing a comma or a
newline. `--skip` takes one unit per call, so an id containing any delimiter
survives.

An acquittal the skill says you earned is recorded with `report acquit`:

```bash
grimes-contract report acquit --category=SEC --path=<file> \
  --claim="<claim>" --scope="<scope>" \
  --probe-action="<command>" --probe-exit=<n> --probe-output="<excerpt>" \
  --control-mutation="<mutation>" --control-action="<command>" \
  --control-exit=<n> --control-output="<excerpt>"
```

The `--control-*` flags go together or not at all, and the command derives the
control's outcome from `--control-exit` rather than taking it as a flag. The
skill governs when a control is required and what it has to show.

### Seal and run

Sealing happens inside the run, as the provider command: the engine exports the
run identity to the provider it invokes, and `report seal` reads it from there,
so the flags below carry only what the caller supplies.

`$GRIMES_TARGET_CONTENT` is where the engine put the bytes under review: a
directory when the target is a tree, a file when it is one file, and a code
target may be either. Read the target from there rather than from the scope,
which for a pasted argument names no path at all.

```bash
grimes run --dir=. \
  --provider-command="grimes-contract report seal" \
  --provider-arg="--routed=<COR,SEC,...>" \
  --provider-arg="--examined=<N>" \
  --provider-arg="--disproved=<K>" \
  --provider-arg="--summary=<one-sentence BLUF>" \
  --adjudicator-command="claude" \
  --adjudicator-arg="-p" \
  --adjudicator-arg="<the adjudicator prompt>" \
  --adjudicator-fresh \
  <target>
```

`--provider-command` is split on whitespace, so anything containing a space —
a summary, most obviously — goes in its own `--provider-arg`, which is not
split. Quoting inside `--provider-command` does not survive the split.

The target goes last: flags after it are not parsed. Pass `--auto-loop` only when
the caller asked for it; it is off by default.

`--examined` and `--disproved` are the self-grind counts the skill defines. They
are recorded, not checked.

You are the provider, so the engine cannot call you. It reads what you wrote and
treats it as untrusted input: it recomputes each finding's identity from its own
anchor and evidence, admits it to the ledger, derives the verdict tuple and the
colour, and counts what this pass surfaced that the last did not.

### What you must not write

Do not write a verdict, a colour, a finding ID, a timestamp, loop state, or a
result file. A review that certifies its own pass is not a review, and a
hand-written record is rejected rather than believed. There is no field in the
report for any of them.

Do not change the iteration either. The engine advances it, and two components
incrementing one counter skips iterations and corrupts the cap that makes the
loop terminate.

End your turn after the engine returns. If another iteration is warranted, the
stop hook will start one.

---

**Begin Phase 1 now.**
