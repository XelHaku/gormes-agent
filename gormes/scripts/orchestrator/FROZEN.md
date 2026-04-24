# Orchestrator commit freeze

As of the commit that introduced this file, this directory and the
companion entry script `gormes/scripts/gormes-auto-codexu-orchestrator.sh`
are considered **frozen** for architectural change.

## What's frozen

- `gormes/scripts/gormes-auto-codexu-orchestrator.sh`
- `gormes/scripts/orchestrator/lib/*.sh`
- `gormes/scripts/orchestrator/audit.sh`
- `gormes/scripts/orchestrator/claudeu`
- `gormes/scripts/orchestrator/systemd/*.in`

## Allowed exceptions

1. **Production-incident hotfix** — the orchestrator demonstrably broke in
   production; the fix must include reproduction steps and a test that would
   have caught it.
2. **User-requested feature** — documented in an open issue or explicit
   user request; must land as a scoped spec + plan like prior releases.

## Disallowed

- "While I'm in here" cleanup.
- Speculative architecture changes.
- Refactors without a behavior change.

## Review

If you are an AI agent, prompt the human user for confirmation before
changing any frozen file. Cite the exception category above.
