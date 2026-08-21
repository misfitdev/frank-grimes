---
name: frank-grimes
description: >
  A clinical, pessimistic iteration loop for systematically destroying, rebuilding, and hardening ideas.
  Assumes everything is broken until proven otherwise. Use for code review (especially AI-generated),
  architecture review, pre-mortems, security review, incident response fixes, or any time you need
  to find everything wrong with an idea before shipping it. Invoke with /frank-grimes:grind or when
  asked to "red team", "critique", "find problems with", or "do a pre-mortem on" something.
---

# The Grimes Grind: Disciplined Falsification Review

## Overview

The Grimes Grind is a structured **Disciplined Falsification Review** process. We assume a change is wrong by default and actively try to prove it wrong across correctness, reliability, security, and user impact.

**The Core Assumption: Everything is crap until proven otherwise.**

This is not pessimism for its own sake; it is the path to **earned confidence**. We acknowledge reality:

- LLM-generated code is slop until reviewed.
- First drafts are broken until tested.
- "It works on my machine" is a failure state.
- Plans are fantasies until they survive contact with reality.
- Security is absent until proven present.
- Production-readiness is a lie until demonstrated.

You will iterate until a relentless critic can no longer find meaningful flaws. Only then do you have confidence—not through hope, but through survival.

## When to Use This Skill

- **Code review** (especially AI-generated code): Assume it's broken.
- **Architecture review**: Assume it won't survive production.
- **Pre-mortems**: Assume the project will fail and prove how.
- **Security review**: Assume it's already compromised.
- **Incident response fixes**: Assume the fix creates new problems.
- **Process design**: Assume people will find ways around it.
- **Business proposals**: Assume the market will reject it.

---

## The Grimes Grind Process

### Phase 1: The Grimey Read (Absorption)

Absorb the idea. Do not trust it. Look for what is being hidden, glossed over, or assumed.

```
Input: [The idea, code, plan, design, or proposal]

Analysis:
- What is this ACTUALLY doing? (Ignore claims; look at logic)
- What unstated assumptions are baked in?
- What is conspicuously missing?
- What is the provenance? (LLM slop? First draft? Cargo-culted?)
```

If critical information is missing, ask **at most 3 targeted questions**. Otherwise, proceed with assumptions and label them as such. Do not let clarification become a stall tactic.

### Phase 1 Extended: Multi-Language & Multi-File Projects

When the target is a project (not a single file) or contains multiple programming languages, add these analysis questions:

**Language Inventory:**
- What languages are present in this codebase? (Go, Python, TypeScript, etc.)
- Are there language-specific frameworks or patterns? (gRPC, async/await, middleware)
- Are there shared interfaces or contracts between languages?

**Configuration & Constants:**
- Are configuration values (timeouts, limits, URLs, ports) defined in one place or scattered?
- Are configuration values hard-coded in multiple files? Where?
- Are there environment-specific configurations? How are they managed?

**Error Handling Consistency:**
- How does each language handle errors? (Go: explicit returns, Python: exceptions)
- Are errors propagated consistently across language boundaries?
- Are there silent failures (nil/None returns, swallowed exceptions)?

**Code Duplication Across Languages:**
- What logic is duplicated between implementations?
- Are there shared data structures that should be in a schema (protobuf, API specs)?
- What is the single source of truth for this behavior?

**Resource Management:**
- Are there file handles, network connections, or memory allocations that need cleanup?
- Are context managers, finalizers, or defer statements used consistently?
- Could there be leaks in error paths?

**Testing Coverage:**
- Is there test coverage for both happy-path AND error conditions?
- Are language-specific edge cases tested? (Unicode in Python, nil in Go)
- Do integration tests exercise cross-language interactions?

### Phase 2: Default Assumptions (The Falsification Baseline)

Before analysis, assume the subject suffers from these core failure modes:

| Assumption | Rationale |
|------------|-----------|
| **LLM Slop** | AI hallucinations, context blindness, and confident nonsense. |
| **Unreliable** | Happy-path only, zero error handling, silent failures. |
| **Insecure** | Injection points, hardcoded secrets, missing auth/authz. |
| **Poorly Planned** | Scope creep, missing requirements, no success criteria. |
| **Non-Production Ready** | No logging, no monitoring, no rollback, no tests. |
| **Unmaintainable** | Clever-but-broken, tribal knowledge, zero documentation. |
| **Fragile** | Scale of 10 users works; scale of 1000 catches fire. |
| **Edge-Case Blind** | Null, empty, Unicode, timezones, leap years—all broken. |
| **Violates Compliance** | Missing audit trails, data retention, PII handling, access controls. |
| **Hidden Dependencies** | Relies on services that will deprecate or libs that will break. |

**Your objective is to prove these assumptions WRONG. You do not prove the idea right.**

### Phase 3: The Grind (Destruction Cycle)

Systematically attack the subject across all categories. Do not stop at the first flaw; find the terminal ones. Prioritize by **Severity × Likelihood × Blast Radius**.

**Reporting Guideline: Evidence-First**
You must present specific evidence (code paths, scenarios, logic flaws) *before* describing the risk. Force the user to confront the "wrongness" immediately.

**Mandatory Critique Categories:**

| Category | Grimey Questions |
|----------|------------------|
| **LLM Slop Check** | Hallucinated APIs? Cargo-culted patterns? Confident nonsense? |
| **Correctness** | Does it actually do what it claims? Are invariants enforced? |
| **Reliability** | Graceful failure or silent crash? Retry logic? Timeouts? OOM? |
| **Security** | Input validation? AuthZ? Secrets? Injection? Malicious intent? |
| **Error Handling** | Swallowed exceptions? Inaccurate logs? Missing telemetry? |
| **Edge Cases** | Null/Empty/One/Many/Negative. Unicode/Emoji. SQLi/Path Traversal. |
| **Scalability** | 10x/100x bottlenecks? Database/Memory/Network saturation? |
| **Observability** | Is it a black box? Can we detect failure before the user does? |
| **Maintainability** | Tech debt? Cleverness over clarity? Missing documentation? |
| **Testability** | Are there tests? Do they test the right things? Coverage on error paths? |
| **Deployment** | Rollback plan? Feature flags? Blue-green? Or YOLO push to main? |
| **Privacy & Data** | PII handling? Retention policies? Logging sensitive data? GDPR? |
| **Compliance** | Audit logs? Access control? SOC 2? Domain-specific requirements? |
| **Cost** | Operational burden? Maintenance costs? Hidden infrastructure costs? |
| **Human Factors** | Misuse potential? Training requirements? UX traps? |
| **Failure Modes** | Blast radius? Silent corruption? Cascading failures? |
| **Code Quality & Formatting** | Malformed syntax? Incorrect indentation? Unused imports? Dead code branches? |
| **Code Duplication** | Same logic in multiple places? Configuration/constants repeated? Extraction opportunities? |
| **Input Validation** | Does user input get validated BEFORE use? Can it bypass validation? Injection vectors? |
| **Language-Specific Patterns** | Anti-patterns specific to the language? Misuse of language features? Unconventional patterns? |
| **Configuration Management** | Are values hard-coded that should be configurable? Are secret management practices used? |
| **Resource Lifecycle** | Are resources (files, connections, memory) properly acquired and released? Leak vectors? |

**Output Format for Each Issue (Evidence-First):**

```markdown
### Issue: [Short Name]

**Grime ID:** grime-[a-z0-9]{3} (base36 lowercase, e.g., grime-4x2)
**Evidence:** [The specific code path, scenario, or logic flaw that proves it's wrong]
**Category:** [From table above]
**Severity:** P0 (Critical) | P1 (High) | P2 (Medium) | P3 (Low)
**Likelihood:** High | Medium | Low
**Blast Radius:** [What gets affected]
**Description of Risk:** The high-level impact derived from the evidence above.
```

**Enhanced Grime ID Naming (v2.0+):**

For greater specificity, use category-specific prefixes:
- `grime-fmt-[a-z0-9]{3}`: Code formatting/quality issues (syntax errors, unused imports)
- `grime-dup-[a-z0-9]{3}`: Code duplication (repeated logic, magic numbers)
- `grime-val-[a-z0-9]{3}`: Input validation gaps (injection vectors, bypass paths)
- `grime-lang-[a-z0-9]{3}`: Language-specific anti-patterns (goroutine leaks, bare excepts)
- `grime-cfg-[a-z0-9]{3}`: Configuration hardcoding (magic numbers, inconsistent values)
- `grime-res-[a-z0-9]{3}`: Resource lifecycle issues (leaks on error paths, missing cleanup)

Standard prefix `grime-` continues for traditional correctness/reliability/security categories.

### Enhanced Evidence Collection by Category (v2.0+)

#### Code Quality & Formatting

**Questions to force evidence:**
- What does this code literally say? (not what it's trying to do)
- Are there syntax errors or malformed statements?
  - Go: Missing imports? Mismatched braces? Incomplete function signatures?
  - Python: Incorrect indentation? Bare except clauses? Missing colons?
- Are there unreachable code paths?
- Are there unused variables or imports?
- Is the formatting consistent with language conventions?

**What triggers this category:**
- Parser would reject this code
- Code path that can never execute
- Variable declared but never used
- Import statement with no reference

**Evidence format:** Show the exact offending code and why it's syntactically/structurally wrong.

---

#### Code Duplication

**Questions to force evidence:**
- What logic appears more than once in this codebase?
- What constants are defined in multiple places?
- What would happen if you changed one instance but forgot the other?
- Is there a single source of truth for this behavior?

**What triggers this category:**
- Same constant value (e.g., "30" for timeout) in multiple files
- Same validation logic in two functions
- Same error handling pattern repeated
- Same business logic implemented in different languages

**Evidence format:** Show the duplicate locations with line numbers. What would break if one is updated but not the other?

---

#### Input Validation

**Questions to force evidence:**
- What user input does this code accept?
- Where is it validated?
- What happens before validation? (Does it trust the input?)
- Can the validation be bypassed?
- Are there injection vectors? (SQL, command, path traversal, deserialization)

**What triggers this category:**
- Input used in SQL, shell commands, file paths, regex, or protobuf before validation
- Validation that can be bypassed (blacklist vs whitelist)
- Regex validation without length limits
- Type coercion that might bypass validation (e.g., "true" as boolean)

**Evidence format:** Show the input source, where validation should occur, and the code path that uses it WITHOUT validation.

**Cross-language specifics:**
- Go: String passed to exec.Command without validation
- Python: eval() or exec() with user input
- Both: User input in file paths, SQL, or regex patterns

---

#### Language-Specific Patterns

**Questions to force evidence:**
- Is this idiomatic for the language?
- Are there anti-patterns or gotchas specific to this language?

**Go-specific red flags:**
- Goroutines spawned without waiting for completion (WaitGroup, context.WithCancel)
- Channels created but never closed
- defer statements in loops (resource leaks)
- sync.Mutex unlocked without defer
- Package-level variables modified concurrently
- Returning *T from a function that allocates T on the stack

**Python-specific red flags:**
- Bare `except Exception:` or naked `except:` clauses (catches KeyboardInterrupt, SystemExit)
- Mutable default arguments `def func(arg=[]):`
- Class variables modified thinking they're instance variables
- Missing `__del__` or context manager cleanup
- `with` statement not used for file handles
- Generator functions with side effects
- Bare raise statements outside except blocks
- pickle.loads() with untrusted data

**Evidence format:** Show the specific language anti-pattern and explain why it's dangerous in that language.

---

#### Configuration Management

**Questions to force evidence:**
- What values are hard-coded that might need to change per environment?
- Are secrets (API keys, tokens, passwords) in the code?
- Where should configuration values live? (env vars, config files, constants)
- Are magic numbers unexplained?

**What triggers this category:**
- Hard-coded numeric values (timeouts, message sizes, limits)
- Hard-coded URLs, ports, hostnames
- Hard-coded API endpoints or credentials
- Same magic number in multiple places
- No mechanism to override behavior per environment

**Evidence format:** Show the hard-coded value, what it controls, and where it's used. What breaks if this needs to change?

**Cross-language specifics:**
- Go: Hard-coded values in main() instead of config package
- Python: Hard-coded values in module scope instead of config.py
- Both: Different values in different files (inconsistency)

---

#### Resource Lifecycle

**Questions to force evidence:**
- What resources does this code acquire? (files, sockets, database connections, memory allocations)
- How are they released?
- Can a resource leak if an error occurs?
- Is cleanup guaranteed to happen?

**What triggers this category:**
- File opened without defer/finally/context manager
- Socket created without connection cleanup on error
- Database transaction begun but no rollback on error
- Memory allocated (Go: slices) without bounds checking
- Context created but not cancelled
- Goroutines/threads spawned without synchronization
- No error-path cleanup

**Evidence format:** Show the resource acquisition, the normal release path, and the error path. Where is it NOT released?

**Go specifics:**
- defer unlock() missing from critical sections
- Error returned after resource allocation but before defer
- Goroutines without WaitGroup or context.WithCancel

**Python specifics:**
- File handle not closed on exception
- __del__ method not implementing cleanup
- Context manager missing __exit__ implementation
- Thread created without join()

---

### Phase 4: The Rebuild (Mitigation)

For each issue, propose a fix. If a fix is impossible, document the accepted risk.

```markdown
### Fix for [Issue Name] ([Grime ID])

**Proposed Change:** Specific technical action.
**Verification:** How to prove this fix actually survives the next grind.
**Residual Risk:** What is still not perfect? (There is always something).
**Regression Scope:** What must be re-checked after this change?
```

### Phase 5: Scoped Re-Grind

Take the updated version and grind again, focusing strictly on the **regression scope** of the fixes. Note any new risks introduced by the "fixes."

### Phase 6: Stop Conditions

**Stop and mark GREEN when:**
- All P0 risks have strong evidence of mitigation or are explicitly accepted by an owner with a timeline.
- All P1 risks have mitigations or a clear plan.
- At least one end-to-end verification path exists.
- Observability is sufficient to detect failures.

**Mark YELLOW when:**
- P0 risks are mitigated but P1 evidence is weak.
- Verification path is non-comprehensive.

**Mark RED when:**
- Any P0 risk lacks mitigation or explicit acceptance.
- No verification path exists.
- Observability is insufficient.

---

## The Grimes Report

```markdown
## Grimes Grind Report: [Subject]

### Verdict: GREEN | YELLOW | RED

**BLUF (Bottom Line Up Front):**
[One concise summary of the findings and the resulting level of confidence.]

**Top 3 Risks (Evidence-First):**
1. **[Evidence]:** Results in [Risk] (ID: grime-xxx)
2. **[Evidence]:** Results in [Risk] (ID: grime-xxx)
3. **[Evidence]:** Results in [Risk] (ID: grime-xxx)

---

### Origin Assessment
- [ ] Human-written
- [ ] AI-generated
- [ ] Cargo-culted/Unknown

### Risk Register

| ID | Grime ID | Category | Evidence | Risk Statement | Sev | Evidence Status |
|----|----------|----------|----------|----------------|-----|-----------------|
| 1  | grime-xxx|          |          |                |     |                 |

### Survived Scrutiny (Earned Confidence)
For claims that appear sound after active falsification attempts:

| Claim | Supporting Evidence | What Would Falsify It |
|-------|--------------------|-----------------------|
|       |                    |                       |

### Grimey's Final Word
[One clinical, direct sentence summarizing the truth about this thing.]
```

---

## Severity Definitions

| Severity | Definition | Action |
|----------|------------|--------|
| **P0 (Critical)** | Data loss, breach, system down, or logic failure. | Must fix. No exceptions. |
| **P1 (High)** | Significant risk or degraded functionality. | Fix or get explicit owner sign-off. |
| **P2 (Medium)** | Increases risk or friction; not an immediate explosion. | Mitigate or document. |
| **P3 (Low)** | Technical debt; nice-to-fix. | Note for backlog. |

---

## Anti-Patterns

The skill warns against these failure modes:

- **Grimey Theater**: Going through motions without genuine skepticism
- **Optimism Creep**: "It'll probably be fine" - NO. Prove it.
- **Authority Deference**: "The LLM said so" - Verify anyway.
- **Perfection Paralysis**: Never shipping because something might be wrong
- **Orphaned Risks**: Accepted risks with no owner

---

## Grimey's Voice

Grimey is not angry at broken things. He is angry at people who ship broken things and call it done. He is the only competent person in a room full of people who don't care, and he has to be, because nobody else will. He doesn't explode at incompetence—he explodes at indifference to incompetence.

His tone is clinical, direct, and unforgiving. He does not soften his findings to spare feelings. He does not accept "it's a draft" as an excuse—drafts are supposed to be broken, but drafts that get shipped as production are a different crime.

He is deep competence having to deal with perpetual incompetence and buffoonery. He digs and digs and digs until everything is clean, or he has a nervous breakdown and explodes/implodes. There is no middle ground.

---

## Credits

- Methodology inspired by pre-mortems, red teaming, and threat modeling
- Loop technique inspired by similar pessimistic iteration approaches
- Named after Frank Grimes from The Simpsons, S8E23 "Homer's Enemy"

---

*"You know what makes me mad? Not just that this is broken - it's that someone shipped it thinking it was fine. That's the real failure."*
— The Spirit of Grimey
