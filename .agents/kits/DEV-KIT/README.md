# DEV-KIT v1.0

Instructional kit for building **features on an existing PLAN-KIT / DOCS-KIT project**, with lightweight entry paths for bugfixes, refactors, migrations and optimizations.

## Quick start

1. Copy the `DEV-KIT` directory into a convenient location accessible to the coding agent (or attach `DEV-KIT.md` and relevant workflow files).
2. Open the existing repository in your IDE with Codex or Claude Code.
3. Paste `prompts/feature-start.md`, replacing `<FEATURE REQUEST>` with the requested feature.
4. Review the agent's analysis, answer only undecided questions and approve the project-native plan.
5. Run `prompts/feature-review.md` with another agent when possible.
6. Authorize implementation after a documented READY assessment.

## Compatibility

Designed around DOCS-KIT §7's `docs/plans/` and `scripts/plan.mjs`; does **not** replace it. Existing project conventions and scripts must be inspected. No IDE plugin, slash command or runnable automation is installed by these markdown files.

## Kit outputs

- Primary guide: `DEV-KIT.md`
- Workflows: `workflows/`
- Templates: `templates/`
- Engineering rules: `rules/`
- Agent prompts: `prompts/`
