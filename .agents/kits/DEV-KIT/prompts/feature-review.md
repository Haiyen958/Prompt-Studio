# Independent feature review prompt

Review the proposed feature change in this repository as an independent auditor. Read `DEV-KIT.md`, `workflows/feature.md` and `templates/readiness-review.md`, plus actual canonical docs, plan, code and tests. Do not begin implementation or modify source.

Audit requirement → design → contract → task → acceptance-test traceability. Test business permissions/invariants, tenant isolation, concurrency, retries, migration/backwards compatibility, observability and rollbacks where relevant. Cite specific files/symbols. Separate confirmed defects, risks, missing decisions and unverified assumptions. Classify P0–P3 and recommend minimal changes. Distinguish existing project approvals from missing ones.

Produce an independent readiness report and a `READY / CONDITIONALLY READY / NOT READY` verdict tied to plan and document revision. Only report READY if blockers and required decisions are resolved and owner approval is evidenced. Do not invent successful tests or automatic enforcement.
