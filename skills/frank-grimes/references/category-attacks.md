# Category Attack Cards

Read only the cards routed by Phase 1. A probe is an action plus a recorded result, not a question left unanswered. Probes are ordered options governed by Phase 5's marginal-yield rule, not a completion checklist. Commands below are selection rules: execute only checked-in commands or tools already available in the environment, never install a tool or use the network for a local probe. Current-landscape research is governed separately by the research phase and [references/research.md](research.md).

## COR — Correctness and invariants

**Enumerate:** stated outputs, state transitions, boundary conditions, cross-file invariants, and the call paths that can violate them.

**Cheapest decisive probes, in order:**

1. Run the repository's existing focused unit or conformance test for the changed behavior; record command, exit code, and bounded output.
2. Trace one normal case and the `empty`, `one`, boundary, and invalid cases from entry to observable result; cite every branch crossed.
3. Construct the smallest local input or fixture that contradicts a critical invariant and execute it with an existing test runner or executable.
4. Compare duplicated producers/consumers of the same rule or schema and quote the first incompatible pair.

**E1 here:** a focused test, executable, or recorded deterministic trace produces an output or state transition that violates the named invariant.

**False-accusation trap:** treating a surprising implementation as incorrect without first establishing the contract, reachable path, and required result.

## INT — Interfaces, input handling, and data integrity

**Enumerate:** entry points, parsers, serializers, validation order, type/unit/encoding boundaries, persistence writes, and producer-consumer pairs.

**Cheapest decisive probes, in order:**

1. Run existing parser, schema, API, or integration tests and record command, exit code, and bounded output.
2. Trace one value from its least-trusted source through validation and conversion to each sink; cite where its type, unit, encoding, or constraints change.
3. Execute existing fixtures with empty, oversized, malformed, duplicate, reordered, and boundary values where the interface accepts them.
4. Round-trip one representative record through the existing serializer/parser or writer/reader and compare the observable fields.

**E1 here:** an executed request, fixture, or round trip is accepted, rejected, corrupted, truncated, or reinterpreted contrary to the stated interface contract.

**False-accusation trap:** calling absence of validation at one layer a defect when an enforced upstream boundary is both in scope and demonstrably unavoidable.

## SEC — Security, privacy, and compliance

**Enumerate:** trust boundaries, identities, authorization decisions, attacker-controlled data, secrets, sensitive data, privileged sinks, retention/audit claims, and tenant boundaries.

**Cheapest decisive probes, in order:**

1. Run checked-in security tests or configured local scanners first; record command, exit code, configuration used, and bounded output.
2. Trace one unauthorized actor and one hostile input from entry to privileged action or sensitive sink; cite each enforcement decision.
3. Execute an existing local request/test while removing identity, changing resource ownership, or crossing a tenant boundary; record status and response.
4. Search the scoped artifacts for credential material and sensitive-data sinks with a recorded `rg` pattern, then inspect every hit before classifying it.
5. Compare declared privacy/compliance controls with the exact code, configuration, log, or process artifact that implements each control.

**E1 here:** an executed local test demonstrates unauthorized access, injection, secret disclosure, tenant crossing, missing audit behavior, or a security control rejecting the attack.

**False-accusation trap:** asserting exploitability from a dangerous-looking sink without proving attacker control, reachability, missing mediation, and relevant deployment conditions.

## REL — Reliability, error handling, concurrency, and resource lifecycle

**Enumerate:** failure sources, error paths, timeouts, retry ownership, idempotency, concurrency/ordering assumptions, resource acquisition/release, shutdown, and partial-state behavior.

**Cheapest decisive probes, in order:**

1. Run existing failure-path, race, integration, or recovery tests supported by the repository; record command, exit code, and bounded output.
2. Force one available dependency, input, or operation to fail using an existing fixture or test seam and record the returned error, state, and cleanup.
3. Repeat or reorder the same supported local operation to test idempotency and ordering; compare outputs and persistent state.
4. Trace each acquired resource and spawned task to normal, error, cancellation, and shutdown termination paths; cite any path without a terminal action.
5. Execute an existing concurrency test or bounded repeated run and record hangs, races, duplicate effects, or stable completion.

**E1 here:** an executed failure, cancellation, repeated operation, or concurrent run demonstrates a crash, hang, leak, corruption, duplicate effect, or successful containment.

**False-accusation trap:** treating a language pattern as universally unsafe without demonstrating ownership, lifetime, concurrency, and an actual bad path.

## OPS — Operability, observability, deployment, and recovery

**Enumerate:** deploy steps, configuration rollout, health signals, logs/metrics/traces, alert ownership, rollback/recovery paths, migrations, and failure visibility.

**Cheapest decisive probes, in order:**

1. Run checked-in build, packaging, configuration-validation, or smoke commands that work locally; record command, exit code, and bounded output.
2. Trigger one available failure fixture and record whether the documented signal identifies the failed component and affected request or operation.
3. Trace a rollout and rollback through checked-in manifests/scripts, quoting the version, configuration, and state transitions each actually performs.
4. Execute an existing dry-run, status, restore, or migration verification command; record status and output without mutating production state.

**E1 here:** a local build/smoke/dry-run or failure fixture demonstrates deploy failure, invisible failure, non-reversible state, broken recovery, or a working operational control.

**False-accusation trap:** imposing service-level observability or rollback requirements on a target whose review contract has no deployed runtime or mutable operational state.

## PER — Performance, scalability, and cost

**Enumerate:** workload dimensions, asymptotic operations, fan-out, allocation/storage growth, external calls, contention points, quotas, and cost multipliers.

**Cheapest decisive probes, in order:**

1. Run existing benchmarks, load fixtures, or profiling commands with their checked-in parameters; record command, exit code, input size, and bounded output.
2. Count work, allocations, queries, or external calls for two available input sizes and record the measured growth.
3. Execute the largest checked-in fixture through the normal local path and record time, memory/output size, and termination status using available tools.
4. Trace fan-out and retry multiplication from one request and cite the configured bounds or absence of bounds.

**E1 here:** an executed benchmark or measured local run shows a reproducible resource curve, limit breach, unbounded fan-out, or stable behavior at a contract-relevant size.

**False-accusation trap:** declaring a bottleneck or cost crisis from big-O intuition alone without a relevant workload, measured behavior, or a reachable unbounded path.

## VER — Verification and testability

**Enumerate:** critical invariants, tests mapped to them, assertions, fixtures, negative paths, determinism controls, integration boundaries, and release gates.

**Cheapest decisive probes, in order:**

1. Run the repository's documented local test command and record command, exit code, skipped tests, and bounded output.
2. Map each contract-critical invariant to a named test and quote the assertion that would fail if the invariant broke.
3. Make a temporary, reversible perturbation in a disposable copy or through an existing fault fixture and run the mapped test; record whether it fails, then leave the target unchanged.
4. Run the narrow test twice under the same checked-in conditions and record inconsistent results, hidden prerequisites, or stable completion.

**E1 here:** an executed test fails on current behavior, survives a falsifying perturbation, flakes under identical conditions, or correctly detects the injected fault.

**False-accusation trap:** equating test count, file presence, or coverage percentage with verification of the actual critical invariant.

## MNT — Maintainability, configuration, duplication, and change safety

**Enumerate:** sources of truth, repeated policy/logic, configuration ownership, coupling, generated versus maintained artifacts, dead paths, and change surfaces for critical behavior.

**Cheapest decisive probes, in order:**

1. Run existing format, lint, type, generation-drift, or static-analysis commands; record command, exit code, and bounded output.
2. Search for each critical constant, rule, or schema with a recorded `rg` command and inspect whether multiple hits are independent or must change together.
3. Trace one representative requirement change and list every artifact that must be edited; quote incompatible or undocumented sources of truth.
4. Run an existing generator or consistency check in a disposable worktree/copy and record drift without overwriting user changes.

**E1 here:** an existing checker reports drift/structural failure, or a controlled consistency check demonstrates that one required change leaves another authoritative artifact stale.

**False-accusation trap:** reporting duplication, a hard-coded value, or non-idiomatic style without showing divergent ownership, a real change hazard, or an enforced repository rule. Trace precedence before accusing: a caller deliberately overriding a default leaves one effective value and is not split ownership without contrary contract evidence.

## DEP — Dependencies, supply chain, and external contracts

**Enumerate:** direct dependencies, lockfiles, vendored/generated inputs, version constraints, licenses when claimed, external APIs, feature flags, and fallback behavior.

**Cheapest decisive probes, in order:**

1. Run existing offline dependency, lockfile, generation, or vulnerability checks configured by the repository; record command, exit code, database/cache condition, and bounded output.
2. Compare manifests to lockfiles/vendor metadata with checked-in validation commands or exact citations and record mismatches.
3. Trace every used external symbol or response field to the pinned declaration, vendored schema, generated client, fixture, or checked-in contract.
4. Disable or fail an external dependency through an existing local seam and record fallback, propagated error, and resulting state.

**E1 here:** an executed offline check or local failure fixture demonstrates unresolved/pinned drift, incompatible contract use, compromised metadata, broken fallback, or a clean dependency state.

**False-accusation trap:** claiming a version, API, license, or vulnerability fact from memory when it is not established by the scoped local artifacts and available offline data.

## HUM — Human factors, misuse, and operational process

**Enumerate:** actors, permissions, incentives, handoffs, high-consequence actions, defaults, warnings, approval/override paths, cognitive load, and recovery from operator error.

**Cheapest decisive probes, in order:**

1. Walk the shortest documented path for a novice and for a rushed privileged actor, recording each decision and irreversible action.
2. Execute any local CLI/UI fixture with omitted, ambiguous, repeated, and mistaken inputs; record defaults, warnings, confirmations, and recovery behavior.
3. Trace one required approval or handoff through the artifact and cite where ownership and completion become observable.
4. Run an existing dry-run or rollback path after a plausible operator mistake and record whether the actor can detect and recover from it.

**E1 here:** an executed interaction or dry-run demonstrates an unsafe default, silent misuse, bypassed approval, unrecoverable operator error, or an effective guardrail.

**False-accusation trap:** substituting reviewer preference for evidence about the named actor, task, consequence, and observed interaction path.
