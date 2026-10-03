#!/usr/bin/env python3
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Repository-specific developer-guideline audit (standard library only).

Go checks use a comment/string-aware lexer; Python checks use ASTs. Clone and
unused-helper findings are review candidates, not semantic proof of dead code.
No code is executed, deleted, or rewritten by this audit.
"""

from __future__ import annotations

import argparse
import ast
import copy
import io
import json
import os
import re
import sys
import tokenize
from collections import Counter, defaultdict
from collections.abc import Iterable
from dataclasses import asdict, dataclass, field
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SKIP_DIRS = {
    ".git",
    ".g8e",
    ".github",
    ".claude",
    ".codex",
    ".local.dev",
    ".pytest_cache",
    ".ruff_cache",
    ".mypy_cache",
    ".venv",
    "venv",
    "__pycache__",
    "bin",
    "build",
    "coverage",
    "dist",
    "node_modules",
    "target",
    "third_party",
    "vendor",
    "site",
}
SOURCE_EXTENSIONS = {".go", ".py", ".js", ".jsx", ".mjs", ".ts", ".tsx", ".sh", ".mk"}
SPECIAL_FILES = {"Dockerfile", "Makefile"}
CONFIDENCE_WEIGHT = {"high": 5, "medium": 2, "low": 1}


@dataclass(frozen=True)
class Rule:
    rule: str
    description: str
    guideline: str
    confidence: str = "medium"


RULES = {
    rule.rule: rule
    for rule in (
        Rule(
            "errors-new",
            "Use a centralized error outside internal/constants/errors.go.",
            "INV-ERR-02",
            "high",
        ),
        Rule(
            "panic-production",
            "Return errors for recoverable production failures; review this panic.",
            "INV-CODE-06",
        ),
        Rule(
            "untyped-map",
            "Review whether this map replaces a known typed contract.",
            "INV-TYPE-01",
        ),
        Rule(
            "direct-runtime-io",
            "Route runtime state I/O through RuntimeFileService.",
            "INV-FS-01",
        ),
        Rule(
            "hardcoded-runtime-path",
            "Use the owned runtime path constants.",
            "INV-FS-02",
        ),
        Rule(
            "filepath-literal",
            "Use path constants for reusable path fragments.",
            "INV-FS-02",
        ),
        Rule(
            "ensure-or-get-or-create",
            "Review this helper for hidden creation or combined reads/writes.",
            "INV-CODE-12",
            "low",
        ),
        Rule(
            "direct-go-test",
            "Run platform suites through ./g8e test or the owning Make target.",
            "INV-TEST-01",
        ),
        Rule(
            "os-chdir",
            "Review source-discovery exception, explanation, and cleanup for Chdir.",
            "INV-TEST-11",
            "low",
        ),
        Rule(
            "parallel-integration-test",
            "Integration and E2E tests must not use t.Parallel.",
            "INV-TEST-02",
            "high",
        ),
        Rule(
            "error-string-comparison",
            "Use errors.Is/As or typed status instead of rendered errors.",
            "INV-ERR-06",
        ),
        Rule(
            "raw-env-key",
            "Use the registered environment key constant.",
            "INV-ENV-04",
            "high",
        ),
        Rule(
            "python-model-hub",
            "Import Pydantic model helpers through app.models.base.",
            "docs/ensemble/devs.md#coding-standards",
            "high",
        ),
        Rule(
            "python-untyped-error",
            "Use the owned app.errors failure type for application errors.",
            "docs/ensemble/devs.md#coding-standards",
        ),
        Rule(
            "local-execution-bypass",
            "Review local execution outside the owned Operator tool path.",
            "docs/ensemble/devs.md#scope-and-architecture",
        ),
        Rule(
            "swallowed-exception",
            "Review an exception discarded without propagation or evidence.",
            "INV-CODE-04",
        ),
        Rule(
            "duplicate-function",
            "Consolidate substantial identical function bodies with the existing owner.",
            "INV-CODE-11",
            "high",
        ),
        Rule(
            "renamed-function-clone",
            "Review structurally identical bodies with renamed bindings.",
            "INV-CODE-11",
        ),
        Rule(
            "unused-private-helper",
            "No reference outside this private helper was found in scanned source; verify dynamic callers and build variants.",
            "INV-CODE-11",
            "low",
        ),
        Rule(
            "protocol-constant-copy",
            "Import the shared protocol constant instead of redeclaring its public identifier.",
            "INV-TYPE-06",
        ),
        Rule(
            "protocol-go-constant-drift",
            "A literal Go constant disagrees with its explicit _go_const protocol registry entry.",
            "INV-TYPE-06",
            "high",
        ),
        Rule(
            "protocol-registry-drift",
            "Bundled Python registry differs from its canonical protocol registry; regenerate through the owner.",
            "INV-TYPE-06",
            "high",
        ),
        Rule(
            "codemap-missing-owner",
            "The code map points to a missing owned path; repair the map or restore the owner.",
            "INV-CODE-14",
            "high",
        ),
        Rule(
            "scan-error",
            "A file could not be parsed or read; audit coverage is incomplete.",
            "audit coverage",
            "high",
        ),
    )
}


@dataclass(frozen=True)
class Location:
    path: str
    line: int
    symbol: str = ""


@dataclass(frozen=True)
class Finding:
    path: str
    line: int
    rule: str
    description: str
    text: str
    guideline: str
    confidence: str
    related: tuple[Location, ...] = ()


@dataclass(frozen=True)
class Token:
    value: str
    start: int
    end: int
    line: int
    kind: str


@dataclass
class Source:
    path: str
    text: str
    suffix: str
    test: bool
    tokens: list[Token] = field(default_factory=list)
    clean: str = ""
    mask: str = ""
    tree: ast.Module | None = None

    def finding(
        self,
        rule: str,
        line: int,
        related: tuple[Location, ...] = (),
        evidence: str | None = None,
    ) -> Finding:
        spec = RULES[rule]
        lines = self.text.splitlines()
        text = (
            evidence
            if evidence is not None
            else (lines[line - 1].strip() if 0 < line <= len(lines) else "")
        )
        return Finding(
            self.path,
            line,
            rule,
            spec.description,
            text[:300],
            spec.guideline,
            spec.confidence,
            related,
        )


@dataclass(frozen=True)
class Function:
    source: Source
    name: str
    line: int
    exact: str
    normalized: str
    identifiers: Counter[str]
    private: bool
    substantial: bool

    @property
    def location(self) -> Location:
        return Location(self.source.path, self.line, self.name)


def blank(text: str) -> str:
    return re.sub(r"[^\n]", " ", text)


# Strings and comments are consumed atomically, so braces in them never affect
# function extraction. Go raw strings and escaped quotes are included.
GO_TOKEN = re.compile(
    r"(?P<comment>//[^\n]*|/\*[\s\S]*?\*/)|"
    r'(?P<string>`[^`]*`|"(?:\\.|[^"\\])*"|\'(?:\\.|[^\'\\])*\')|'
    r"(?P<name>[A-Za-z_]\w*)|(?P<number>\d[\w.]*)|"
    r"(?P<space>\s+)|(?P<operator>:=|==|!=|<=|>=|&&|\|\||<-|\+\+|--|\S)"
)
GO_KEYWORDS = {
    "break",
    "case",
    "chan",
    "const",
    "continue",
    "default",
    "defer",
    "else",
    "fallthrough",
    "for",
    "func",
    "go",
    "goto",
    "if",
    "import",
    "interface",
    "map",
    "package",
    "range",
    "return",
    "select",
    "struct",
    "switch",
    "type",
    "var",
    "true",
    "false",
    "nil",
}


def lex(source: Source) -> None:
    clean, mask = list(source.text), list(source.text)
    if source.suffix == ".py":
        offsets = [0]
        for line in source.text.splitlines(keepends=True):
            offsets.append(offsets[-1] + len(line))
        for token in tokenize.generate_tokens(io.StringIO(source.text).readline):
            start = offsets[token.start[0] - 1] + token.start[1]
            end = offsets[token.end[0] - 1] + token.end[1]
            if token.type in (tokenize.COMMENT, tokenize.STRING):
                mask[start:end] = blank(source.text[start:end])
            if token.type == tokenize.COMMENT:
                clean[start:end] = blank(source.text[start:end])
            if token.type in (
                tokenize.NAME,
                tokenize.STRING,
                tokenize.OP,
                tokenize.NUMBER,
            ):
                source.tokens.append(
                    Token(
                        token.string,
                        start,
                        end,
                        token.start[0],
                        "name" if token.type == tokenize.NAME else "other",
                    )
                )
        source.tree = ast.parse(source.text, filename=source.path)
    else:
        line = 1
        for match in GO_TOKEN.finditer(source.text):
            kind, value = match.lastgroup, match.group()
            if kind in ("comment", "string"):
                mask[match.start() : match.end()] = blank(value)
            if kind == "comment":
                clean[match.start() : match.end()] = blank(value)
            if kind not in ("comment", "space"):
                source.tokens.append(
                    Token(value, match.start(), match.end(), line, kind or "")
                )
            line += value.count("\n")
    source.clean, source.mask = "".join(clean), "".join(mask)


def is_test(path: str) -> bool:
    parts = Path(path).parts
    name = Path(path).name
    return (
        bool(set(parts) & {"test", "tests", "testdata", "test-fixtures", "fakes"})
        or name.endswith("_test.go")
        or name.startswith("test_")
        or bool(re.search(r"\.(?:test|spec)\.", name))
        or bool(
            set(parts[:-1])
            & {"cmdtest", "storagetest", "pubsubtest", "governancetest", "keystoretest"}
        )
    )


def generated(path: str, text: str) -> bool:
    return (
        bool(set(Path(path).parts) & {"generated", "gen", "_data"})
        or path.startswith(
            (
                "internal/services/gateway/docs/",
                "internal/services/gateway/console/static/",
                "internal/services/gateway/explorer/static/",
            )
        )
        or bool(re.search(r"(?:\.pb\.go|_pb2(?:_grpc)?\.(?:py|pyi)|_gen\.go)$", path))
        or bool(
            re.search(
                r"(?im)(?:code generated.*do not edit|automatically generated|auto-generated|@generated)",
                text[:2500],
            )
        )
    )


def iter_files(root: Path) -> Iterable[Path]:
    # Prune before descent: rglob would traverse vendor and node_modules anyway.
    for directory, dirs, files in os.walk(root, followlinks=False):
        dirs[:] = sorted(
            d
            for d in dirs
            if d not in SKIP_DIRS
            and not d.startswith(".g8e")
            and not d.endswith(".egg-info")
            and not Path(directory, d).is_symlink()
        )
        for name in sorted(files):
            path = Path(directory, name)
            if not path.is_symlink() and (
                path.suffix in SOURCE_EXTENSIONS or name in SPECIAL_FILES
            ):
                yield path


def regex_findings(source: Source) -> list[Finding]:
    findings = []
    if source.suffix != ".go":
        # The owning Make test runner is allowed to invoke the compiler's runner.
        if source.suffix in {".sh", ".mk"} and Path(source.path).name != "Makefile":
            for n, line in enumerate(source.text.splitlines(), 1):
                if not line.lstrip().startswith("#") and re.search(
                    r"(?:^|[\s;&|])go\s+test\b", line
                ):
                    findings.append(source.finding("direct-go-test", n))
        return findings
    patterns = {}
    if source.test:
        patterns["os-chdir"] = r"\bos\.Chdir\s*\("
        if set(Path(source.path).parts) & {"integration", "e2e"} or re.search(
            r"(?m)^//go:build.*\b(?:integration|e2e)\b", source.text
        ):
            patterns["parallel-integration-test"] = r"\b\w+\.Parallel\s*\("
    else:
        patterns.update(
            {
                "raw-env-key": r'\bos\.(?:Getenv|LookupEnv)\s*\(\s*["`]',
                "panic-production": r"\bpanic\s*\(",
                "untyped-map": r"\bmap\s*\[\s*string\s*\]\s*(?:interface\s*\{\s*\}|any\b)",
                "ensure-or-get-or-create": r"\bfunc\s+(?:\([^)]*\)\s*)?(?:ensure[A-Z]\w*|getOrCreate[A-Z]\w*)\s*\(",
                "error-string-comparison": r"\b\w+\.Error\s*\(\s*\)\s*(?:==|!=)",
            }
        )
        if source.path != "internal/constants/errors.go":
            patterns["errors-new"] = r"\berrors\.New\s*\("
        if not source.path.startswith(("internal/constants/", "internal/services/fs/")):
            patterns["filepath-literal"] = r'\bfilepath\.Join\s*\([^)]*?"'
            patterns["direct-runtime-io"] = (
                r"\bos\.(?:ReadFile|WriteFile|Mkdir|MkdirAll|Remove|RemoveAll|Stat|Open|OpenFile)"
                r"\s*\([^;)]*(?:\.g8e|RuntimeDir|PKIDir|CredentialsDir|VaultDir)"
            )
            for token in source.tokens:
                if token.kind == "string" and re.search(
                    r"(?:^|[/\\])\.g8e(?:[/\\]|$)", token.value.strip('"`')
                ):
                    findings.append(
                        source.finding("hardcoded-runtime-path", token.line)
                    )
    aliases = {}
    for index, token in enumerate(source.tokens):
        if token.value != "import":
            continue
        cursor = index + 1
        block = cursor < len(source.tokens) and source.tokens[cursor].value == "("
        if block:
            cursor += 1
        while cursor < len(source.tokens) and source.tokens[cursor].value != ")":
            item = source.tokens[cursor]
            alias = None
            if item.kind != "string":
                alias = item.value
                cursor += 1
                if cursor >= len(source.tokens):
                    break
                item = source.tokens[cursor]
            package = item.value.strip('"')
            if package in {"errors", "os", "path/filepath"} and alias not in {"_", "."}:
                aliases[package.rsplit("/", 1)[-1]] = (
                    alias or package.rsplit("/", 1)[-1]
                )
            cursor += 1
            if not block:
                break
    for rule, pattern in patterns.items():
        for package, alias in aliases.items():
            pattern = pattern.replace(package + r"\.", re.escape(alias) + r"\.")
        for match in re.finditer(pattern, source.clean):
            if not source.mask[match.start() : match.start() + 1].isspace():
                findings.append(
                    source.finding(rule, source.text.count("\n", 0, match.start()) + 1)
                )
    return findings


def dotted(node: ast.AST) -> str:
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return dotted(node.value) + "." + node.attr
    return ""


def resolved_name(node: ast.AST, aliases: dict[str, str]) -> str:
    name = dotted(node)
    first, _, rest = name.partition(".")
    return aliases.get(first, first) + ("." + rest if rest else "")


def python_findings(source: Source) -> list[Finding]:
    if source.tree is None or source.test:
        return []
    findings = []
    aliases = {}
    for node in ast.walk(source.tree):
        if isinstance(node, ast.Import):
            for item in node.names:
                aliases[item.asname or item.name.split(".")[0]] = (
                    item.name if item.asname else item.name.split(".")[0]
                )
                if (
                    item.name == "pydantic"
                    and source.path.startswith("ensemble/app/")
                    and source.path != "ensemble/app/models/base.py"
                ):
                    findings.append(source.finding("python-model-hub", node.lineno))
        elif isinstance(node, ast.ImportFrom):
            for item in node.names:
                aliases[item.asname or item.name] = f"{node.module}.{item.name}"
            if (
                source.path.startswith("ensemble/app/")
                and source.path != "ensemble/app/models/base.py"
                and node.module == "pydantic"
            ):
                findings.append(source.finding("python-model-hub", node.lineno))
    for node in ast.walk(source.tree):
        if isinstance(node, ast.Call):
            name = resolved_name(node.func, aliases)
            key = (
                node.args[0]
                if node.args
                else next(
                    (
                        keyword.value
                        for keyword in node.keywords
                        if keyword.arg == "key"
                    ),
                    None,
                )
            )
            if (
                name in {"os.getenv", "os.environ.get", "os.environ.__getitem__"}
                and isinstance(key, ast.Constant)
                and isinstance(key.value, str)
            ):
                findings.append(source.finding("raw-env-key", node.lineno))
            if source.path.startswith("ensemble/app/") and (
                name.startswith("subprocess.")
                or name
                in {
                    "os.system",
                    "os.popen",
                    "asyncio.create_subprocess_exec",
                    "asyncio.create_subprocess_shell",
                }
            ):
                findings.append(source.finding("local-execution-bypass", node.lineno))
        elif (
            isinstance(node, ast.Subscript)
            and resolved_name(node.value, aliases) == "os.environ"
        ):
            if (
                isinstance(node.ctx, ast.Load)
                and isinstance(node.slice, ast.Constant)
                and isinstance(node.slice.value, str)
            ):
                findings.append(source.finding("raw-env-key", node.lineno))
        elif isinstance(node, ast.Raise) and source.path.startswith("ensemble/app/"):
            exception = node.exc.func if isinstance(node.exc, ast.Call) else node.exc
            if exception is not None and dotted(exception) in {
                "Exception",
                "RuntimeError",
                "ValueError",
            }:
                findings.append(source.finding("python-untyped-error", node.lineno))
        elif isinstance(node, ast.ExceptHandler):
            if all(isinstance(statement, ast.Pass) for statement in node.body):
                findings.append(source.finding("swallowed-exception", node.lineno))
    return findings


class NormalizeBindings(ast.NodeTransformer):
    """Rename only Python local bindings, preserving globals, attributes and literals."""

    def __init__(self, node: ast.FunctionDef | ast.AsyncFunctionDef):
        names = [
            arg.arg
            for arg in (*node.args.posonlyargs, *node.args.args, *node.args.kwonlyargs)
        ]
        names += [arg.arg for arg in (node.args.vararg, node.args.kwarg) if arg]
        names += [
            item.id
            for item in ast.walk(node)
            if isinstance(item, ast.Name) and isinstance(item.ctx, ast.Store)
        ]
        self.names = {
            name: f"local{index}" for index, name in enumerate(dict.fromkeys(names))
        }

    def visit_Name(self, node: ast.Name) -> ast.Name:
        node.id = self.names.get(node.id, node.id)
        return node

    def visit_arg(self, node: ast.arg) -> ast.arg:
        node.arg = self.names.get(node.arg, node.arg)
        return node


def python_functions(source: Source, min_tokens: int) -> list[Function]:
    if source.tree is None or source.test:
        return []
    functions = []
    top_level = {id(node) for node in source.tree.body}
    for node in ast.walk(source.tree):
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        # Nested definitions are intentionally excluded: capture semantics matter.
        if any(
            isinstance(item, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef))
            for statement in node.body
            for item in ast.walk(statement)
        ):
            continue
        body = [
            statement
            for statement in node.body
            if not (
                isinstance(statement, ast.Expr)
                and isinstance(statement.value, ast.Constant)
                and isinstance(statement.value.value, str)
            )
        ]
        tokens = [
            token
            for token in source.tokens
            if node.lineno <= token.line <= (node.end_lineno or node.lineno)
        ]
        normalized = NormalizeBindings(node)
        exact = ast.dump(
            ast.Module(body=body, type_ignores=[]), include_attributes=False
        )
        canonical = normalized.visit(
            copy.deepcopy(ast.Module(body=body, type_ignores=[]))
        )
        functions.append(
            Function(
                source,
                node.name,
                node.lineno,
                exact,
                ast.dump(canonical, include_attributes=False),
                Counter(token.value for token in tokens if token.kind == "name"),
                id(node) in top_level
                and node.name.startswith("_")
                and not node.name.startswith("__")
                and not node.decorator_list,
                len(tokens) >= min_tokens and len(body) >= 3,
            )
        )
    return functions


def go_functions(source: Source, min_tokens: int) -> list[Function]:
    if source.suffix != ".go" or source.test:
        return []
    tokens, functions = source.tokens, []
    # Pair delimiters once; named declarations can then skip return structs and
    # nested function types without confusing their braces with the function body.
    pairs, stack = {}, []
    for index, token in enumerate(tokens):
        if token.value in {"(", "[", "{"}:
            stack.append(index)
        elif token.value in {")", "]", "}"} and stack:
            opener = stack.pop()
            if tokens[opener].value == {")": "(", "]": "[", "}": "{"}[token.value]:
                pairs[opener] = index
    for index, token in enumerate(tokens):
        if token.value != "func":
            continue
        cursor, method = index + 1, False
        if cursor < len(tokens) and tokens[cursor].value == "(":
            method = True
            cursor = pairs.get(cursor, len(tokens)) + 1
        if cursor >= len(tokens) or tokens[cursor].kind != "name":
            continue
        name = tokens[cursor].value
        cursor += 1
        if cursor < len(tokens) and tokens[cursor].value == "[":
            cursor = pairs.get(cursor, len(tokens)) + 1
        if cursor >= len(tokens) or tokens[cursor].value != "(":
            continue
        cursor = pairs.get(cursor, len(tokens)) + 1
        while cursor < len(tokens) and tokens[cursor].value != "{":
            if tokens[cursor].value in {"(", "["}:
                cursor = pairs.get(cursor, len(tokens)) + 1
            elif (
                tokens[cursor].value in {"struct", "interface"}
                and cursor + 1 < len(tokens)
                and tokens[cursor + 1].value == "{"
            ):
                cursor = pairs.get(cursor + 1, len(tokens)) + 1
            else:
                cursor += 1
        end = pairs.get(cursor)
        if end is None:
            continue
        body = tokens[cursor + 1 : end]
        values, normalized, bindings = [], [], {}
        for pos, item in enumerate(body):
            values.append(item.value)
            # Preserve selectors, types/call targets, literals, and keywords.
            # Bare identifiers are only a lexical similarity signal in Go.
            if (
                item.kind == "name"
                and item.value not in GO_KEYWORDS
                and not item.value[0].isupper()
                and (pos == 0 or body[pos - 1].value != ".")
                and (pos + 1 == len(body) or body[pos + 1].value != "(")
            ):
                normalized.append(bindings.setdefault(item.value, f"id{len(bindings)}"))
            else:
                normalized.append(item.value)
        identifiers = Counter(
            item.value for item in tokens[index : end + 1] if item.kind == "name"
        )
        functions.append(
            Function(
                source,
                name,
                token.line,
                json.dumps(values),
                json.dumps(normalized),
                identifiers,
                not method and name[0].islower() and name not in {"main", "init"},
                len(body) >= min_tokens and len({item.line for item in body}) >= 6,
            )
        )
    return functions


def structural_findings(
    functions: list[Function], references: dict[str, Counter[str]]
) -> list[Finding]:
    findings = []
    for function in functions:
        if (
            function.private
            and references[function.source.suffix][function.name]
            <= function.identifiers[function.name]
        ):
            findings.append(
                function.source.finding(
                    "unused-private-helper", function.line, evidence=function.name
                )
            )
    # Linear grouping rather than a quadratic all-pairs similarity search.
    groups: dict[tuple[str, str], list[Function]] = defaultdict(list)
    for function in functions:
        if function.substantial:
            groups[(function.source.suffix, function.normalized)].append(function)
    for group in groups.values():
        if len(group) < 2:
            continue
        for function in group:
            others = [other for other in group if other is not function]
            exact = [other for other in others if other.exact == function.exact]
            rule = "duplicate-function" if exact else "renamed-function-clone"
            findings.append(
                function.source.finding(
                    rule,
                    function.line,
                    tuple(other.location for other in (exact or others)),
                    evidence=function.name,
                )
            )
    return findings


def registry_entries(
    value: object, path: tuple[str, ...] = ()
) -> Iterable[tuple[tuple[str, ...], dict]]:
    if isinstance(value, dict):
        if isinstance(value.get("value"), str):
            yield path, value
        for key, child in value.items():
            yield from registry_entries(child, (*path, key))


def go_constant_index(
    root: Path,
) -> tuple[dict[str, list[tuple[str, Location]]], list[Finding]]:
    values: dict[str, list[tuple[str, Location]]] = defaultdict(list)
    findings = []
    # Compare explicit literal declarations only. Aliases, iota, computed values,
    # and struct-backed registries remain the owning conformance tests' job.
    declaration = re.compile(
        r"(?m)^[ \t]*(?:const[ \t]+)?([A-Za-z_]\w*)[ \t]+"
        r'(?:[A-Za-z_][\w.]*[ \t]+)?=[ \t]*("(?:\\.|[^"\\])*"|`[^`]*`)[ \t]*$'
    )
    for path in sorted((root / "internal/constants").glob("*.go")):
        if path.name.endswith("_test.go") or path.is_symlink():
            continue
        source = Source(path.relative_to(root).as_posix(), "", ".go", False)
        try:
            source.text = path.read_text(encoding="utf-8")
            lex(source)
            for match in declaration.finditer(source.clean):
                if source.mask[match.start(1)].isspace():
                    continue
                literal = match[2]
                value = (
                    literal[1:-1]
                    if literal.startswith("`")
                    else ast.literal_eval(literal)
                )
                location = Location(
                    source.path,
                    source.text.count("\n", 0, match.start(1)) + 1,
                    match[1],
                )
                values[match[1]].append((value, location))
        except (OSError, UnicodeError, SyntaxError, ValueError) as exc:
            findings.append(source.finding("scan-error", 1, evidence=str(exc)))
    return values, findings


def alignment_findings(root: Path) -> tuple[list[Finding], dict[str, list[Location]]]:
    go_values, findings = go_constant_index(root)
    constants = defaultdict(list)
    registry_dir = root / "protocol/constants"
    for path in sorted(registry_dir.glob("*.json")):
        relative = path.relative_to(root).as_posix()
        source = Source(relative, "", ".json", False)
        try:
            data = json.loads(path.read_text())
            for keys, entry in registry_entries(data):
                # Only stable public identifiers with explicit Python ownership;
                # generic values like 'enabled' and 'error' would be too noisy.
                value = entry["value"]
                for go_value, location in go_values.get(entry.get("_go_const"), []):
                    if go_value != value:
                        findings.append(
                            source.finding(
                                "protocol-go-constant-drift",
                                1,
                                (location,),
                                evidence=f"{'.'.join(keys)}: registry={value!r}; Go={go_value!r}",
                            )
                        )
                if (
                    entry.get("_python_const")
                    and len(value) >= 8
                    and ("_" in value or ":" in value or "/" in value)
                ):
                    constants[value].append(Location(relative, 1, ".".join(keys)))
            bundle = root / "protocol/python/g8e/_data" / path.name
            if bundle.parent.is_dir() and (
                not bundle.is_file() or json.loads(bundle.read_text()) != data
            ):
                findings.append(
                    source.finding(
                        "protocol-registry-drift",
                        1,
                        (Location(bundle.relative_to(root).as_posix(), 1),),
                        evidence=path.name,
                    )
                )
        except (OSError, ValueError, UnicodeError) as exc:
            findings.append(source.finding("scan-error", 1, evidence=str(exc)))
    codemap = root / "docs/devs/codemap.md"
    if codemap.is_file():
        source = Source("docs/devs/codemap.md", "", ".md", False)
        try:
            source.text = codemap.read_text(encoding="utf-8")
        except (OSError, UnicodeError) as exc:
            findings.append(source.finding("scan-error", 1, evidence=str(exc)))
        for line, text in enumerate(source.text.splitlines(), 1):
            # Only literal, repo-relative owner paths. Globs and CLI prose are excluded.
            for match in re.finditer(
                r"`((?:internal|ensemble|protocol|console|evaluation-explorer|scripts|cmd|test)/[^`\s]*)`",
                text,
            ):
                owner = match[1]
                if (
                    not any(char in owner for char in "*<>…")
                    and not (root / owner).exists()
                ):
                    findings.append(
                        source.finding("codemap-missing-owner", line, evidence=owner)
                    )
    return findings, constants


def protocol_copy_findings(
    source: Source, constants: dict[str, list[Location]]
) -> list[Finding]:
    findings = []
    if (
        not source.path.startswith("ensemble/app/")
        or source.tree is None
        or source.test
    ):
        return []
    for node in ast.walk(source.tree):
        if isinstance(node, (ast.Assign, ast.AnnAssign)) and isinstance(
            node.value, ast.Constant
        ):
            value = node.value.value
            targets = node.targets if isinstance(node, ast.Assign) else [node.target]
            if (
                isinstance(value, str)
                and value in constants
                and any(
                    isinstance(target, ast.Name) and target.id.isupper()
                    for target in targets
                )
            ):
                findings.append(
                    source.finding(
                        "protocol-constant-copy",
                        node.lineno,
                        tuple(constants[value]),
                    )
                )
    return findings


def audit(
    root: Path, include_generated: bool = False, min_tokens: int = 60
) -> tuple[list[Finding], int]:
    findings, constants = alignment_findings(root)
    functions: list[Function] = []
    references: dict[str, Counter[str]] = defaultdict(Counter)
    scanned = 0
    for path in iter_files(root):
        relative = path.relative_to(root).as_posix()
        source = Source(relative, "", path.suffix, is_test(relative))
        try:
            source.text = path.read_text(encoding="utf-8")
            if not include_generated and generated(relative, source.text):
                continue
            if path.suffix in {".go", ".py"}:
                lex(source)
            scanned += 1
            findings.extend(regex_findings(source))
            findings.extend(python_findings(source))
            findings.extend(protocol_copy_findings(source, constants))
            references[source.suffix].update(
                token.value for token in source.tokens if token.kind == "name"
            )
            functions.extend(
                python_functions(source, min_tokens)
                if source.suffix == ".py"
                else go_functions(source, min_tokens)
            )
            # Keep only body fingerprints and symbol references across files.
            # Whole-repository AST/token retention consumes excessive memory.
            source.tree = None
            source.tokens.clear()
            source.clean = source.mask = ""
        except (OSError, UnicodeError, SyntaxError, tokenize.TokenError) as exc:
            findings.append(
                source.finding(
                    "scan-error", getattr(exc, "lineno", None) or 1, evidence=str(exc)
                )
            )
    findings.extend(structural_findings(functions, references))
    return sorted(
        set(findings), key=lambda item: (item.path, item.line, item.rule)
    ), scanned


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--root",
        type=Path,
        default=ROOT,
        help="Repository root, preserving owner-relative paths",
    )
    parser.add_argument(
        "--guidelines",
        type=Path,
        help="Guidelines file (default: <root>/docs/devs/devs.md)",
    )
    parser.add_argument("--min-violations", type=int, default=1)
    parser.add_argument(
        "--limit", type=int, default=25, help="Maximum ranked files; 0 means unlimited"
    )
    parser.add_argument(
        "--min-clone-tokens",
        type=int,
        default=60,
        help="Minimum substantial function size",
    )
    parser.add_argument("--include-generated", action="store_true")
    parser.add_argument(
        "--rule", action="append", choices=sorted(RULES), help="Repeat to select rules"
    )
    parser.add_argument(
        "--confidence",
        choices=CONFIDENCE_WEIGHT,
        default="low",
        help="Minimum confidence to report",
    )
    parser.add_argument(
        "--fail-on",
        choices=CONFIDENCE_WEIGHT,
        help="Exit 1 on findings at this confidence or higher, before display limits",
    )
    parser.add_argument("--list-rules", action="store_true")
    parser.add_argument("--json", action="store_true", dest="as_json")
    args = parser.parse_args(argv)
    if args.limit < 0 or args.min_violations < 1 or args.min_clone_tokens < 1:
        parser.error(
            "limit must be nonnegative; min-violations and min-clone-tokens must be positive"
        )
    return args


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    if args.list_rules:
        if args.as_json:
            print(json.dumps([asdict(rule) for rule in RULES.values()], indent=2))
        else:
            for rule in RULES.values():
                print(
                    f"{rule.rule} [{rule.confidence}] {rule.guideline}: {rule.description}"
                )
        return 0
    root = args.root.resolve()
    guidelines = (args.guidelines or root / "docs/devs/devs.md").resolve()
    if not root.is_dir() or not guidelines.is_file():
        print(
            f"error: root or guidelines missing: {root}, {guidelines}", file=sys.stderr
        )
        return 2
    findings, scanned = audit(root, args.include_generated, args.min_clone_tokens)
    # Coverage errors cannot be hidden by a rule or confidence selection.
    coverage_errors = [finding for finding in findings if finding.rule == "scan-error"]
    selected = [
        finding
        for finding in findings
        if (not args.rule or finding.rule in args.rule)
        and CONFIDENCE_WEIGHT[finding.confidence] >= CONFIDENCE_WEIGHT[args.confidence]
    ]
    by_file: dict[str, list[Finding]] = defaultdict(list)
    for finding in selected:
        by_file[finding.path].append(finding)
    score = lambda items: sum(CONFIDENCE_WEIGHT[item.confidence] for item in items)
    ranked = sorted(
        by_file.items(), key=lambda item: (-score(item[1]), -len(item[1]), item[0])
    )
    ranked = [item for item in ranked if len(item[1]) >= args.min_violations]
    if args.limit:
        ranked = ranked[: args.limit]
    guideline_path = (
        str(guidelines.relative_to(root))
        if guidelines.is_relative_to(root)
        else str(guidelines)
    )
    summary = {
        "scanned_files": scanned,
        "findings": len(selected),
        "files": len(by_file),
        "coverage_errors": len(coverage_errors),
        "by_rule": dict(sorted(Counter(item.rule for item in selected).items())),
    }
    if args.as_json:
        print(
            json.dumps(
                {
                    "guidelines": guideline_path,
                    "summary": summary,
                    "coverage_errors": [asdict(item) for item in coverage_errors],
                    "files": [
                        {
                            "path": path,
                            "score": score(items),
                            "violations": [asdict(item) for item in items],
                        }
                        for path, items in ranked
                    ],
                },
                indent=2,
            )
        )
    else:
        print(
            f"Developer-guideline triage: {len(selected)} findings in {len(by_file)} files; {scanned} files scanned"
        )
        print(f"Guidelines: {guideline_path}")
        for path, items in ranked:
            print(f"\n{score(items):3d} points / {len(items):3d} findings  {path}")
            for item in items:
                print(
                    f"  {item.line:4d} {item.rule} [{item.confidence}] ({item.guideline}): {item.text}"
                )
                for related in item.related:
                    print(
                        f"       related: {related.path}:{related.line} {related.symbol}"
                    )
        if coverage_errors:
            print(
                f"\nAudit incomplete: {len(coverage_errors)} coverage errors",
                file=sys.stderr,
            )
            for item in coverage_errors:
                print(f"{item.path}:{item.line}: {item.text}", file=sys.stderr)
        if not selected:
            print("No matching findings.")
    if coverage_errors:
        return 2
    return int(
        bool(
            args.fail_on
            and any(
                CONFIDENCE_WEIGHT[item.confidence] >= CONFIDENCE_WEIGHT[args.fail_on]
                for item in selected
            )
        )
    )


if __name__ == "__main__":
    raise SystemExit(main())
