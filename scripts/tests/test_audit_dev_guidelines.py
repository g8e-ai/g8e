# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Behavioral regression tests; no Go compiler or Ensemble dependencies needed."""

import contextlib
import importlib.util
import io
import json
import sys
import tempfile
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location(
    "guideline_audit", Path(__file__).parents[1] / "audit-dev-guidelines.py"
)
AUDIT = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = AUDIT
SPEC.loader.exec_module(AUDIT)


class AuditTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.write("docs/devs/devs.md", "Developer guidelines")

    def write(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        return path

    def findings(self, **kwargs):
        return AUDIT.audit(self.root, **kwargs)[0]

    def rules(self, **kwargs):
        return [item.rule for item in self.findings(**kwargs)]

    def cli(self, *args):
        output, errors = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            code = AUDIT.main(["--root", str(self.root), *args])
        return code, output.getvalue(), errors.getvalue()

    def test_multiline_call_and_comment_string_exclusions(self):
        self.write(
            "internal/services/example.go",
            """package example
import "errors"
// errors.New("comment")
/* panic("comment") */
var documentation = `errors.New("example")`
func execute() error {
    return errors.New(
        "failure",
    )
}
""",
        )
        found = [f for f in self.findings() if f.rule == "errors-new"]
        self.assertEqual([f.line for f in found], [7])

    def test_aliases_preserve_error_and_environment_checks(self):
        self.write(
            "internal/example.go",
            """package example
import (
    failures "errors"
    system "os"
)
func execute() error {
    system.Getenv("SWITCH")
    return failures.New("failure")
}
""",
        )
        self.assertIn("errors-new", self.rules())
        self.assertIn("raw-env-key", self.rules())

    def test_only_canonical_error_file_is_exempt(self):
        self.write(
            "internal/constants/errors.go",
            'package constants\nvar Owned = errors.New("owned")',
        )
        self.write(
            "internal/constants/other.go",
            'package constants\nvar Other = errors.New("other")',
        )
        self.assertEqual(
            [f.path for f in self.findings() if f.rule == "errors-new"],
            ["internal/constants/other.go"],
        )

    def test_runtime_owner_exempt_but_multiline_consumer_reported(self):
        text = """package example
func load() {
    os.ReadFile(
        RuntimeDir,
    )
    filepath.Join(
        base, "pki",
    )
}
"""
        self.write("internal/services/fs/file_service.go", text)
        self.write("internal/services/example.go", text)
        findings = [
            f
            for f in self.findings()
            if f.rule in {"direct-runtime-io", "filepath-literal"}
        ]
        self.assertEqual(
            {f.rule for f in findings}, {"direct-runtime-io", "filepath-literal"}
        )
        self.assertEqual({f.path for f in findings}, {"internal/services/example.go"})

    def test_test_only_rules_do_not_report_injected_production_errors(self):
        self.write(
            "test/integration/example_test.go",
            """package example
func TestBehavior(t *testing.T) {
    t.Parallel()
    panic("injected")
    errors.New("injected")
    os.Getenv("TEST_SWITCH")
    os.Chdir("fixture")
}
""",
        )
        self.assertEqual(set(self.rules()), {"parallel-integration-test", "os-chdir"})

    def test_build_tag_recognizes_integration_test(self):
        self.write(
            "internal/example_test.go",
            "//go:build integration && linux\npackage example\nfunc TestBehavior(t *testing.T) { t.Parallel() }",
        )
        self.assertIn("parallel-integration-test", self.rules())

    def test_walk_prunes_worktrees_vendor_and_symlinks(self):
        for prefix in (
            "vendor",
            "node_modules",
            ".claude/worktrees/old",
            ".g8e-backup",
            ".venv",
        ):
            self.write(
                f"{prefix}/example.go",
                'package example\nfunc execute() { panic("bad") }',
            )
        self.write(
            "internal/example.go", 'package example\nfunc execute() { panic("bad") }'
        )
        (self.root / "linked").symlink_to(
            self.root / "internal", target_is_directory=True
        )
        self.assertEqual(
            [f.path for f in self.findings() if f.rule == "panic-production"],
            ["internal/example.go"],
        )

    def test_generated_headers_and_language_outputs_are_excluded(self):
        for name, header in (
            ("internal/example_gen.go", ""),
            ("internal/example.go", "// Code generated by owner. DO NOT EDIT.\n"),
            ("protocol/python/g8e/common/v1/common_pb2.py", ""),
        ):
            self.write(
                name,
                header
                + (
                    'package example\nfunc execute() { panic("bad") }'
                    if name.endswith("go")
                    else 'raise ValueError("bad")'
                ),
            )
        self.assertEqual(self.findings(), [])
        self.assertIn("panic-production", self.rules(include_generated=True))

    def test_python_ast_checks_resolve_aliases_and_ignore_docstrings(self):
        self.write(
            "ensemble/app/service.py",
            '''import os as system
from subprocess import run as execute
from pydantic import BaseModel
"""raise ValueError('example'); os.getenv('example')"""
def load():
    key = system.environ["SECRET"]
    key = system.getenv("SECRET")
    execute(["whoami"])
    try:
        operation()
    except Exception:
        pass
    raise ValueError("failure")
''',
        )
        rules = self.rules()
        self.assertEqual(rules.count("raw-env-key"), 2)
        for rule in (
            "python-model-hub",
            "local-execution-bypass",
            "swallowed-exception",
            "python-untyped-error",
        ):
            self.assertEqual(rules.count(rule), 1)

    def test_python_model_owner_and_tests_are_exempt(self):
        self.write("ensemble/app/models/base.py", "from pydantic import BaseModel")
        self.write(
            "ensemble/tests/test_behavior.py",
            'from pydantic import BaseModel\nraise ValueError("injected")',
        )
        self.assertEqual(self.findings(), [])

    def test_go_clones_handle_strings_braces_and_renamed_locals(self):
        body = """
    value := "{literal}"
    if request == nil {
        return value
    }
    value = transform(request)
    record(value)
    return value
"""
        self.write(
            "internal/a.go",
            "package example\nfunc first(request *Request) string {" + body + "}",
        )
        self.write(
            "internal/b.go",
            "package example\nfunc second(request *Request) string {" + body + "}",
        )
        self.write(
            "internal/c.go",
            "package example\nfunc third(input *Request) string {"
            + body.replace("request", "input").replace("value", "result")
            + "}",
        )
        found = self.findings(min_tokens=15)
        self.assertEqual(len([f for f in found if f.rule == "duplicate-function"]), 2)
        clones = [f for f in found if f.rule == "renamed-function-clone"]
        self.assertEqual(len(clones), 1)
        self.assertEqual({loc.symbol for loc in clones[0].related}, {"first", "second"})

    def test_python_clones_preserve_calls_and_literals(self):
        body = """def NAME(ARG):
    VALUE = parse(ARG)
    VALUE = validate(VALUE)
    record(VALUE, "literal")
    return VALUE
"""
        self.write(
            "ensemble/app/a.py",
            body.replace("NAME", "first")
            .replace("ARG", "request")
            .replace("VALUE", "value"),
        )
        self.write(
            "ensemble/app/b.py",
            body.replace("NAME", "second")
            .replace("ARG", "input")
            .replace("VALUE", "result"),
        )
        self.write(
            "ensemble/app/c.py",
            body.replace("NAME", "third")
            .replace("ARG", "input")
            .replace("VALUE", "result")
            .replace('"literal"', '"different"'),
        )
        self.assertEqual(
            len(
                [
                    f
                    for f in self.findings(min_tokens=15)
                    if f.rule == "renamed-function-clone"
                ]
            ),
            2,
        )
        self.assertFalse(
            any(
                f.path.endswith("c.py") and "clone" in f.rule
                for f in self.findings(min_tokens=15)
            )
        )

    def test_small_functions_not_clones_but_unused_private_still_found(self):
        self.write(
            "ensemble/app/a.py",
            "def _unused():\n    return 1\ndef public():\n    return 1",
        )
        found = self.findings()
        self.assertEqual(
            [(f.rule, f.text) for f in found], [("unused-private-helper", "_unused")]
        )

    def test_reference_as_callback_or_in_tests_prevents_unused_report(self):
        self.write(
            "ensemble/app/a.py",
            "def _callback():\n    return 1\ndef _tested():\n    return 2\nregister(_callback)",
        )
        self.write(
            "ensemble/tests/test_callbacks.py",
            "from app.a import _tested\nassert _tested() == 2",
        )
        self.assertNotIn("unused-private-helper", self.rules())

    def test_recursion_is_not_external_reference(self):
        self.write("internal/example.go", "package example\nfunc unused() { unused() }")
        self.assertIn("unused-private-helper", self.rules())

    def test_registry_comparison_is_semantic_and_detects_drift(self):
        self.write(
            "protocol/constants/collections.json",
            '{"collections":{"tasks":{"value":"tasks"}}}',
        )
        self.write(
            "protocol/python/g8e/_data/collections.json",
            '{\n "collections": {"tasks": {"value": "tasks"}}\n}',
        )
        self.assertNotIn("protocol-registry-drift", self.rules())
        self.write("protocol/python/g8e/_data/collections.json", '{"collections":{}}')
        found = next(f for f in self.findings() if f.rule == "protocol-registry-drift")
        self.assertEqual(
            found.related[0].path, "protocol/python/g8e/_data/collections.json"
        )

    def test_protocol_copy_finds_owned_identifiers_but_not_arbitrary_text(self):
        self.write(
            "protocol/constants/collections.json",
            json.dumps(
                {
                    "collections": {
                        "records": {
                            "value": "agent_records",
                            "_python_const": "AGENT_RECORDS",
                        }
                    }
                }
            ),
        )
        self.write(
            "ensemble/app/constants/collections.py",
            'RECORDS = "agent_records"\ndescription = "agent_records"',
        )
        self.assertEqual(
            len([f for f in self.findings() if f.rule == "protocol-constant-copy"]), 1
        )

    def test_codemap_checks_literal_owners_but_skips_patterns(self):
        self.write(
            "docs/devs/codemap.md",
            "`internal/missing.go`\n`ensemble/app/*.py`\n`protocol/<package>/`",
        )
        found = next(f for f in self.findings() if f.rule == "codemap-missing-owner")
        self.assertEqual((found.line, found.text), (1, "internal/missing.go"))

    def test_fail_on_ignores_display_truncation_and_count_filter(self):
        self.write(
            "internal/example.go",
            'package example\nfunc execute() error { return errors.New("failure") }',
        )
        code, output, _ = self.cli(
            "--fail-on", "high", "--min-violations", "999", "--json"
        )
        self.assertEqual(code, 1)
        parsed = json.loads(output)
        self.assertEqual(parsed["files"], [])
        self.assertEqual(parsed["summary"]["by_rule"]["errors-new"], 1)

    def test_coverage_errors_cannot_be_hidden_by_rule_filters(self):
        self.write("ensemble/app/broken.py", "def broken(:\n")
        code, output, _ = self.cli("--rule", "duplicate-function", "--json")
        self.assertEqual(code, 2)
        self.assertEqual(json.loads(output)["summary"]["coverage_errors"], 1)
        self.assertEqual(len(json.loads(output)["coverage_errors"]), 1)

    def test_rule_selection_and_confidence_filters(self):
        self.write(
            "internal/example.go", 'package example\nfunc unused() { panic("failure") }'
        )
        code, output, _ = self.cli(
            "--rule",
            "unused-private-helper",
            "--confidence",
            "high",
            "--fail-on",
            "high",
            "--json",
        )
        self.assertEqual(code, 0)
        self.assertEqual(json.loads(output)["summary"]["findings"], 0)

    def test_raw_environment_keys_cover_backticks_keywords_and_import_aliases(self):
        self.write(
            "internal/example.go",
            "package example\nfunc execute() { os.Getenv(`SWITCH`) }",
        )
        self.write(
            "ensemble/app/example.py",
            'from os import getenv as read, environ as env\nread(key="SWITCH")\nenv["SECRET"]',
        )
        self.assertEqual(self.rules().count("raw-env-key"), 3)

    def test_missing_bundled_registry_is_reported_when_bundle_tree_exists(self):
        self.write("protocol/constants/collections.json", '{"collections": {}}')
        (self.root / "protocol/python/g8e/_data").mkdir(parents=True)
        self.assertIn("protocol-registry-drift", self.rules())

    def test_production_directory_suffix_does_not_imply_test(self):
        self.write(
            "internal/contest/handler.go",
            'package contest\nfunc execute() { panic("failure") }',
        )
        self.assertIn("panic-production", self.rules())

    def test_named_go_return_struct_does_not_confuse_clone_extraction(self):
        body = """{
    result := struct { Value string }{Value: "literal"}
    record(result)
    validate(result)
    record(result)
    validate(result)
    return result
}"""
        self.write(
            "internal/a.go",
            "package example\nfunc first() struct { Value string } " + body,
        )
        self.write(
            "internal/b.go",
            "package example\nfunc second() struct { Value string } " + body,
        )
        self.assertEqual(self.rules(min_tokens=15).count("duplicate-function"), 2)

    def test_literal_go_registry_parity_includes_generated_owners(self):
        self.write(
            "protocol/constants/collections.json",
            '{"collections":{"records":{"value":"agent_records","_go_const":"CollectionRecords"}}}',
        )
        path = "internal/constants/collections_gen.go"
        self.write(
            path,
            '// Code generated by owner. DO NOT EDIT.\npackage constants\nconst (\n CollectionRecords CollectionName = "agent_records" // comment\n)',
        )
        self.assertNotIn("protocol-go-constant-drift", self.rules())
        self.write(
            path,
            'package constants\nconst CollectionRecords CollectionName = "other_records"',
        )
        found = next(
            f for f in self.findings() if f.rule == "protocol-go-constant-drift"
        )
        self.assertEqual(found.related[0].symbol, "CollectionRecords")
        self.assertEqual(found.related[0].line, 2)

    def test_computed_go_constant_is_left_to_conformance_checks(self):
        self.write(
            "protocol/constants/collections.json",
            '{"collections":{"records":{"value":"agent_records","_go_const":"CollectionRecords"}}}',
        )
        self.write(
            "internal/constants/collections.go",
            'package constants\nconst CollectionRecords = "agent_" + "records"',
        )
        self.assertNotIn("protocol-go-constant-drift", self.rules())

    def test_rule_catalog_does_not_require_repository(self):
        code, output, _ = self.cli("--list-rules", "--json")
        self.assertEqual(code, 0)
        self.assertIn(
            "duplicate-function", {rule["rule"] for rule in json.loads(output)}
        )


if __name__ == "__main__":
    unittest.main()
