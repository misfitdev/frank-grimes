# Non-Code Target Mappings

Read only the target type selected in the review contract. These mappings define what the ten categories mean for that target; the routed attack card still supplies the probes and evidence rules. A category may be excluded only under Phase 1's P0-routing rule.

## Architecture

| Category | Claims to enumerate and attack |
|----------|--------------------------------|
| COR | Required system behaviors, state transitions, consistency model, and invariants across components. |
| INT | Protocols, schemas, trust-boundary crossings, data ownership, compatibility, and integrity across interfaces. |
| SEC | Identities, authorization boundaries, tenant/data isolation, secrets, privacy, abuse cases, and control obligations. |
| REL | Failure domains, partial failure, retry/idempotency ownership, ordering, concurrency, recovery, and resource exhaustion. |
| OPS | Deploy topology, configuration, health signals, migration, rollback, restoration, and operator visibility. |
| PER | Workload assumptions, capacity limits, fan-out, contention, latency budgets, storage growth, and cost multipliers. |
| VER | Executable models, prototypes, contract tests, failure drills, acceptance criteria, and evidence for critical claims. |
| MNT | Sources of truth, coupling, evolution paths, configuration ownership, and blast radius of change. |
| DEP | External services, protocols, version assumptions, supplier failure, quotas, portability, and exit paths. |
| HUM | Operator actions, approval boundaries, dangerous defaults, handoffs, misuse, and recovery from human error. |

## Incident

| Category | Claims to enumerate and attack |
|----------|--------------------------------|
| COR | Timeline, observed symptoms, causal chain, root-cause claim, and whether the fix blocks the demonstrated mechanism. |
| INT | Inputs, data transitions, corrupt records/messages, boundary mismatches, and propagation between affected components. |
| SEC | Compromise indicators, privilege use, containment boundaries, sensitive exposure, evidence preservation, and disclosure duties. |
| REL | Trigger, amplification, retry/cascade behavior, failover, partial recovery, recurrence paths, and fix-induced failure modes. |
| OPS | Detection delay, alert/action path, mitigation commands, rollback, restoration, runbook accuracy, and recovery evidence. |
| PER | Saturation, traffic/resource curves, queue growth, cost spike, capacity margin, and whether load was cause or symptom. |
| VER | Reproducer, timeline corroboration, counterfactual test, fix verification, regression test, and recurrence monitor. |
| MNT | Contributing complexity, configuration drift, ownership gaps, stale documentation, and change hazards exposed by the incident. |
| DEP | Provider/library/service contribution, contract change, degraded mode, dependency evidence, and fallback behavior. |
| HUM | Decision context, confusing signals, handoffs, access friction, incentives, unsafe defaults, and learning actions without authorship blame. |

## Process

| Category | Claims to enumerate and attack |
|----------|--------------------------------|
| COR | Intended outcome, entry/exit criteria, required state transitions, exceptions, and proof the process produces its claimed result. |
| INT | Handoffs, forms/data, queues, role boundaries, validation, record integrity, and incompatible upstream/downstream expectations. |
| SEC | Role separation, least privilege, approvals, sensitive records, auditability, bypass paths, and control obligations. |
| REL | Single-person dependencies, missed handoffs, retries/escalations, overload, absence, exception handling, and recovery from partial completion. |
| OPS | Ownership, status visibility, escalation signals, rollback/correction, record retention, and ability to detect stalled or failed execution. |
| PER | Cycle time, queue growth, throughput, bottlenecks, duplicated labor, cost, and behavior under peak demand. |
| VER | Measurable success criteria, sampled records, dry runs, exception drills, control tests, and evidence that the process is followed. |
| MNT | Policy ownership, versioning, duplicated guidance, change communication, training burden, and stale-artifact risk. |
| DEP | Vendor/team/tool prerequisites, service levels, external approvals, contingency paths, and assumptions outside process ownership. |
| HUM | Incentives, cognitive load, ambiguity, workarounds, accessibility, authority gradients, misuse, and recoverability of mistakes. |

## Proposal

| Category | Claims to enumerate and attack |
|----------|--------------------------------|
| COR | Problem statement, causal model, promised outcome, success criteria, constraints, and contradictions between claims and mechanism. |
| INT | Stakeholder boundaries, required inputs/outputs, data flows, adoption interfaces, compatibility, and integrity of exchanged information. |
| SEC | Abuse cases, access/data implications, privacy, regulatory claims, control costs, and who gains dangerous capability. |
| REL | Failure scenarios, reversibility, contingency, phased exposure, dependency failure, and outcome under incorrect assumptions. |
| OPS | Ownership, delivery path, rollout, support, monitoring, rollback/exit, and evidence needed to operate the proposal after approval. |
| PER | Demand assumptions, unit economics, capacity, schedule/resource constraints, scaling curves, and downside cost. |
| VER | Falsifiable hypothesis, baseline, experiment, acceptance threshold, counter-metric, sample, and decision date. |
| MNT | Ongoing ownership, policy/documentation burden, change cost, accumulated complexity, and decommission path. |
| DEP | Market/vendor/team/legal/technical assumptions, external commitments, substitute options, and contract fragility. |
| HUM | User incentives, adoption friction, gaming, misuse, training, stakeholder opposition, and who bears failure consequences. |
