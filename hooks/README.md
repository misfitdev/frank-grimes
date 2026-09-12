# Stop Hook Integration

The Grimes Grind auto-loop uses a stop hook to intercept exit attempts and continue iterating.

The hook decides nothing. The engine owns the loop: it derives the verdict, advances the iteration, and writes the run record. The hook locates the repository and the binary, asks the engine, and reports the answer. Verdict rules live in the skill, not here.

Iteration stops when a pass is confirmed, when an iteration surfaces no new P0/P1 findings, or when the cap is reached.

## How It Works

1. A grind runs through the engine, which writes `.grimes/state.pb`, `.grimes/result.pb`, and `.grimes/ledger.pb`
2. On session stop, the agent's hook system calls `hooks/stop.sh`
3. The hook invokes `grimes loop`, which verifies the record and decides:
   - **Exit 0**: Allow the session to end (pass confirmed, no new P0/P1 findings, cap reached, or the record could not be verified)
   - **Exit 2**: Block exit and re-inject the grind prompt (continue iterating)

## The Run Record

The record is protobuf, not JSON, and the engine writes it. A hand-written verdict is rejected rather than believed.

`grimes loop` refuses to treat a result as terminal unless its digest is the one `.grimes/state.pb` recorded, its run id matches, its target fingerprint matches, and it declares contract major 2 and orchestrator authorship. A GREEN result additionally has to satisfy the contract's own rule that a pass carries an independent review of the same target with no oscillation.

Anything that fails those checks is quarantined under `.grimes/quarantine/` and the session ends without a pass recorded. Inspect the current record with:

```bash
grimes state --show     # the loop state, if a review is in progress
grimes state --clear    # discard it
```

## Locating the Repository

An installed hook runs from a versioned plugin cache, so its own path says nothing about which repository is under review. The hook resolves the project from `GRIMES_PROJECT_DIR`, then `CLAUDE_PROJECT_DIR`, then the working directory, and only then falls back to its own parent. Set one of the first two if your agent runs the hook from outside the repository.

## Integration by Provider

### Claude Code

Place the hook configuration in your Claude Code plugin or project settings:

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "hooks/stop.sh",
            "timeout": 600,
            "statusMessage": "Checking if grind loop should continue"
          }
        ]
      }
    ]
  }
}
```

See `adapters/claude-code/hooks.json` for a ready-to-use configuration.

### OpenCode

OpenCode supports hook configuration via `AGENTS.md` or plugin manifests. Add a stop hook that calls the script:

```yaml
hooks:
  stop:
    - command: hooks/stop.sh
      timeout: 600
```

See `adapters/opencode/AGENTS.md` for details.

### Codex

Codex supports hook configuration in `openai.yaml` or project settings. Configure a stop hook:

```yaml
hooks:
  stop:
    - command: hooks/stop.sh
      timeout: 600
```

See `adapters/codify/AGENTS.md` for details.

### Other Providers

Any agent framework that supports stop hooks can integrate with this script. The contract is simple:

1. Call `hooks/stop.sh` on session stop
2. Read stdout for the continuation prompt (if exit code is 2)
3. Respect the exit code: 0 = allow exit, 2 = continue

If your provider doesn't support hooks, you can simulate the loop by:
1. Running the grind through `grimes run`
2. Running `grimes loop` after each iteration
3. Re-invoking the grind while it exits 2

## Dependencies

- `grimes`: the engine binary, found next to the hook, in `bin/`, or on `PATH`

If the binary is missing the hook allows the session to end and records no pass.

## Troubleshooting

**Hook not triggering**: Verify your agent's hook system is configured to call `hooks/stop.sh` on stop events.

**State file not found**: The record is created when a review runs through the engine. If the hook reports nothing, check that it resolved the right repository; set `GRIMES_PROJECT_DIR` if it did not.

**grimes not found**: Install the binary or put it on `PATH`. Without it the hook cannot verify a run, so it ends the session and records no pass.

**Corrupted or unverifiable record**: The hook quarantines it under `.grimes/quarantine/` rather than deleting it, and ends the session without a pass. The quarantined file is the only evidence of how the run broke.
