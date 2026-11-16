# Optimization workflow (lightweight)

1. State measured problem, workload, baseline, reproducible benchmark and success threshold.
2. Profile bottlenecks before selecting cache, indexes, algorithms, scaling or concurrency changes.
3. Evaluate correctness, resource/cost tradeoffs, invalidation and operational side effects.
4. Implement smallest measurable intervention behind a safe rollout when applicable.
5. Compare before/after under equivalent loads, keep correctness and regression tests, document regressions.
6. Attach results to DOCS-KIT native change plan; do not assert improvement without actual metrics.
