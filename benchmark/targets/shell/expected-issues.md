# Expected Issues for bad-script.sh

This file documents the flaws that a good Grimes Grind should find in `bad-script.sh`.

## Security Issues (P0/P1)

1. **Hardcoded production server credentials** - `scp` and `ssh` use `root@prod-server` with no key management, no SSH config, no credential handling. If this script is committed, the server address is exposed.

2. **No input validation on VERSION** - The script uses `$VERSION` directly in `git tag` without sanitization. A malicious version string like `v1.0; rm -rf /` could execute arbitrary commands.

3. **Running as root on production** - `ssh root@prod-server` and `scp` to root's home directory. No principle of least privilege.

## Reliability Issues (P0/P1)

4. **No error handling** - The script continues after every command regardless of success/failure. If `go build` fails, the script still tries to `scp` a non-existent binary.

5. **Kill all matching processes** - `ps aux | grep app | awk '{print $2}' | xargs kill` kills ALL processes matching "app", including the grep process itself and any unrelated processes. No graceful shutdown, no PID file, no signal handling.

6. **No rollback mechanism** - If the deployment fails, there's no way to revert. No previous version kept, no blue-green deployment, no feature flag.

7. **No verification after deployment** - Script says "Deployment complete!" without checking if the service actually started, is healthy, or is serving requests.

## Correctness Issues (P1/P2)

8. **Race condition in process kill** - Between killing the process and copying the new binary, there's a window where no service is running. No atomic swap.

9. **scp/ssh without host key verification** - No `StrictHostKeyChecking` configuration. Vulnerable to man-in-the-middle attacks on first connection.

10. **Binary built locally, deployed remotely** - The Go binary is built on the local machine and copied to the server. If architectures differ (local is macOS ARM, server is Linux AMD64), the binary won't run.

## Maintainability Issues (P2/P3)

11. **No logging** - All output goes to stdout with no log file, no timestamp, no structured logging. Debugging a failed deployment means re-running the script.

12. **Hardcoded paths** - `/opt/app/`, `./bin/app`, `./cmd/main.go` are all hardcoded. No configuration for different environments.

13. **No idempotency** - Running the script twice could cause issues (killing already-dead processes, re-pushing tags).

14. **No timeout on ssh/scp** - If the network hangs, the script hangs indefinitely.

## Edge Cases (P2)

15. **VERSION could contain special characters** - Git tags have restrictions, but the script doesn't validate. A version like `v1.0-beta/2` might fail.

16. **No check if binary exists before scp** - If build fails, scp fails with a confusing error message.

## What a Good Grind Should Find

A thorough grind should identify at least 10-12 of these issues, with evidence (specific lines from the script), proper severity ratings, and a justified verdict (likely RED given the P0 security and reliability issues).
