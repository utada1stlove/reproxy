# AGENTS.md

## Mission
- Build `reproxy` as a lightweight edge proxy manager for entry machines.
- Keep the default implementation MVP-first: simple inputs, deterministic config generation, easy operations.

## Working Rules
- Always read `codex-action/00-master-plan.md` before making changes.
- Always update `codex-action/progress.md` after each completed phase.
- Create missing directories and files when needed.
- Record architecture decisions in docs before or alongside code changes.
- Prefer small, reviewable changes and minimal dependencies.
- Treat file-based state and generated Nginx config as the MVP baseline.
- Keep HTTPS automation implemented as a thin hook around external ACME tooling.
- Keep documentation in sync with implementation and startup flow.
- Always summarize created and modified files at the end.
