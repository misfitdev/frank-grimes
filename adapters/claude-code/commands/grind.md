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

### Independent adjudication

Required before any `pass`, per the skill's "Independent Adjudication" section. This adapter supplies the second context: delegate to the **grimey-verifier** subagent via the Agent tool, passing exactly this and nothing else:

```text
Target: <repository-relative path or scope>
Target digest: <git rev-parse HEAD, or a content hash of the reviewed scope>
Claimed verdict tuple:
  decision: <block|conditional|pass>
  residual risk: <critical|high|moderate|low|unknown>
  review confidence: <high|medium|low>
  review completeness: <sufficient|limited|inconclusive>
```

Do NOT include findings, evidence, severities, grime IDs, proposed fixes, the report, or your reasoning. The verifier's value is that it has not seen them; contaminating the prompt destroys the only thing it provides.

Resolve the two verdicts by the skill's table: the stricter decision wins, and an independent pass never upgrades your own `block` or `conditional`. If the verifier cannot run, record `Independent adjudication: not available`, set `review_confidence=low`, and cap at `conditional`/YELLOW.

---

## 3.0 STRUCTURED RETURN

After the verdict is derived, output the following structured result. This is non-negotiable: it enables auto-loop orchestration:

```
GRIMES_RESULT: {
  "iteration": <current iteration number>,
  "max_iterations": <maximum allowed iterations>,
  "verdict": "GREEN|YELLOW|RED",
  "issues_found": <count of total issues identified>,
  "issues_fixed": <count of VERIFIED closures only; never the count of edits>,
  "verification": {
    "status": "passed|failed|unavailable|not_applicable",
    "command": "<exact command, or null>",
    "exit_code": <integer, or null>,
    "selected_by": "explicit|repo_check|documented_command|none"
  },
  "grime_findings": [
    {
      "grime_id": "grime-xxx-123",
      "category": "Category name",
      "severity": "P0|P1|P2|P3",
      "status": "VERIFIED|FIXED|UNFIXED",
      "evidence": "Specific code path, scenario, or evidence",
      "fix_applied": "Description of fix applied, or null if unfixed"
    }
  ],
  "commit_hash": "abc1234... or null",
  "summary": "One-sentence BLUF describing the verdict"
}
```

Then update the state file at `.grimes-state.json`: set `last_verdict`, `issues_found`, `issues_fixed`, `new_p0_p1`, `last_commit`, `last_grind_timestamp`. Preserve all other fields, and never write `iteration`; that field belongs to the hook.

`new_p0_p1` is the count of P0/P1 findings this iteration surfaced that the previous iteration did not. It is how the hook knows whether another pass is yielding anything; report `0` honestly when an iteration turned up nothing new.

**Do not start the next iteration yourself, and do not change `iteration`.** The stop hook owns the loop: it decides whether another pass happens, increments the counter, and re-injects the prompt. Two components incrementing the same counter skips iterations and corrupts the cap that makes the loop terminate.

End your turn after emitting the result. If another iteration is warranted, the hook will start it.

---

**Begin Phase 1 now.**
