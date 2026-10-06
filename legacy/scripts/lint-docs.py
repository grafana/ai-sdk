#!/usr/bin/env python3
"""Structural linter for the project's Markdown documentation.

Complements markdownlint (which covers prose/style) by enforcing the rules from
the docs-structure spec and CONTRIBUTING contract that markdownlint cannot see:

  1. Relative links resolve to a real file.
  2. Intra-repo anchor links (#section) resolve to a real heading.
  3. Every non-index page under docs/ ends with a nav footer linking the index.
  4. Every page under docs/ is linked from the docs index (docs/README.md).
  5. docs/ pages do not reproduce option/signature reference tables
     (the "godoc owns the API reference" drift boundary).

No third-party dependencies. Run from the repo root:

    python3 scripts/lint-docs.py
"""

from __future__ import annotations

import os
import re
import sys

# Files outside docs/ that are still part of the doc surface and should have
# their links/anchors checked.
ROOT_DOCS = ["README.md", "CONTRIBUTING.md", "AGENTS.md"]

DOCS_DIR = "docs"
DOCS_INDEX = os.path.join(DOCS_DIR, "README.md")
EXAMPLES_DIR = "examples"

LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.*)")
# Heuristic for a reference table that belongs in godoc, not docs/.
OPTION_TABLE_RE = re.compile(r"^\|\s*(Option|Field|Method|Signature)\b", re.IGNORECASE)


def iter_lines_skipping_fences(text: str):
    """Yield (lineno, line) for lines outside fenced code blocks."""
    in_fence = False
    for i, line in enumerate(text.splitlines(), 1):
        if line.lstrip().startswith("```"):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        yield i, line


def slugify(heading: str) -> str:
    """Approximate GitHub's heading -> anchor slug algorithm."""
    s = heading.strip().lower()
    s = s.replace("`", "")
    s = re.sub(r"[^\w\s-]", "", s)
    return re.sub(r"\s+", "-", s)


def collect_anchors(path: str) -> set[str]:
    anchors: set[str] = set()
    with open(path, encoding="utf-8") as fh:
        for _, line in iter_lines_skipping_fences(fh.read()):
            m = HEADING_RE.match(line)
            if m:
                anchors.add(slugify(m.group(2)))
    return anchors


def gather_files() -> list[str]:
    files = [f for f in ROOT_DOCS if os.path.exists(f)]
    for base in (DOCS_DIR, EXAMPLES_DIR):
        for dirpath, _, names in os.walk(base):
            for n in names:
                if n.endswith(".md"):
                    files.append(os.path.join(dirpath, n))
    return sorted(files)


def check_links(files: list[str], errors: list[str]) -> None:
    anchors = {os.path.normpath(f): collect_anchors(f) for f in files}
    for f in files:
        base = os.path.dirname(f)
        with open(f, encoding="utf-8") as fh:
            for i, line in iter_lines_skipping_fences(fh.read()):
                for m in LINK_RE.finditer(line):
                    url = m.group(1).strip()
                    if url.startswith(("http://", "https://", "mailto:")):
                        continue
                    path, _, frag = url.partition("#")
                    target = os.path.normpath(os.path.join(base, path)) if path else os.path.normpath(f)
                    if path and not os.path.exists(target):
                        errors.append(f"{f}:{i}: dead link -> {url}")
                        continue
                    if frag and target in anchors and frag not in anchors[target]:
                        errors.append(f"{f}:{i}: dead anchor -> #{frag} (in {target})")


def check_footers_and_tables(errors: list[str]) -> None:
    for dirpath, _, names in os.walk(DOCS_DIR):
        for n in names:
            if not n.endswith(".md"):
                continue
            path = os.path.join(dirpath, n)
            text = open(path, encoding="utf-8").read()

            # Contract: no reference tables in docs/.
            for i, line in iter_lines_skipping_fences(text):
                if OPTION_TABLE_RE.match(line.strip()):
                    errors.append(
                        f"{path}:{i}: reference table belongs in godoc, not docs/ "
                        f"(link to pkg.go.dev instead)"
                    )

            # Index files are exempt from the nav-footer rule.
            if os.path.normpath(path) == os.path.normpath(DOCS_INDEX):
                continue
            if "Docs index" not in text:
                errors.append(f"{path}: missing nav footer (no 'Docs index' link)")


def check_index_coverage(errors: list[str]) -> None:
    if not os.path.exists(DOCS_INDEX):
        errors.append(f"{DOCS_INDEX}: docs index is missing")
        return
    index_text = open(DOCS_INDEX, encoding="utf-8").read()
    for dirpath, _, names in os.walk(DOCS_DIR):
        for n in names:
            if not n.endswith(".md"):
                continue
            path = os.path.join(dirpath, n)
            if os.path.normpath(path) == os.path.normpath(DOCS_INDEX):
                continue
            rel = os.path.relpath(path, DOCS_DIR)
            if rel not in index_text:
                errors.append(f"{path}: not linked from the docs index ({DOCS_INDEX})")


def main() -> int:
    if not os.path.isdir(DOCS_DIR):
        print(f"error: run from the repo root (no {DOCS_DIR}/ here)", file=sys.stderr)
        return 2

    files = gather_files()
    errors: list[str] = []

    check_links(files, errors)
    check_footers_and_tables(errors)
    check_index_coverage(errors)

    if errors:
        print(f"docs lint: {len(errors)} issue(s) found\n")
        for e in sorted(errors):
            print(f"  {e}")
        return 1

    print(f"docs lint: OK ({len(files)} files checked)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
