#!/usr/bin/env bash
# Dies the way a provider that sandboxes its own commands dies inside the
# engine's boundary. The message is the kernel's, not any provider's.
set -euo pipefail
echo "sandbox-exec: sandbox_apply: Operation not permitted" >&2
exit 0
