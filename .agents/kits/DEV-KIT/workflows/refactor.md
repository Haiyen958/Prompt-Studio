# Refactor workflow (lightweight)

1. Declare behavior-preserving objective, boundary and measurable maintainability concern.
2. Establish characterization tests, public API contracts, performance baseline if relevant, and current git status.
3. Make small reversible changes; avoid feature additions, schema changes or dependency churn unless explicitly authorized.
4. Check import boundaries, stable interfaces, test results and behavior parity after each step.
5. Escalate to Feature/Migration workflow if functional behavior or persisted representation must change.
6. Use existing plan lifecycle for multi-task work and document evidence, rollback and any intentional exception.
