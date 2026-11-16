# Bugfix workflow (lightweight)

1. Reproduce the **actual** fault with inputs, environment, version and observable evidence. If nonreproducible, mark uncertain instead of inventing root cause.
2. Compare expected behavior with domain rules and tests. If expectation is unclear, resolve a decision before changing behavior.
3. Identify smallest root-cause change, blast radius, security implications and rollback.
4. Create/attach a regression test that fails for the original defect where feasible; then implement minimal fix.
5. Use the repository-native DOCS-KIT `docs/plans/` lifecycle for nontrivial fixes; for small fixes obey project requirements for task tracking and authorization.
6. Run targeted/full verification appropriate to risk, record evidence and update documentation when the **specification** changes.
7. Do not mislabel a desired new behavior as a bugfix just to skip feature review.
