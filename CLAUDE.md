# Gormes Repository Instructions

This repository is in a Go port phase. Unless the user explicitly changes this policy, follow these rules:

- Do not write or modify any Python code.
- Do not edit legacy Python paths such as `run_agent.py`, `cli.py`, `agent/`, `gateway/`, `hermes_cli/`, `tools/`, `tui_gateway/`, `cron/`, `acp_adapter/`, `tests/`, or any other `.py` files.
- Only edit the root `README.md` and files under `gormes/`.
- Treat the existing Python codebase as upstream reference only.
- If a requested change requires touching Python, stop and ask the user whether they want to override this rule.
