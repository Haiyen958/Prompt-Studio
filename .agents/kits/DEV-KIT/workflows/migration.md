# Migration workflow (lightweight)

1. Identify current/target schema, runtime or provider plus data volume and compatibility expectations.
2. Require an owner-approved change contract: read/write behavior during migration, old clients, consistency and downtime tolerance.
3. Prepare expand → backfill → verify → switch → contract, or justify a different strategy.
4. Specify idempotence, retry/checkpointing, verification counts/checksums, observability and failure recovery.
5. Evaluate rollback feasibility explicitly (a destructive migration may require restore/forward-fix instead).
6. Test on representative anonymized data; seek authorization before touching production.
7. Integrate with `docs/plans/`, update canonical DB/operations docs and retain evidence.
