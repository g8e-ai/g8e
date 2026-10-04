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

    def test_model_override_is_used_without_running_ollama(self):
        with (
            patch.dict(FULL.os.environ, {"OLLAMA_MODELS": "/custom/model files"}),
            patch.object(FULL.subprocess, "run") as run,
        ):
            self.assertEqual(FULL.model_root(), "/custom/model files")
            run.assert_not_called()


if __name__ == "__main__":
    unittest.main()
