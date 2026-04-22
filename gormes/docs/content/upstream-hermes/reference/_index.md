---
title: "Reference"
description: "Complete reference for CLI commands, environment variables, and configuration."
weight: 4
---

# Reference

This section is the operator-facing surface ledger for Hermes. For Gormes, it answers a different question than the developer guide: not "how is Hermes implemented?" but "what commands, configuration, tool surfaces, and catalogs must a Go port still expose?"

## Porting Goal

Read this section as the compatibility inventory for the external surface area:

- **Surface** tells you which operator or integrator-facing interface exists upstream.
- **Method used** tells you the main upstream mechanism or source of truth for that interface.
- **Porting implication** tells you what kind of compatibility or replacement Gormes needs.

## Full Reference Inventory for Go Porting

| Surface | Method used upstream | Porting implication for Gormes |
|---|---|---|
| [CLI Commands](./cli-commands/) | Central CLI entrypoint in `hermes_cli/main.py` plus subcommand dispatch helpers | Rebuild command parity from one command tree, not ad hoc binaries |
| [Slash Commands](./slash-commands/) | Shared command registry resolved by interactive CLI and messaging gateway | Keep one source of truth for interactive commands across surfaces |
| `hermes honcho` command family | Provider-specific CLI owned by the Honcho memory plugin (`status`, `setup`, `strategy`, `peer`, `mode`, `tokens`, `identity`, `sync`, `enable`, `disable`, `migrate`) | Honcho parity is not complete unless this operator surface is accounted for too |
| [Profile Commands](./profile-commands/) | Profile-aware `HERMES_HOME` overrides and profile management commands | Preserve multi-profile isolation and switching semantics |
| [Environment Variables](./environment-variables/) | `.env`-driven provider keys, platform tokens, and behavior flags | Port env var behavior carefully; many integrations depend on exact names |
| Honcho env and config surface | `HONCHO_API_KEY`, `HONCHO_BASE_URL`, plus profile-local and global `honcho.json` config resolution | Preserve config resolution order and operator expectations if Honcho remains supported |
| [MCP Config Reference](./mcp-config-reference/) | MCP server declarations over stdio or HTTP, with transport and tool filtering config | Keep config compatibility if Gormes remains MCP-capable |
| [Tools Reference](./tools-reference/) | Built-in tool schemas and operator-facing descriptions generated from the tool runtime | Regenerate from Go tool metadata, but keep stable tool contracts where possible |
| [Toolsets Reference](./toolsets-reference/) | Named bundles of tools and platform presets | Preserve grouping semantics so existing workflows still make sense |
| [Skills Catalog](./skills-catalog/) | Bundled skill inventory shipped with Hermes | Useful as a parity checklist for first-party skills |
| [Optional Skills Catalog](./optional-skills-catalog/) | Official opt-in skill inventory installed separately from the base runtime, including the `honcho` optional skill | Keep the split between bundled and optional skill surfaces, and explicitly track Honcho's operator guidance |
| [FAQ](./faq/) | Human-facing operational explanations and compatibility notes | Lower runtime priority, but useful for preserving migration guidance and expectations |

## What Matters Most for Go Parity

Not every reference page is equally important during the port.

### Highest priority

- [CLI Commands](./cli-commands/)
- [Slash Commands](./slash-commands/)
- [Environment Variables](./environment-variables/)
- [Tools Reference](./tools-reference/)
- [Toolsets Reference](./toolsets-reference/)
- [MCP Config Reference](./mcp-config-reference/)

These pages define the surfaces users and external tooling directly touch.

**Honcho belongs in this highest-priority set whenever Honcho support is in scope.** It spans commands, env vars, config files, tools, and memory-provider behavior.

### Secondary priority

- [Profile Commands](./profile-commands/)
- [Skills Catalog](./skills-catalog/)
- [Optional Skills Catalog](./optional-skills-catalog/)

These matter once the core runtime is stable enough to expose compatibility-level UX.

### Lowest priority

- [FAQ](./faq/)

Important for documentation quality, but it should follow working runtime parity rather than lead it.
