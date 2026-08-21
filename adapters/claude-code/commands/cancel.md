---
description: Cancel an active Grimes Grind loop or manage session state
arguments:
  - name: session-id
    description: "Session ID to cancel (optional, defaults to current session)"
    required: false
  - name: list
    description: "Set to 'all' to list all active grinds in .grimes/sessions/"
    required: false
allowed-tools:
  - Bash
  - Read
---

# Cancel Command

Terminates an active Grimes Grind loop for the current or specified session.

## Execution

### Default (no arguments): Terminate Current Session Grind

1. Determine session ID:
   - Use `CLAUDE_SESSION_ID` if available.
   - Otherwise, use MD5 hash of current working directory.
2. Verify existence of state file: `.grimes-state.json`.
3. If found:
   - Read and report clinical summary (iterations, verdict, issue counts).
   - Delete the state file.
4. If not found:
   - Report: "No active grind found for this session."

### Terminate Specific Session (--session SESSION_ID)

1. Verify state file for the provided ID.
2. If found:
   - Report summary and delete file.
3. If not found:
   - Report: "Session not found."

### List All Active Grinds (--list all)

1. Enumerate all state files in `.grimes/sessions/`.
2. For each file, report: Session ID, Iteration, Last Verdict, Target.

## Output Examples

**Terminate active grind (current session):**
```
Grimes Grind terminated.

Session: abc123def456
Status:
- Iterations: 2 of 5
- Last Verdict: YELLOW
- Issues identified: 8
- Issues mitigated: 3

State file removed.
```

**List all active grinds:**
```
Active Grimes Grinds:

- Session: abc123def456 | Iteration: 2/5 | Verdict: YELLOW | Target: auth_logic.py
- Session: xyz789abc123 | Iteration: 1/5 | Verdict: RED    | Target: "New deployment plan"

Total: 2 active grinds.
```
