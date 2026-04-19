#!/usr/bin/env python3
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[2]
MAIN = ROOT / "hermes_cli" / "main.py"
DOCS = ROOT / "website" / "docs"
OUT = ROOT / "gormes" / "testdata"

cmd_pattern = re.compile(r'\badd_parser\("([^"]+)"')

commands = []
for line in MAIN.read_text(encoding="utf-8").splitlines():
    m = cmd_pattern.search(line)
    if m:
        commands.append(m.group(1))

paths = []
for path in DOCS.rglob("*.md"):
    rel = path.relative_to(DOCS).as_posix()
    paths.append(rel.removesuffix(".md"))

OUT.mkdir(parents=True, exist_ok=True)
(OUT / "cli_surface.json").write_text(json.dumps({"commands": sorted(set(commands))}, indent=2), encoding="utf-8")
(OUT / "docs_surface.json").write_text(json.dumps({"paths": sorted(set(paths))}, indent=2), encoding="utf-8")
