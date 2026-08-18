#!/usr/bin/env python3
"""Check the curated documentation tree and its volatile public surfaces."""

from __future__ import annotations

import json
import re
import sys
from collections import deque
from pathlib import Path
from urllib.parse import urlparse


ROOT = Path(__file__).resolve().parents[1]
PUBLIC = ROOT / "documentation"


def local_targets(markdown: str):
    for match in re.finditer(r"\[[^\]]+\]\(([^)]+)\)", markdown):
        target = match.group(1).strip().strip("<>")
        if not target or target.startswith("#"):
            continue
        parsed = urlparse(target)
        if parsed.scheme or parsed.netloc:
            continue
        yield target.split("#", 1)[0]
    for match in re.finditer(r"^\s*\[[^\]]+\]:\s*(\S+)", markdown, flags=re.MULTILINE):
        target = match.group(1).strip().strip("<>")
        parsed = urlparse(target)
        if parsed.scheme or parsed.netloc:
            continue
        yield target.split("#", 1)[0]


def resolve(source: Path, target: str) -> Path:
    candidate = (source.parent / target).resolve()
    if target.endswith("/") and (candidate / "README.md").is_file():
        return candidate / "README.md"
    return candidate


def main() -> int:
    errors: list[str] = []
    pages = sorted(PUBLIC.rglob("*.md"))
    if not pages:
        errors.append("documentation tree is empty")

    for section in ("overview", "getting-started", "architecture", "design", "contracts", "guides", "operations", "reference", "governance", "adr"):
        if not (PUBLIC / section / "README.md").is_file():
            errors.append(f"missing section landing page: documentation/{section}/README.md")

    links: dict[Path, list[Path]] = {}
    for page in pages:
        text = page.read_text(encoding="utf-8")
        if len([line for line in text.splitlines() if line.strip()]) < 8:
            errors.append(f"stub-like public page: {page.relative_to(ROOT)}")
        if page.name not in {"BAR.md", "PLAN.md"} and "## Next reads" not in text:
            errors.append(f"page contract omits Next reads: {page.relative_to(ROOT)}")
        mermaid_open = False
        for line_number, line in enumerate(text.splitlines(), start=1):
            if line == "```mermaid":
                if mermaid_open:
                    errors.append(f"nested Mermaid fence in {page.relative_to(ROOT)}:{line_number}")
                mermaid_open = True
            elif mermaid_open and line == "```":
                mermaid_open = False
        if mermaid_open:
            errors.append(f"unterminated Mermaid fence in {page.relative_to(ROOT)}")
        for forbidden in ("/Users/", "/mnt/", "/home/", "/private/var/"):
            if forbidden in text:
                errors.append(f"machine-local path {forbidden!r} in {page.relative_to(ROOT)}")
        for pattern in (r"-----BEGIN [A-Z ]*PRIVATE KEY-----", r"\b(?:sk|ghp|glpat)-[A-Za-z0-9_-]{16,}\b", r"\bAKIA[0-9A-Z]{16}\b"):
            if re.search(pattern, text):
                errors.append(f"credential-like content in {page.relative_to(ROOT)}")
        resolved_links: list[Path] = []
        for target in local_targets(text):
            resolved = resolve(page, target)
            if resolved.is_file():
                resolved_links.append(resolved)
            elif resolved.is_dir():
                if (resolved / "README.md").is_file():
                    resolved_links.append(resolved / "README.md")
            else:
                errors.append(f"broken link in {page.relative_to(ROOT)}: {target}")
        links[page] = resolved_links

    index = PUBLIC / "README.md"
    if index not in pages:
        errors.append("documentation/README.md is missing")
    else:
        reachable: set[Path] = set()
        queue = deque([index])
        while queue:
            page = queue.popleft()
            if page in reachable:
                continue
            reachable.add(page)
            queue.extend(links.get(page, []))
        for page in pages:
            if page not in reachable:
                errors.append(f"orphan public page: {page.relative_to(ROOT)}")

    for relative in ("README.md", "CONTRIBUTING.md", "SECURITY.md", "SUPPORT.md", "CODE_OF_CONDUCT.md", "CHANGELOG.md", "docs/README.md"):
        if not (ROOT / relative).is_file():
            errors.append(f"missing open-source entrypoint: {relative}")

    root_text = (ROOT / "README.md").read_text(encoding="utf-8")
    if "documentation/README.md" not in root_text:
        errors.append("root README does not link to documentation/README.md")
    for policy in ("CONTRIBUTING.md", "SECURITY.md", "SUPPORT.md", "CODE_OF_CONDUCT.md"):
        if policy not in root_text:
            errors.append(f"root README does not link to {policy}")

    cli_page = (PUBLIC / "reference" / "cli.md").read_text(encoding="utf-8")
    cli_source = (ROOT / "cmd" / "agentic-stream" / "main.go").read_text(encoding="utf-8")
    for command in ("version", "validate", "run", "run-live", "serve", "config effective"):
        if command not in cli_page:
            errors.append(f"CLI reference omits command: {command}")
    for source_marker in ("newVersionCommand", "newValidateCommand", "newRunCommand", "newRunLiveCommand", "newServeCommand", "newConfigEffectiveCommand"):
        if source_marker not in cli_source:
            errors.append(f"CLI source marker missing: {source_marker}")

    http_page = (PUBLIC / "reference" / "http-api.md").read_text(encoding="utf-8")
    for route in ("/health/live", "/health/ready", "/v1/events", "/metrics", "/control/drain", "/control/kill"):
        if route not in http_page:
            errors.append(f"HTTP reference omits route: {route}")

    config_page = (PUBLIC / "reference" / "configuration.md").read_text(encoding="utf-8")
    for variable in ("AGENTIC_STREAM_MODEL_API_KEY", "AGENTIC_STREAM_SUBSCRIBER_TOKEN", "AGENTIC_STREAM_CONTROL_TOKEN", "AGENTIC_STREAM_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_ENDPOINT"):
        if variable not in config_page:
            errors.append(f"configuration reference omits environment variable: {variable}")

    invariant_page = (PUBLIC / "architecture" / "invariants.md").read_text(encoding="utf-8")
    if len(re.findall(r"^\d+\. ", invariant_page, flags=re.MULTILINE)) != 10:
        errors.append("public invariant page does not contain exactly ten numbered invariants")

    status_path = PUBLIC / "governance" / "release-status.json"
    try:
        status = json.loads(status_path.read_text(encoding="utf-8"))
        for key in ("channel", "version", "stable_release", "implemented", "partial", "deferred", "release_blockers"):
            if key not in status:
                errors.append(f"release status omits key: {key}")
    except (OSError, json.JSONDecodeError) as exc:
        errors.append(f"invalid release-status.json: {exc}")

    if errors:
        for error in errors:
            print(f"docs-check: {error}", file=sys.stderr)
        print(f"docs-check: {len(errors)} failure(s)", file=sys.stderr)
        return 1
    print(f"docs-check: {len(pages)} public Markdown pages and volatile surfaces verified")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
