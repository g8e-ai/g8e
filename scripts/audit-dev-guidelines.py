#!/usr/bin/env python3
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Rank repository files by statically detectable developer-guideline violations.

This is a triage tool, not a complete semantic linter. Every finding points to a
rule that can be checked from source text; review the reported lines before
changing code.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Callable, Iterable

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_GUIDELINES = ROOT / "docs" / "devs" / "devs.md"

SKIP_DIRS = {
    ".git",
    ".g8e",
    ".github",
    ".local.dev",
    ".pytest_cache",
    ".venv",
    "__pycache__",
    "bin",
    "build",
    "coverage",
    "dist",
    "node_modules",
    "target",
    "third_party",
    "vendor",
}

SOURCE_EXTENSIONS = {
    ".c",
    ".cc",
    ".cpp",
    ".go",
    ".h",
    ".hpp",
    ".js",
    ".jsx",
    ".mjs",
    ".py",
    ".sh",
    ".ts",
    ".tsx",
}
SPECIAL_FILES = {"Dockerfile", "Makefile"}


@dataclass(frozen=True)
class Finding:
    path: str
    line: int
    rule: str
    description: str
    text: str


@dataclass(frozen=True)
class Rule:
    rule: str
    description: str
    pattern: re.Pattern[str]
    extensions: frozenset[str] | None = None
    matcher: Callable[[re.Match[str], str], bool] | None = None


RULES = (
    Rule(
        "errors-new",
        "Use a centralized typed error instead of errors.New outside internal/constants.",
        re.compile(r"\berrors\.New\s*\("),
        frozenset({".go"}),
        lambda _match, path: not path.startswith("internal/constants/"),
    ),
    Rule(
        "panic-production",
        "Production paths must return errors instead of panicking.",
        re.compile(r"\bpanic\s*\("),
        frozenset({".go"}),
    ),
    Rule(
        "untyped-map",
        "Known contracts must use typed models rather than untyped map containers.",
        re.compile(r"\bmap\s*\[\s*string\s*\]\s*(?:interface\s*\{\}|any)"),
        frozenset({".go"}),
    ),
    Rule(
        "direct-runtime-io",
        "Runtime .g8e I/O belongs behind RuntimeFileService, not direct os file calls.",
        re.compile(r"\bos\.(?:ReadFile|WriteFile|Mkdir|MkdirAll|Remove|RemoveAll|Stat)\s*\("),
        frozenset({".go"}),
        lambda match, path: "/internal/services/fs/" not in path.replace("\\", "/") and any(
            marker in match.string for marker in (".g8e", "RuntimeDir", "PKIDir", "CredentialsDir", "VaultDir")
        ),
    ),
    Rule(
        "hardcoded-runtime-path",
        "Runtime paths must use constants rather than inline .g8e path fragments.",
        re.compile(r"(?:['\"]|`)[^'\"`]*(?:\\/|/)\.g8e(?:[/\\]|['\"`])"),
    ),
    Rule(
        "filepath-literal",
        "Path construction must use path constants instead of filepath.Join string literals.",
        re.compile(r"filepath\.Join\s*\(\s*['\"]"),
        frozenset({".go"}),
    ),
    Rule(
        "ensure-or-get-or-create",
        "Do not hide writes behind ensure* or getOrCreate* helpers.",
        re.compile(r"\b(?:func\s+)?(?:ensure[A-Z]\w*|getOrCreate[A-Z]\w*)\s*\("),
        frozenset({".go"}),
    ),
    Rule(
        "direct-go-test",
        "Run platform tests through ./g8e test or the owning Make target.",
        re.compile(r"(?:^|[\s;&|])go\s+test(?:\s|$)"),
        frozenset({".sh", ".md", ".mk"}),
    ),
    Rule(
        "os-chdir",
        "Tests must not use os.Chdir to align runtime state.",
        re.compile(r"\bos\.Chdir\s*\("),
        frozenset({".go"}),
    ),
    Rule(
        "generated-output-edit",
        "Generated protobuf and OpenAPI output should be changed through its owner.",
        re.compile(r"(?i)(?:generated|swagger|openapi).*(?:edit|write|modify)|(?:edit|write|modify).*(?:generated|swagger|openapi)"),
        frozenset({".md"}),
    ),
)


def should_scan(path: Path, root: Path, guidelines: Path) -> bool:
    if not path.is_file() or path.resolve() == guidelines:
        return False
    if set(path.relative_to(root).parts) & SKIP_DIRS:
        return False
    return path.name in SPECIAL_FILES or path.suffix in SOURCE_EXTENSIONS or path.suffix == ".md"


def repository_relative_path(path: Path) -> str:
    try:
        return path.resolve().relative_to(ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def iter_files(root: Path, guidelines: Path) -> Iterable[Path]:
    for path in sorted(root.rglob("*")):
        if should_scan(path, root, guidelines):
            yield path


def find_file(path: Path, root: Path, include_generated: bool) -> list[Finding]:
    relative = path.relative_to(root).as_posix()
    repository_relative = repository_relative_path(path)
    if not include_generated and ("generated" in repository_relative.lower() or "swagger" in repository_relative.lower()):
        return []

    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError as exc:
        print(f"warning: cannot read {relative}: {exc}", file=sys.stderr)
        return []

    findings: list[Finding] = []
    for line_number, line in enumerate(lines, start=1):
        for rule in RULES:
            if rule.extensions is not None and path.suffix not in rule.extensions and path.name not in rule.extensions:
                continue
            match = rule.pattern.search(line)
            if match is None or (rule.matcher is not None and not rule.matcher(match, repository_relative)):
                continue
            findings.append(Finding(relative, line_number, rule.rule, rule.description, line.strip()))
    return findings


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT, help="Repository root to scan")
    parser.add_argument("--guidelines", type=Path, default=DEFAULT_GUIDELINES, help="Guidelines file to verify")
    parser.add_argument("--min-violations", type=int, default=1, help="Only show files with at least this many findings")
    parser.add_argument("--limit", type=int, default=25, help="Maximum ranked files to print; 0 means unlimited")
    parser.add_argument("--include-generated", action="store_true", help="Include generated and Swagger/OpenAPI paths")
    parser.add_argument("--json", action="store_true", dest="as_json", help="Emit machine-readable JSON")
    return parser.parse_args()


def display_guidelines_path(guidelines: Path, root: Path) -> str:
    try:
        return guidelines.relative_to(root).as_posix()
    except ValueError:
        try:
            return guidelines.relative_to(ROOT).as_posix()
        except ValueError:
            return str(guidelines)


def main() -> int:
    args = parse_args()
    root = args.root.resolve()
    guidelines = args.guidelines.resolve()
    if not root.is_dir():
        print(f"error: scan root is not a directory: {root}", file=sys.stderr)
        return 2
    if not guidelines.is_file():
        print(f"error: guidelines file not found: {guidelines}", file=sys.stderr)
        return 2

    findings = [finding for path in iter_files(root, guidelines) for finding in find_file(path, root, args.include_generated)]
    by_file: dict[str, list[Finding]] = {}
    for finding in findings:
        by_file.setdefault(finding.path, []).append(finding)
    ranked = sorted(by_file.items(), key=lambda item: (-len(item[1]), item[0]))
    ranked = [item for item in ranked if len(item[1]) >= args.min_violations]
    if args.limit:
        ranked = ranked[: args.limit]

    guidelines_path = display_guidelines_path(guidelines, root)
    if args.as_json:
        print(json.dumps({"guidelines": guidelines_path, "files": [{"path": path, "violations": [asdict(finding) for finding in file_findings]} for path, file_findings in ranked]}, indent=2))
        return 0

    print(f"Developer-guideline triage: {len(findings)} findings in {len(by_file)} files")
    print(f"Guidelines: {guidelines_path}")
    if not ranked:
        print("No matching findings.")
        return 0
    for path, file_findings in ranked:
        print(f"\n{len(file_findings):3d}  {path}")
        for finding in file_findings:
            print(f"      {finding.line:4d}  {finding.rule}: {finding.text}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
