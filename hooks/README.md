# Stop Hook Integration

The Grimes Grind auto-loop uses a stop hook to intercept exit attempts and continue iterating. The hook owns the loop: it decides whether another pass happens, increments the counter, and re-injects the prompt. The grind itself must never advance the iteration.

Iteration stops when a pass is confirmed, when an iteration surfaces no new P0/P1 findings, or when the cap is reached. Verdict rules live in the skill, not here.

## How It Works

1. During a grind, the agent writes state to `.grimes-state.json` in the project root
2. On session stop, the agent's hook system calls `hooks/stop.sh`
3. The hook reads the state file and decides:
   - **Exit 0**: Allow the session to end (pass confirmed, no new P0/P1 findings, max iterations reached, or auto-loop disabled)
   - **Exit 2**: Block exit and re-inject the grind prompt (continue iterating)
4. If continuing, the hook increments the iteration counter and outputs the next prompt

## State File Format

```json
{
  "iteration": 2,
  "max_iterations": 5,
  "last_verdict": "YELLOW",
  "target": "./src/auth.py",
  "auto_loop": true,
  "issues_found": 8,
  "issues_fixed": 3,
  "new_p0_p1": 2,
  "last_commit": "abc1234",
  "last_grind_timestamp": "2026-08-21T10:30:00Z"
}
```

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
1. Running the grind manually
2. Reading `.grimes-state.json` after each iteration
3. If verdict is not GREEN and iterations remain, re-invoking the grind with the updated state

## Dependencies

- `jq`: required for JSON parsing in the stop hook

Install on macOS: `brew install jq`
Install on Ubuntu/Debian: `apt-get install jq`
Install on Fedora: `dnf install jq`

## Troubleshooting

**Hook not triggering**: Verify your agent's hook system is configured to call `hooks/stop.sh` on stop events.

**State file not found**: The state file is created when a grind starts with `auto_loop: true`. If you're not using auto-loop, the hook will exit cleanly without finding state.

**jq not found**: Install jq. The hook cannot parse JSON without it and will exit cleanly with a warning.

**Corrupted state**: If the state file is invalid JSON or missing required fields, the hook removes it and exits cleanly. Start a new grind to recreate it.
