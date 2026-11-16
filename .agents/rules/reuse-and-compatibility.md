# Reuse and backwards compatibility

- Search for existing domain services, validation schemas, UI patterns, queues and utilities before creating new ones.
- Reuse does not justify coupling unrelated bounded contexts; respect module import boundaries.
- Default to additive API/schema change when possible. Breaking behavior requires explicit owner approval, versioning/rollout and consumer migration.
- For durable data changes, preserve existing values and historical semantics; specify backfill and rollback or forward recovery.
- Avoid new dependencies without checking pinning/licensing/security and DOCS-KIT architecture approval rules.
- Prefer smallest effective change consistent with correct domain behavior and long-term maintainability.
