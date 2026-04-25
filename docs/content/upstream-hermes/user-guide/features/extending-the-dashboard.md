---
title: "Extending the Dashboard"
description: "Build themes and plugins for the Hermes web dashboard"
weight: 16
---

# Extending the Dashboard

Upstream Hermes consolidated the old dashboard plugin reference into a broader dashboard extension contract. The current upstream page is the canonical surface for:

- **Dashboard themes**: YAML files in `~/.hermes/dashboard-themes/` that repaint the browser dashboard palette, typography, layout, component chrome, and theme assets.
- **UI plugins**: plugin directories with `dashboard/manifest.json` and a JavaScript bundle that can register tabs, replace built-in pages, or inject components into named shell slots.
- **Backend plugins**: optional Python API route files mounted under `/api/plugins/<name>/` for dashboard plugin backends.

The important Gormes porting point is that this is no longer just "custom dashboard tabs." Hermes now treats the dashboard as an extension surface with runtime theme discovery, shell slots, page override points, plugin static assets, and optional backend routes.

## Runtime Shape Upstream

Themes and plugins are drop-in runtime assets. A user can add a theme YAML file or dashboard plugin directory without forking the Hermes dashboard source or rebuilding the frontend application. The dashboard discovers those assets, exposes them through dashboard APIs, and applies them in the running browser UI.

For the Go port, keep the runtime contract separate from the upstream implementation details:

- Theme discovery should remain an API/status surface, even if Gormes does not reuse the upstream React implementation.
- Plugin registration should preserve stable names, labels, icons, route paths, shell slots, hidden slot-only plugins, and built-in page overrides.
- Backend routes should remain isolated under plugin-specific API prefixes.
- Static asset serving should be scoped to plugin directories and should not become a general filesystem read endpoint.
- The dashboard should be able to report extension load errors without taking down unrelated dashboard pages.

## Porting Implication

Gormes should track dashboard extension parity in small layers:

1. Inventory and status API for themes and dashboard plugins.
2. Safe static asset serving for discovered plugin bundles.
3. UI shell slots and built-in page override metadata.
4. Optional backend route registration, only after the API server/plugin boundary is stable.

This mirrors the current upstream direction while leaving the Go dashboard implementation free to use native Gormes APIs and public status contracts.

## Compatibility Note

The former `Dashboard Plugins` document is now a compatibility pointer. New architecture work should reference this page and the upstream source file:

`../hermes-agent/website/docs/user-guide/features/extending-the-dashboard.md`
