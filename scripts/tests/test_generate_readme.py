#!/usr/bin/env python3
# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, 2.0.

"""Unit tests for scripts/generate_readme.py.

Tests run with the standard library only and exercise validation, aggregation,
rendering, safety, and drift behavior using synthetic fixtures.
"""

import json
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import generate_readme as gr


FIXTURES = Path(__file__).resolve().parent / "fixtures" / "readme"
VALID = FIXTURES / "valid"
TEMPLATE = VALID.parent.parent.parent.parent.parent / "docs" / "templates" / "README.md.tmpl"


class TestLoadSnapshot(unittest.TestCase):
    def test_load_valid_snapshot(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        self.assertEqual(snapshot.manifest.publication_schema_version, "1.0.0")
        self.assertEqual(len(snapshot.eval_runs), 1)
        self.assertEqual(snapshot.receipt_verification.total_receipts, 9)
        self.assertEqual(len(snapshot.demo_reports), 1)

    def test_missing_index(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp))
        self.assertIn("missing index.json", str(ctx.exception))

    def test_unsupported_publication_schema(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            index = Path(tmp) / "index.json"
            index.write_text(json.dumps({"publication_schema_version": "9.9.9"}))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp))
        self.assertIn("unsupported publication_schema_version", str(ctx.exception))

    def test_path_traversal_blocked(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            data_dir = Path(tmp) / "data"
            data_dir.mkdir()
            # Copy valid manifest for eval_runs to satisfy minimum eval run check.
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["receipt_verification"]["result_path"] = "../evil.json"
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("path traversal", str(ctx.exception))

    def test_checksum_mismatch(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["manifest_sha256"] = "0" * 64
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("checksum mismatch", str(ctx.exception))

    def test_undeclared_artifact_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            (Path(tmp) / "snap" / "extra.txt").write_text("secret")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("undeclared artifact", str(ctx.exception))

    def test_duplicate_run_id(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"].append(index["eval_runs"][0])
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("duplicate run_id", str(ctx.exception))

    def test_unsupported_eval_schema(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            manifest_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "manifest.json"
            manifest = json.loads(manifest_path.read_text())
            manifest["schema_version"] = "0.0.0"
            manifest_path.write_text(json.dumps(manifest))
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["manifest_sha256"] = gr._sha256_file(manifest_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("unsupported eval schema version", str(ctx.exception))

    def test_duplicate_metric_row(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            with metrics_path.open("a") as f:
                f.write(metrics_path.read_text().splitlines()[0] + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("duplicate metric row", str(ctx.exception))

    def test_metric_references_missing_attempt(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            lines = metrics_path.read_text().splitlines()
            first = json.loads(lines[0])
            first["evidence_ref"] = "attempts/missing-attempt"
            lines[0] = json.dumps(first)
            metrics_path.write_text("\n".join(lines) + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("missing attempt", str(ctx.exception))

    def test_non_finite_metric_value(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            lines = metrics_path.read_text().splitlines()
            first = json.loads(lines[0])
            first["value"] = float("nan")
            lines[0] = json.dumps(first)
            metrics_path.write_text("\n".join(lines) + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("must be finite", str(ctx.exception))

    def test_ineligible_metric_with_zero_denominator(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            lines = metrics_path.read_text().splitlines()
            first = json.loads(lines[0])
            first["eligible"] = True
            first["denominator_contribution"] = 0
            lines[0] = json.dumps(first)
            metrics_path.write_text("\n".join(lines) + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("positive denominator", str(ctx.exception))


class TestProjectMetrics(unittest.TestCase):
    def test_project_eligible_only(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        projections = gr._project_metrics(snapshot.eval_runs)
        for key, p in projections.items():
            self.assertTrue(p.denominator > 0)
            self.assertTrue(0.0 <= p.rate <= 1.0)

    def test_pass_fail_rate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            metrics_path = Path(tmp) / "snap" / "eval" / "runs" / "run-2026-09-01-synthetic-a" / "metrics.jsonl"
            lines = metrics_path.read_text().splitlines()
            filtered = [line for line in lines if '"metric_id":"ifeval_subset_verifier"' in line and '"arm_id":"baseline"' in line]
            metrics_path.write_text("\n".join(filtered) + "\n")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["eval_runs"][0]["metrics_sha256"] = gr._sha256_file(metrics_path)
            index_path.write_text(json.dumps(index))
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
            projections = gr._project_metrics(snapshot.eval_runs)
            baseline = projections["ifeval_subset_verifier__baseline"]
            self.assertEqual(baseline.denominator, 5)
            self.assertEqual(baseline.numerator, 3)
            self.assertAlmostEqual(baseline.rate, 0.6)


class TestRenderReadme(unittest.TestCase):
    def test_markers_rendered(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        template = TEMPLATE.read_text()
        rendered = gr.render_readme(snapshot, template)
        self.assertIn("Generated by scripts/generate_readme.py", rendered)
        self.assertIn("### Eval Metrics", rendered)
        self.assertIn("### Receipt Verification", rendered)
        self.assertIn("### Governance and State Proof", rendered)
        self.assertIn("### Independently Verified Demonstrations", rendered)
        self.assertIn("### Evidence Identity", rendered)
        self.assertIn("### CI and Reproducibility", rendered)

    def test_missing_marker_fails(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        template = "{{EVAL_METRICS}}"
        with self.assertRaises(gr.ReadmeError) as ctx:
            gr.render_readme(snapshot, template)
        self.assertIn("missing template markers", str(ctx.exception))

    def test_unknown_marker_fails(self) -> None:
        snapshot = gr.load_snapshot(VALID)
        template = "{{UNKNOWN}}"
        with self.assertRaises(gr.ReadmeError) as ctx:
            gr.render_readme(snapshot, template)
        self.assertIn("unknown template marker", str(ctx.exception))

    def test_html_injection_escaped(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["platform_version"] = "<script>alert(1)</script>"
            index_path.write_text(json.dumps(index))
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
            template = TEMPLATE.read_text()
            rendered = gr.render_readme(snapshot, template)
            self.assertIn("&lt;script&gt;alert(1)&lt;/script&gt;", rendered)
            self.assertNotIn("<script>alert(1)</script>", rendered)

    def test_invalid_demo_report_blocks_render(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            report_path = Path(tmp) / "snap" / "demo" / "demo-allow-001" / "compliance-report.json"
            report = json.loads(report_path.read_text())
            report["valid"] = False
            report_path.write_text(json.dumps(report))
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["demo_reports"][0]["report_sha256"] = gr._sha256_file(report_path)
            index_path.write_text(json.dumps(index))
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
            template = TEMPLATE.read_text()
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.render_readme(snapshot, template)
        self.assertIn("invalid or has failures", str(ctx.exception))

    def test_zero_receipts_not_a_pass(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            receipt_path = Path(tmp) / "snap" / "eval" / "receipt-verification.json"
            receipt = json.loads(receipt_path.read_text())
            receipt["total_receipts"] = 0
            receipt["verified_signatures"] = 0
            receipt["verified_persistence"] = 0
            receipt_path.write_text(json.dumps(receipt))
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["receipt_verification"]["result_sha256"] = gr._sha256_file(receipt_path)
            index_path.write_text(json.dumps(index))
            snapshot = gr.load_snapshot(Path(tmp) / "snap")
            template = TEMPLATE.read_text()
            rendered = gr.render_readme(snapshot, template)
            self.assertIn("| Pass | no |", rendered)

    def test_unsafe_ci_label_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["ci_links"].append({"label": "<b>Evil</b>", "url": "https://example.com", "kind": "workflow_status"})
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("unsupported ci link label", str(ctx.exception))

    def test_unsupported_claim_label_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["claim_labels"].append("custom_unsupported_claim")
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("unsupported claim label", str(ctx.exception))


class TestGenerate(unittest.TestCase):
    def test_generate_atomic_and_deterministic(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            gr.generate(TEMPLATE, VALID, out)
            first = out.read_text()
            gr.generate(TEMPLATE, VALID, out)
            second = out.read_text()
            self.assertEqual(first, second)
            self.assertTrue(out.exists())

    def test_check_passes_when_identical(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            gr.generate(TEMPLATE, VALID, out)
            gr.generate(TEMPLATE, VALID, out, check=True)

    def test_check_fails_on_drift(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            out.write_text("stale readme")
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.generate(TEMPLATE, VALID, out, check=True)
        self.assertIn("drift check failed", str(ctx.exception))

    def test_check_fails_when_output_missing(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.generate(TEMPLATE, VALID, out, check=True)
        self.assertIn("does not exist", str(ctx.exception))


class TestMain(unittest.TestCase):
    def test_main_generate_and_check(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            code = gr.main([
                "--snapshot-dir", str(VALID),
                "--template", str(TEMPLATE),
                "--output", str(out),
            ])
            self.assertEqual(code, 0)
            code = gr.main([
                "--check",
                "--snapshot-dir", str(VALID),
                "--template", str(TEMPLATE),
                "--output", str(out),
            ])
            self.assertEqual(code, 0)

    def test_main_check_fails_on_drift(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "README.md"
            out.write_text("stale")
            code = gr.main([
                "--check",
                "--snapshot-dir", str(VALID),
                "--template", str(TEMPLATE),
                "--output", str(out),
            ])
            self.assertEqual(code, 1)

    def test_main_invalid_snapshot_fails(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            code = gr.main([
                "--snapshot-dir", str(tmp),
                "--template", str(TEMPLATE),
                "--output", str(Path(tmp) / "README.md"),
            ])
            self.assertEqual(code, 1)


class TestSafetyScans(unittest.TestCase):
    def test_private_key_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            (Path(tmp) / "snap" / "keys").mkdir()
            (Path(tmp) / "snap" / "keys" / "actuator.pem").write_text(
                "-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----\n"
            )
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("undeclared artifact", str(ctx.exception))

    def test_absolute_path_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            shutil.copytree(VALID, Path(tmp) / "snap")
            index_path = Path(tmp) / "snap" / "index.json"
            index = json.loads(index_path.read_text())
            index["receipt_verification"]["result_path"] = "/etc/passwd"
            index_path.write_text(json.dumps(index))
            with self.assertRaises(gr.ReadmeError) as ctx:
                gr.load_snapshot(Path(tmp) / "snap")
        self.assertIn("absolute path", str(ctx.exception))


class TestFormatRate(unittest.TestCase):
    def test_format_rate(self) -> None:
        self.assertEqual(gr._format_rate(1.0), "100.0%")
        self.assertEqual(gr._format_rate(0.0), "0.0%")
        self.assertEqual(gr._format_rate(0.5), "50.0%")


if __name__ == "__main__":
    unittest.main()
