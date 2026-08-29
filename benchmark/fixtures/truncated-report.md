| grime-c04 | open | REL | E1 `bash -n` clean, traced dry run shows trap ordering | caller's working directory is restored | caller left in a deleted directory | P1 | none | probe attempted; reproduced |
| grime-d92 | open | INT | E2 `bad-script.sh:14` `TOOLCHAIN_URL="$2"` | arguments are validated before use | unvalidated URL reaches the network | P1 | none | probe attempted |

### Survived Scrutiny (Earned Acquittals)

| Claim or Invariant | Specific Probe Performed | Recorded Result | Scope of Acquittal |
|--------------------|--------------------------|-----------------|---------------------|
| Script fails fast on error | `rg -n '^set ' bad-script.sh` | line 3 `set -euo pipefail` | error handling only |

### Not Examined

| Area or Claim | Why Not Examined | Evidence Needed | Completeness Effect |
|---------------|------------------|-----------------|---------------------|
| Runtime behavior of the deletion path | Sandbox is read-only | A disposable container | caps review_completeness at limited |
