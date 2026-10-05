# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Host startup behavior without Gateway, GPU, or Ollama dependencies."""

import contextlib
import importlib.util
import io
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location(
    "full", Path(__file__).parents[1] / "full.py"
)
FULL = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(FULL)


class FullTests(unittest.TestCase):
    def test_ensemble_start_uses_this_python_and_shared_runtime(self):
        with patch.object(FULL, "start_local") as launch:
            FULL.start_ensemble()
        call = launch.call_args
        self.assertEqual(call.args[1], FULL.ROOT / ".local.dev/full/ensemble")
        self.assertEqual(call.args[2][:3], [sys.executable, "-m", "app.serve"])

    def test_stop_cleans_stale_pid_without_signalling(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            pid_file = root / ".local.dev/full/ensemble/full.pid"
            pid_file.parent.mkdir(parents=True)
            pid_file.write_text("99999999")
            with (
                patch.object(FULL, "ROOT", root),
                patch.object(FULL, "ensemble_running", return_value=False),
                patch.object(FULL.os, "kill") as kill,
            ):
                FULL.stop_ensemble()
            kill.assert_not_called()
            self.assertFalse(pid_file.exists())

    def test_stop_terminates_local_ensemble_process(self):
        with tempfile.TemporaryDirectory() as temp:
            process = subprocess.Popen(
                [sys.executable, "-c", "import time; time.sleep(30)", "app.serve"]
            )
            self.addCleanup(process.wait)
            self.addCleanup(
                lambda: process.terminate() if process.poll() is None else None
            )
            root = Path(temp)
            pid_file = root / ".local.dev/full/ensemble/full.pid"
            pid_file.parent.mkdir(parents=True)
            pid_file.write_text(str(process.pid))
            with patch.object(FULL, "ROOT", root):
                FULL.stop_ensemble()
            self.assertIsNotNone(process.poll())
            self.assertFalse(pid_file.exists())

    def test_ensemble_dry_run_restart_does_not_stop(self):
        with (
            patch.object(
                sys, "argv", ["full.py", "--ensemble-action", "restart", "--dry-run"]
            ),
            patch.object(FULL, "stop_ensemble") as stop,
            patch.object(FULL, "start_ensemble") as start,
        ):
            FULL.main()
        stop.assert_not_called()
        start.assert_called_once_with(True)

    def test_ensemble_action_skips_operator_prompts(self):
        with (
            patch.object(sys, "argv", ["full.py", "--ensemble-action", "restart"]),
            patch.object(FULL, "stop_ensemble") as stop,
            patch.object(FULL, "start_ensemble") as start,
            patch.object(FULL, "prepare_identities") as prepare,
            patch.object(FULL, "report_workloads", return_value=False),
            patch("builtins.input") as prompt,
        ):
            FULL.main()
        stop.assert_called_once()
        start.assert_called_once_with(False)
        prompt.assert_not_called()
        prepare.assert_called_once_with([], False, False)

    def setup_identities(self, root):
        current = (
            "-----BEGIN CERTIFICATE-----\nY3VycmVudA==\n-----END CERTIFICATE-----\n"
        )
        old = "-----BEGIN CERTIFICATE-----\nb2xk\n-----END CERTIFICATE-----\n"
        ca = root / ".g8e/pki/root/root_ca.crt"
        ca.parent.mkdir(parents=True)
        ca.write_text(current)
        bundle = root / FULL.TRUST_PATH
        bundle.parent.mkdir(parents=True)
        bundle.write_text(current)
        directory = root / "operator"
        saved = directory / FULL.TRUST_PATH
        saved.parent.mkdir(parents=True)
        saved.write_text(old)
        return directory, saved, current

    def test_stale_identity_declined_changes_nothing(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            directory, saved, _ = self.setup_identities(root)
            before = saved.read_bytes()
            with (
                patch.object(FULL, "ROOT", root),
                patch("builtins.input", return_value="n"),
                patch.object(FULL.subprocess, "run") as run,
                self.assertRaisesRegex(RuntimeError, "declined"),
            ):
                FULL.prepare_identities([("observer", "localhost", str(directory))])
            run.assert_not_called()
            self.assertEqual(saved.read_bytes(), before)
            self.assertFalse((root / FULL.WORKLOADS_PATH).exists())

    def test_confirmed_reset_scoped_to_selected_local_workloads(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            directory, saved, current = self.setup_identities(root)
            with (
                patch.object(FULL, "ROOT", root),
                patch.object(FULL.subprocess, "run") as run,
                patch("builtins.input") as prompt,
            ):
                FULL.prepare_identities(
                    [
                        ("observer", "localhost", str(directory)),
                        ("provenance", "remote", "/remote"),
                    ],
                    reset=True,
                )
            prompt.assert_not_called()
            self.assertEqual(
                run.call_args.args[0][1:],
                [
                    "operator",
                    "reset-identity",
                    "--working-dir",
                    str(directory),
                    "--yes",
                ],
            )
            self.assertEqual(run.call_count, 1)
            self.assertEqual(saved.read_text(), current)
            self.assertEqual(
                (root / ".local.dev/full/ensemble" / FULL.TRUST_PATH).read_text(),
                current,
            )

    def test_same_ca_does_not_reset_or_prompt(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            directory, saved, current = self.setup_identities(root)
            saved.write_text(current)
            with (
                patch.object(FULL, "ROOT", root),
                patch.object(FULL.subprocess, "run") as run,
                patch("builtins.input") as prompt,
            ):
                FULL.prepare_identities([("observer", "localhost", str(directory))])
            prompt.assert_not_called()
            run.assert_not_called()

    def test_preflight_refuses_symlink_before_any_reset(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            directory, saved, _ = self.setup_identities(root)
            saved.unlink()
            saved.symlink_to(root / FULL.TRUST_PATH)
            with (
                patch.object(FULL, "ROOT", root),
                patch.object(FULL.subprocess, "run") as run,
                self.assertRaisesRegex(RuntimeError, "symlink"),
            ):
                FULL.prepare_identities(
                    [("observer", "localhost", str(directory))], reset=True
                )
            run.assert_not_called()

    def test_startup_report_ignores_previous_connection_and_detects_exit(self):
        with tempfile.TemporaryDirectory() as temp:
            directory = Path(temp)
            (directory / "full.pid").write_text("123")
            log = directory / "full.log"
            old = "operator pub/sub WebSocket connected\n"
            log.write_text(old + "operator enrollment: request submitted\n")
            with patch.object(FULL, "ensemble_running", return_value=True):
                self.assertEqual(
                    FULL.workload_state("observer", directory, len(old))[0],
                    "awaiting approval",
                )
            with patch.object(FULL, "ensemble_running", return_value=False):
                self.assertEqual(
                    FULL.workload_state("observer", directory)[0], "failed"
                )

    def test_reset_failure_does_not_replace_trust(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            directory, saved, _ = self.setup_identities(root)
            before = saved.read_bytes()
            with (
                patch.object(FULL, "ROOT", root),
                patch.object(
                    FULL.subprocess,
                    "run",
                    side_effect=subprocess.CalledProcessError(1, []),
                ),
                self.assertRaises(subprocess.CalledProcessError),
            ):
                FULL.prepare_identities(
                    [("observer", "localhost", str(directory))], reset=True
                )
            self.assertEqual(saved.read_bytes(), before)

    def test_dry_run_never_starts_or_creates_state(self):
        with tempfile.TemporaryDirectory() as temp:
            target = Path(temp) / "absent"
            with patch.object(FULL.subprocess, "Popen") as launch:
                FULL.start_local("test", target, ["unused"], dry_run=True)
            launch.assert_not_called()
            self.assertFalse(target.exists())

    def test_start_is_detached_and_keeps_identity_cwd(self):
        with tempfile.TemporaryDirectory() as temp:
            process = subprocess.Popen(
                [sys.executable, "-c", "import time; time.sleep(30)"]
            )
            self.addCleanup(process.wait)
            self.addCleanup(process.terminate)
            with patch.object(FULL.subprocess, "Popen", return_value=process) as launch:
                FULL.start_local("observer", temp, ["g8e", "operator", "start"])
                FULL.start_local("observer", temp, ["g8e", "operator", "start"])
            self.assertEqual(launch.call_count, 1)
            self.assertEqual(launch.call_args.kwargs["cwd"], Path(temp))
            self.assertTrue(launch.call_args.kwargs["start_new_session"])
            self.assertEqual((Path(temp) / "full.pid").read_text(), f"{process.pid}\n")

    def test_failed_launch_is_reported_and_pid_removed(self):
        with tempfile.TemporaryDirectory() as temp:
            with self.assertRaisesRegex(RuntimeError, "full.log"):
                FULL.start_local(
                    "g8ee", temp, [sys.executable, "-c", "raise SystemExit(7)"]
                )
            self.assertFalse((Path(temp) / "full.pid").exists())

    def test_role_identity_collision_fails_before_launch(self):
        answers = [
            "localhost",
            "/tmp/shared",
            "/tmp/models",
            "127.0.0.1",
            "/tmp/shared",
            "localhost",
            "/tmp/observer",
            "http://localhost:11434",
        ]
        with (
            patch.object(sys, "argv", ["full.py", "--dry-run"]),
            patch("builtins.input", side_effect=answers),
            self.assertRaisesRegex(ValueError, "separate working directory"),
        ):
            FULL.main()

    def test_remote_roles_are_printed_and_g8ee_remains_local(self):
        answers = [
            "windows",
            "C:\\g8e\\provenance",
            "windows",
            "C:\\g8e\\observer",
            "C:\\Users\\bob\\.ollama\\models",
            "localhost",
            "/tmp/inference",
            "http://192.168.1.2:11434",
            "localhost",
            "/tmp/data",
            "g8e.local",
        ]
        output = io.StringIO()
        with (
            patch.object(sys, "argv", ["full.py", "--dry-run"]),
            patch("builtins.input", side_effect=answers),
            patch.object(FULL, "start_local") as launch,
            contextlib.redirect_stdout(output),
        ):
            FULL.main()
        self.assertEqual(
            [call.args[0] for call in launch.call_args_list], ["inference", "data", "g8ee"]
        )
        self.assertIn("http://192.168.1.2:11434", launch.call_args_list[0].args[2])
        self.assertIn("Windows PowerShell", output.getvalue())
        self.assertIn("--model-storage-root", output.getvalue())
        self.assertNotIn("--inference-enabled", output.getvalue())

    def test_summary_uses_live_status_and_cli_management(self):
        output = io.StringIO()
        results = [
            subprocess.CompletedProcess(
                [], 0, "  Gateway    online\n  Operators  2 connected\n", ""
            ),
            subprocess.CompletedProcess([], 0, "  g8ee       running; not ready\n", ""),
        ]
        with (
            patch.object(FULL.subprocess, "run", side_effect=results) as run,
            contextlib.redirect_stdout(output),
        ):
            FULL.print_summary()
        self.assertEqual(run.call_args_list[0].args[0][1:], ["gw", "status", "--brief"])
        self.assertEqual(run.call_args_list[1].args[0][1:], ["ensemble", "status"])
        text = output.getvalue()
        self.assertIn("2 connected", text)
        self.assertIn("running; not ready", text)
        self.assertIn("./g8e operator list", text)
        self.assertIn("./g8e ensemble logs", text)
        self.assertNotIn("kill", text)
        self.assertLessEqual(len(text.splitlines()), 24)

    def test_summary_probe_failure_keeps_management_guidance(self):
        output = io.StringIO()
        with (
            patch.object(FULL.subprocess, "run", side_effect=OSError("unavailable")),
            contextlib.redirect_stdout(output),
        ):
            FULL.print_summary(has_remote=True)
        self.assertIn("status unavailable", output.getvalue())
        self.assertIn("Remote roles need", output.getvalue())
        self.assertIn("./g8e auth enroll pending", output.getvalue())

    def test_dry_run_summary_does_not_probe_services(self):
        with patch.object(FULL.subprocess, "run") as run:
            FULL.print_summary(dry_run=True)
        run.assert_not_called()

    def test_operator_args_roles(self):
        prov = FULL.operator_args("provenance", "localhost:8443", "/dir/prov", "/dir/models", "")
        self.assertIn("--provenance-operator-enabled", prov)
        self.assertIn("--model-storage-root", prov)

        infer = FULL.operator_args("inference", "localhost:8443", "/dir/infer", "", "http://localhost:11434")
        self.assertIn("--inference-enabled", infer)
        self.assertIn("--inference-ollama-endpoint", infer)

        obs = FULL.operator_args("observer", "localhost:8443", "/dir/obs", "", "")
        self.assertIn("--provider-boundary-observer-enabled", obs)

        data = FULL.operator_args("data", "localhost:8443", "/dir/data", "", "")
        self.assertNotIn("--provenance-operator-enabled", data)
        self.assertNotIn("--inference-enabled", data)
        self.assertNotIn("--provider-boundary-observer-enabled", data)
        self.assertEqual(data, ["operator", "start", "--endpoint", "localhost:8443", "--working-dir", "/dir/data"])


if __name__ == "__main__":
    unittest.main()
