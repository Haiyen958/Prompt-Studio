---
name: optimization
description: >-
  DEV-KIT Optimization workflow for evidence-driven performance, latency, memory, throughput, or cost improvements. Use this skill when the user asks to optimize slow queries/endpoints, reduce resource or token usage, improve latency/throughput, or tune caching and concurrency.
---

# DEV-KIT Optimization Workflow

Canonical references: [workflows/optimization.md](../../kits/DEV-KIT/workflows/optimization.md) · [DEV-KIT.md](../../kits/DEV-KIT/DEV-KIT.md)

## Procedure

1. **Define Problem & Baseline Metrics (Read-Only First):**
   - State the concrete symptom, target workload, reproducible benchmark/measurement method, current baseline numbers, and target threshold.
   - Never optimize speculatively without a baseline or reproducible bottleneck measurement.
2. **Profile Bottlenecks:**
   - Profile actual execution paths (queries, I/O, serialization, LLM/network calls, allocations, lock contention) before proposing caching, indexing, batching, algorithmic, or concurrency changes.
3. **Evaluate Trade-Offs & Approval:**
   - Assess correctness risks, cache invalidation/staleness, memory/cost trade-offs, tenant isolation, and operational side effects.
   - Obtain owner approval when introducing new infrastructure, caching semantics, schema indexes (`migration` check), or trade-offs visible to users; track multi-step work in `docs/plans/` when present.
4. **Smallest Measurable Intervention:**
   - Implement the minimal change (behind a config toggle or safe rollout path when appropriate) while preserving all domain invariants and existing contracts.
5. **Before/After Verification:**
   - Run equivalent workloads before and after the change, execute full correctness and regression tests, and attach real metrics + command exit codes to the plan report. Never claim performance gains without measured evidence.
