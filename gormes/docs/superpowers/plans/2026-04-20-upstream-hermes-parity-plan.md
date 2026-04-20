# Feature Parity Dashboard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `port_status` frontmatter to upstream Hermes docs and create a Hugo dashboard showing feature parity between Python upstream and Go implementation.

**Architecture:** Hugo shortcodes read frontmatter from Site.Pages. A `frontmatter.yaml` sets defaults for all upstream-hermes docs. A shell script reads progress.json and updates frontmatter when subphases complete.

**Tech Stack:** Hugo, bash, Python (for JSON/YAML manipulation)

---

## File Inventory

### New Files to Create

| File | Purpose |
|------|---------|
| `docs/frontmatter.yaml` | Hugo default frontmatter for upstream-hermes docs |
| `docs/layouts/shortcodes/port-status-badge.html` | Badge shortcode: {{< port-status-badge "PORTED" >}} |
| `docs/layouts/shortcodes/parity-dashboard.html` | Full dashboard shortcode |
| `docs/content/building-gormes/parity.md` | Dashboard page at /building-gormes/parity/ |
| `scripts/update-port-status.sh` | Reads progress.json → updates upstream doc frontmatter |

### Existing Files to Modify

| File | Change |
|------|--------|
| `docs/data/progress.json` | Add `upstream_docs[]` and `go_equivalent` to each Phase 5 subphase |
| `gormes/Makefile` | Add call to `update-port-status.sh` after build |

### Upstream Docs to Modify (Day 1 population — all ~93 files)

Each gets `port_status: NOT_STARTED`, `port_phase: ""`, `go_equivalent: ""` set via frontmatter.yaml defaults. Individual overrides only if a doc maps to Phase 1-3.

---

## Tasks

### Task 1: Create Hugo frontmatter.yaml

**Files:**
- Create: `docs/frontmatter.yaml`

- [ ] **Step 1: Create docs/frontmatter.yaml**

```yaml
# Default frontmatter for Hugo content
#
# Files under upstream-hermes/ get these defaults unless overridden.
# Individual files can set different values in their frontmatter.

- path: "upstream-hermes/**/*"
  frontmatter:
  - id: ""
    port_status: NOT_STARTED
    port_phase: ""
    go_equivalent: ""
```

- [ ] **Step 2: Commit**

```bash
git add docs/frontmatter.yaml
git commit -m "feat(docs): add Hugo default frontmatter for upstream-hermes parity"
```

---

### Task 2: Create port-status-badge shortcode

**Files:**
- Create: `docs/layouts/shortcodes/port-status-badge.html`

- [ ] **Step 1: Create docs/layouts/shortcodes/port-status-badge.html**

```html
{{/*
  port-status-badge shortcode
  Usage: {{< port-status-badge "PORTED" >}}
        {{< port-status-badge "IN_PROGRESS" >}}
        {{< port-status-badge "NOT_STARTED" >}}

  Renders a colored status badge.
*/}}
{{ $status := .Get 0 }}
{{ $color := "" }}
{{ $label := "" }}

{{ if eq $status "PORTED" }}
  {{ $color = "#22c55e" }}
  {{ $label = "✅ PORTED" }}
{{ else if eq $status "IN_PROGRESS" }}
  {{ $color = "#eab308" }}
  {{ $label = "🔨 IN_PROGRESS" }}
{{ else }}
  {{ $color = "#6b7280" }}
  {{ $label = "⏳ NOT STARTED" }}
{{ end }}

<span style="
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 0.85em;
  font-weight: 600;
  background-color: {{ $color }}22;
  color: {{ $color }};
  border: 1px solid {{ $color }}66;
">{{ $label }}</span>
```

- [ ] **Step 2: Commit**

```bash
git add docs/layouts/shortcodes/port-status-badge.html
git commit -m "feat(docs): add port-status-badge Hugo shortcode"
```

---

### Task 3: Create parity-dashboard shortcode

**Files:**
- Create: `docs/layouts/shortcodes/parity-dashboard.html`

- [ ] **Step 1: Create docs/layouts/shortcodes/parity-dashboard.html**

```html
{{/*
  parity-dashboard shortcode
  Usage: {{< parity-dashboard >}}

  Reads all pages under upstream-hermes/ and displays a parity dashboard
  grouped by section, with status badges and links to Go equivalents.
*/}}

{{ $upstreamSection := "upstream-hermes" }}

{{/* Collect all upstream-hermes pages */}}
{{ $pages := where .Site.Pages "Section" $upstreamSection }}

{{/* Group by first sub-section */}}
{{ $features := where $pages "Type" "==" "page" | first 100 }}

{{/* Count statuses */}}
{{ $total := 0 }}
{{ $ported := 0 }}
{{ $inProgress := 0 }}
{{ $notStarted := 0 }}

{{ range $pages }}
  {{ $total = add $total 1 }}
  {{ if eq .Params.port_status "PORTED" }}
    {{ $ported = add $ported 1 }}
  {{ else if eq .Params.port_status "IN_PROGRESS" }}
    {{ $inProgress = add $inProgress 1 }}
  {{ else }}
    {{ $notStarted = add $notStarted 1 }}
  {{ end }}
{{ end }}

{{/* Overall parity bar */}}
<div style="margin: 2rem 0;">
  <h3>Overall Feature Parity</h3>
  <div style="
    display: flex;
    height: 24px;
    border-radius: 4px;
    overflow: hidden;
    background: #e5e7eb;
    margin-bottom: 0.5rem;
  ">
    {{ if $ported }}
    <div style="width: {{ mul (div (float $ported) (float $total)) 100 }}%; background: #22c55e;" title="Ported: {{ $ported }}"></div>
    {{ end }}
    {{ if $inProgress }}
    <div style="width: {{ mul (div (float $inProgress) (float $total)) 100 }}%; background: #eab308;" title="In Progress: {{ $inProgress }}"></div>
    {{ end }}
  </div>
  <p><strong>{{ $ported }}/{{ $total }}</strong> docs ported ({{ sub (mul (div (float $ported) (float $total)) 100) 0 | printf "%.0f" }}%)
   · {{ $inProgress }} in progress · {{ $notStarted }} not started</p>
</div>

{{/* Per-section breakdown */}}
<div style="margin: 2rem 0;">
  <h3>By Section</h3>

  {{ $sections := dict }}
  {{ range $pages }}
    {{ $section := .CurrentSection.Title }}
    {{ if not (index $sections $section) }}
      {{ $sections = merge $sections (dict $section (dict "total" 0 "ported" 0 "in_progress" 0 "pages" (slice))) }}
    {{ end }}
    {{ $sec := index $sections $section }}
    {{ $sec = merge $sec (dict "total" (add (index $sec "total") 1)) }}
    {{ if eq .Params.port_status "PORTED" }}
      {{ $sec = merge $sec (dict "ported" (add (index $sec "ported") 1)) }}
    {{ else if eq .Params.port_status "IN_PROGRESS" }}
      {{ $sec = merge $sec (dict "in_progress" (add (index $sec "in_progress") 1)) }}
    {{ end }}
    {{ $sec = merge $sec (dict "pages" (append (index $sec "pages") .)) }}
    {{ $sections = merge $sections (dict $section $sec) }}
  {{ end }}

  <table style="width: 100%; border-collapse: collapse;">
    <thead><tr style="text-align: left;">
      <th>Section</th><th>Total</th><th>Ported</th><th>In Progress</th><th>Not Started</th><th>Parity</th>
    </tr></thead>
    <tbody>
    {{ range $name, $sec := $sections }}
      <tr style="border-bottom: 1px solid #e5e7eb;">
        <td style="padding: 0.5rem 0;"><a href="{{ (index (index $sec "pages") 0).RelPermalink }}">{{ $name }}</a></td>
        <td style="padding: 0.5rem;">{{ index $sec "total" }}</td>
        <td style="padding: 0.5rem; color: #22c55e;">{{ index $sec "ported" }}</td>
        <td style="padding: 0.5rem; color: #eab308;">{{ index $sec "in_progress" }}</td>
        <td style="padding: 0.5rem; color: #6b7280;">{{ sub (index $sec "total") (add (index $sec "ported") (index $sec "in_progress")) }}</td>
        <td style="padding: 0.5rem;">
          {{ if gt (index $sec "total") 0 }}
          {{ sub (mul (div (float (index $sec "ported")) (float (index $sec "total"))) 100) 0 | printf "%.0f" }}%
          {{ else }}—{{ end }}
        </td>
      </tr>
    {{ end }}
    </tbody>
  </table>
</div>

{{/* Ported docs table */}}
{{ $portedPages := where $pages "Params.port_status" "PORTED" }}
{{ if $portedPages }}
<div style="margin: 2rem 0;">
  <h3>✅ Ported to Go ({{ len $portedPages }})</h3>
  <table style="width: 100%; border-collapse: collapse;">
    <thead><tr style="text-align: left;">
      <th>Doc</th><th>Phase</th><th>Go Equivalent</th>
    </tr></thead>
    <tbody>
    {{ range $portedPages }}
      <tr style="border-bottom: 1px solid #e5e7eb;">
        <td style="padding: 0.5rem 0;"><a href="{{ .RelPermalink }}">{{ .Title }}</a></td>
        <td style="padding: 0.5rem;">{{ .Params.port_phase }}</td>
        <td style="padding: 0.5rem;">
          {{ if .Params.go_equivalent }}
          <a href="{{ .Params.go_equivalent }}">{{ .Params.go_equivalent }}</a>
          {{ else }}—{{ end }}
        </td>
      </tr>
    {{ end }}
    </tbody>
  </table>
</div>
{{ end }}

{{/* In progress docs table */}}
{{ $inProgressPages := where $pages "Params.port_status" "IN_PROGRESS" }}
{{ if $inProgressPages }}
<div style="margin: 2rem 0;">
  <h3>🔨 In Progress ({{ len $inProgressPages }})</h3>
  <table style="width: 100%; border-collapse: collapse;">
    <thead><tr style="text-align: left;">
      <th>Doc</th><th>Phase</th>
    </tr></thead>
    <tbody>
    {{ range $inProgressPages }}
      <tr style="border-bottom: 1px solid #e5e7eb;">
        <td style="padding: 0.5rem 0;"><a href="{{ .RelPermalink }}">{{ .Title }}</a></td>
        <td style="padding: 0.5rem;">{{ .Params.port_phase }}</td>
      </tr>
    {{ end }}
    </tbody>
  </table>
</div>
{{ end }}
```

- [ ] **Step 2: Commit**

```bash
git add docs/layouts/shortcodes/parity-dashboard.html
git commit -m "feat(docs): add parity-dashboard Hugo shortcode"
```

---

### Task 4: Create parity.md dashboard page

**Files:**
- Create: `docs/content/building-gormes/parity.md`

- [ ] **Step 1: Create docs/content/building-gormes/parity.md**

```markdown
---
title: "Feature Parity Dashboard"
weight: 15
description: "Track which upstream Hermes features have been ported to Gormes (Go)"
---

# Feature Parity Dashboard

Track the migration of upstream Hermes (Python) capabilities to Gormes (Go-native).

**Source:** [progress.json](https://github.com/TrebuchetDynamics/gormes-agent/blob/main/gormes/docs/data/progress.json)
**Build:** Runs `scripts/update-port-status.sh` on `make build`

---

## All Features

{{< parity-dashboard >}}

---

## Phase 5 Subphases

For detailed Phase 5 progress, see the [Executive Roadmap](../architecture_plan/).
```

- [ ] **Step 2: Commit**

```bash
git add docs/content/building-gormes/parity.md
git commit -m "feat(docs): add feature parity dashboard page at /building-gormes/parity/"
```

---

### Task 5: Update progress.json with subphase-to-doc mapping

**Files:**
- Modify: `docs/data/progress.json`

- [ ] **Step 1: Add `upstream_docs[]` and `go_equivalent` to each Phase 5 subphase in progress.json**

Add this mapping based on the actual upstream doc structure. Example for subphase 5.C:

```json
"5.C": {
  "status": "planned",
  "name": "Browser Automation",
  "items": ["Chromedp", "Rod"],
  "upstream_docs": [
    "upstream-hermes/user-guide/features/browser.md"
  ],
  "go_equivalent": "/using-gormes/tools/"
}
```

Repeat for ALL Phase 5 subphases (5.A through 5.Q).

**Mapping reference — key upstream docs per subphase:**

| Subphase | Primary Upstream Docs |
|----------|----------------------|
| 5.A | `reference/tools-reference.md`, `reference/toolsets-reference.md`, `user-guide/features/tools.md` |
| 5.B | `user-guide/features/code-execution.md` |
| 5.C | `user-guide/features/browser.md` |
| 5.D | `user-guide/features/vision.md`, `user-guide/features/image-generation.md` |
| 5.E | `user-guide/features/tts.md`, `user-guide/features/voice-mode.md` |
| 5.F | `reference/skills-catalog.md`, `reference/optional-skills-catalog.md` |
| 5.G | `user-guide/features/mcp.md`, `reference/mcp-config-reference.md` |
| 5.H | `user-guide/features/acp.md` |
| 5.I | `user-guide/features/plugins.md` |
| 5.J | `user-guide/features/tools.md` (approval/security) |
| 5.K | `user-guide/features/code-execution.md` |
| 5.L | `user-guide/features/tools.md` (file ops) |
| 5.M | `user-guide/features/tools.md` (MoA) |
| 5.N | `cli.md`, `reference/cli-commands.md` |
| 5.O | `cli.md`, `reference/cli-commands.md`, `reference/profile-commands.md` |
| 5.P | `user-guide/docker.md` |
| 5.Q | `user-guide/tui.md` |

- [ ] **Step 2: Commit**

```bash
git add docs/data/progress.json
git commit -m "feat(docs): add upstream_docs mapping to progress.json Phase 5 subphases"
```

---

### Task 6: Create update-port-status.sh script

**Files:**
- Create: `scripts/update-port-status.sh`

- [ ] **Step 1: Create scripts/update-port-status.sh**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GORMES_DIR="$(dirname "$SCRIPT_DIR")"
PROGRESS_FILE="$GORMES_DIR/docs/data/progress.json"
UPSTREAM_DIR="$GORMES_DIR/docs/content/upstream-hermes"

if [[ ! -f "$PROGRESS_FILE" ]]; then
    echo "update-port-status: progress.json not found — skipping"
    exit 0
fi

if [[ ! -d "$UPSTREAM_DIR" ]]; then
    echo "update-port-status: upstream-hermes not found — skipping"
    exit 0
fi

python3 << PYEOF
import json
import os
import re

progress_file = "$PROGRESS_FILE"
upstream_dir = "$UPSTREAM_DIR"

with open(progress_file) as f:
    data = json.load(f)

phases = data.get("phases", {})
phase5 = phases.get("5", {}).get("subphases", {})

for subphase_id, subphase in phase5.items():
    status = subphase.get("status", "")
    upstream_docs = subphase.get("upstream_docs", [])
    go_equiv = subphase.get("go_equivalent", "")

    if status == "complete" and upstream_docs:
        for doc_rel_path in upstream_docs:
            doc_path = os.path.join(upstream_dir, doc_rel_path)
            if not os.path.exists(doc_path):
                print(f"warning: {doc_rel_path} not found, skipping")
                continue

            with open(doc_path) as f:
                content = f.read()

            # Update frontmatter
            new_lines = []
            in_frontmatter = False
            frontmatter_updated = False

            for line in content.split("\n"):
                if line.strip() == "---" and not frontmatter_updated:
                    if in_frontmatter:
                        frontmatter_updated = True
                        new_lines.append(f"port_status: PORTED")
                        new_lines.append(f"port_phase: \"{subphase_id}\"")
                        new_lines.append(f"go_equivalent: \"{go_equiv}\"")
                    else:
                        in_frontmatter = True
                        new_lines.append(line)
                        continue
                elif in_frontmatter and not frontmatter_updated:
                    if line.startswith("port_status:"):
                        new_lines.append(f"port_status: PORTED")
                        continue
                    elif line.startswith("port_phase:"):
                        new_lines.append(f"port_phase: \"{subphase_id}\"")
                        continue
                    elif line.startswith("go_equivalent:"):
                        new_lines.append(f"go_equivalent: \"{go_equiv}\"")
                        continue
                    elif line.startswith("---"):
                        frontmatter_updated = True
                        new_lines.append(f"port_status: PORTED")
                        new_lines.append(f"port_phase: \"{subphase_id}\"")
                        new_lines.append(f"go_equivalent: \"{go_equiv}\"")
                        new_lines.append(line)
                        continue

                new_lines.append(line)

            if frontmatter_updated:
                with open(doc_path, "w") as f:
                    f.write("\n".join(new_lines))
                print(f"Updated: {doc_rel_path} → PORTED (phase {subphase_id})")

print("update-port-status complete")
PYEOF
```

- [ ] **Step 2: Make executable**

```bash
chmod +x scripts/update-port-status.sh
```

- [ ] **Step 3: Commit**

```bash
git add scripts/update-port-status.sh
git commit -m "feat(docs): add update-port-status.sh for parity frontmatter updates"
```

---

### Task 7: Update gormes Makefile to call update-port-status.sh

**Files:**
- Modify: `gormes/Makefile`

- [ ] **Step 1: Add update-port-status call to Makefile build target**

In the Makefile, after the `record-progress` call, add:

```makefile
$(BINARY_PATH):
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY_PATH) ./cmd/gormes

define record-benchmark
	@echo "Recording benchmark..."
	@bash scripts/record-benchmark.sh
endef

define record-progress
	@echo "Updating progress..."
	@bash scripts/record-progress.sh
endef

define update-port-status
	@echo "Updating port status..."
	@bash scripts/update-port-status.sh
endef
```

Then add `$(call update-port-status)` after `$(call record-progress)` in the build target.

- [ ] **Step 2: Commit**

```bash
git add gormes/Makefile
git commit -m "feat(docs): add update-port-status.sh to gormes build pipeline"
```

---

### Task 8: Day 1 — Mark Phase 1-3 upstream docs as PORTED

**Files:**
- Modify: ~20 upstream docs that have Phase 1-3 Go equivalents

- [ ] **Step 1: Identify which upstream docs have Gormes equivalents**

Mark these upstream docs with `port_status: PORTED` in their frontmatter (manual override):

| Upstream Doc | Gormes Equivalent |
|-------------|-------------------|
| `user-guide/tui.md` | Phase 1 — Bubble Tea TUI |
| `reference/cli-commands.md` | Partial — gormes CLI |
| `user-guide/features/telegram-adapter.md` | Phase 2.B.1 |
| `user-guide/features/cron.md` | Phase 2.D |
| `user-guide/features/memory.md` | Phase 3 |
| `user-guide/features/skills.md` | Phase 2.G |
| `user-guide/features/delegation.md` | Phase 2.E |

- [ ] **Step 2: Commit**

```bash
git add docs/content/upstream-hermes/
git commit -m "feat(docs): mark Phase 1-3 upstream docs as PORTED"
```

---

### Task 9: Verify — Run make build and check dashboard

- [ ] **Step 1: Run make build in gormes/**

```bash
cd gormes && make clean && make build
```

Expected output:
```
Recording benchmark...
benchmarks.json updated: 16.2 MB
copied to docs/data/benchmarks.json
Updating progress...
progress.json updated: 2026-04-20
Updating README.md...
README.md updated with binary size: 16.2 MB
Updating port status...
update-port-status complete
```

- [ ] **Step 2: Build Hugo docs**

```bash
cd docs && hugo
```

- [ ] **Step 3: Verify parity.md renders correctly**

Check that the dashboard shortcode renders without errors and shows the correct counts.

- [ ] **Step 4: Commit all changes**

---

## Spec Coverage Check

| Spec Requirement | Task |
|-----------------|------|
| Frontmatter schema (port_status, port_phase, go_equivalent) | Task 1 |
| Hugo frontmatter.yaml defaults | Task 1 |
| Badge shortcode | Task 2 |
| Dashboard shortcode | Task 3 |
| Dashboard page at /building-gormes/parity/ | Task 4 |
| Subphase-to-doc mapping in progress.json | Task 5 |
| update-port-status.sh script | Task 6 |
| Build integration (Makefile) | Task 7 |
| Phase 1-3 marked PORTED | Task 8 |
| Verification | Task 9 |

## Placeholder Scan

All tasks have concrete file paths, code, and commands. No TODOs or TBDs.
