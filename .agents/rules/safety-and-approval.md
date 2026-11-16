# Safety, permission and evidence rules

- Read-only inspection by default. Planning/review never implies permission to change code, protected docs, production resources or credentials.
- Follow `AGENTS.md`, repository hooks and explicit owner approvals. No hidden `--no-verify`, skipped tests, fake evidence, or self-issued approval.
- Do not read or log live secrets; ask for redacted contract examples instead.
- Never overwrite dirty working-tree changes without permission; no unrequested destructive git actions.
- Do not commit, push, deploy or mutate production simply because the plan is READY.
- Distinguish *proposed*, *approved*, *implemented* and *verified* at all times.
- Report actual commands and exit codes. If runner/CI evidence is required, agent-written reports are insufficient.
