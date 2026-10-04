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
            self.addCleanup(lambda: process.terminate() if process.poll() is None else None)
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
            patch.object(sys, "argv", ["full.py", "--ensemble-action", "restart", "--dry-run"]),
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
            patch("builtins.input") as prompt,
        ):
            FULL.main()
        stop.assert_called_once()
        start.assert_called_once_with(False)
        prompt.assert_not_called()

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
            [call.args[0] for call in launch.call_args_list], ["inference", "g8ee"]
        )
        self.assertIn("http://192.168.1.2:11434", launch.call_args_list[0].args[2])
        self.assertIn("Windows PowerShell", output.getvalue())
        self.assertIn("--model-storage-root", output.getvalue())
        self.assertNotIn("--inference-enabled", output.getvalue())

    def test_summary_uses_live_status_and_cli_management(self):
        output = io.StringIO()
        results = [
            subprocess.CompletedProcess([], 0, "  Gateway    online\n  Operators  2 connected\n", ""),
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

    def test_model_override_is_used_without_running_ollama(self):
        with (
            patch.dict(FULL.os.environ, {"OLLAMA_MODELS": "/custom/model files"}),
            patch.object(FULL.subprocess, "run") as run,
        ):
            self.assertEqual(FULL.model_root(), "/custom/model files")
            run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
