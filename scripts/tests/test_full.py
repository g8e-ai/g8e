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
    @staticmethod
    def terminate_process(process):
        if process.poll() is None:
            process.kill()
        process.wait(timeout=5)

    def test_missing_dotenv_points_to_runtime_setup(self):
        with (
            patch.dict(sys.modules, {"dotenv": None}),
            self.assertRaisesRegex(RuntimeError, "make ensemble-env"),
        ):
            FULL.load_environment(Path("missing.env"))

    def test_missing_ensemble_dependencies_fail_before_gateway_start(self):
        with (
            patch.object(sys, "argv", ["full.py", "--start-gateway"]),
            patch.object(FULL, "model_root", return_value="/models"),
            patch.object(
                FULL, "load_environment",
                return_value={"OllamaEndpoint": "http://localhost:11434"},
            ),
            patch.object(
                FULL.subprocess, "run",
                side_effect=subprocess.CalledProcessError(1, [sys.executable]),
            ) as run,
            self.assertRaisesRegex(RuntimeError, "make ensemble-env"),
        ):
            FULL.main()
        self.assertEqual(run.call_count, 1)
        self.assertEqual(run.call_args.args[0], [sys.executable, "-c", "import app.serve"])

    def test_dotenv_reads_only_registered_launcher_keys_without_execution(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / ".env"
            path.write_text(
                "export G8E_HOSTNAME='dev.example' # browser host\n"
                'G8E_OLLAMA_ENDPOINT="http://provider:11434"\n'
                "G8E_DATA_HOST=remote.example\n"
                "G8E_HTTP_PORT=9999\n"
                "UNRELATED_SECRET=keep-private\n"
            )
            with patch.dict(FULL.os.environ, {}, clear=True):
                values = FULL.load_environment(path)
            self.assertEqual(
                values,
                {
                    "Hostname": "dev.example",
                    "OllamaEndpoint": "http://provider:11434",
                    "DataHost": "remote.example",
                },
            )

    def test_exported_endpoint_wins_even_when_empty(self):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / ".env"
            path.write_text("G8E_OLLAMA_ENDPOINT=http://file:11434\n")
            with patch.dict(FULL.os.environ, {"G8E_OLLAMA_ENDPOINT": ""}, clear=True):
                self.assertEqual(FULL.load_environment(path)["OllamaEndpoint"], "")

    def test_unattended_missing_endpoint_fails_before_starting_gateway(self):
        with (
            patch.object(sys, "argv", ["full.py", "--start-gateway"]),
            patch.object(FULL, "load_environment", return_value={}),
            patch.object(FULL.subprocess, "run") as run,
            patch("builtins.input") as prompt,
            self.assertRaisesRegex(ValueError, "G8E_OLLAMA_ENDPOINT"),
        ):
            FULL.main()
        prompt.assert_not_called()
        run.assert_not_called()

    def test_unattended_preview_uses_environment_and_never_prompts(self):
        output = io.StringIO()
        with (
            patch.object(sys, "argv", ["full.py", "--dry-run", "--start-gateway"]),
            patch.object(
                FULL,
                "load_environment",
                return_value={
                    "Hostname": "dev.example",
                    "OllamaEndpoint": "http://gpu:11434",
                    "ObserverHost": "remote.example",
                },
            ),
            patch.object(FULL, "model_root", return_value="/models"),
            patch("builtins.input", side_effect=AssertionError("unexpected prompt")),
            patch.object(FULL.subprocess, "run") as run,
            contextlib.redirect_stdout(output),
        ):
            FULL.main()
        run.assert_not_called()
        text = output.getvalue()
        self.assertIn("http://gpu:11434", text)
        self.assertIn("https://dev.example:8443", text)
        self.assertIn("Run on remote.example", text)
        self.assertIn("--public-base-url", text)

    def test_model_root_prefers_populated_wsl_store_over_empty_snap_directory(self):
        store = Path("/mnt/d/ai/Ollama/models")
        directories = {
            Path("/var/snap/ollama/common/models"),
            store,
            store / "manifests",
            store / "blobs",
        }
        with (
            patch.dict(FULL.os.environ, {}, clear=True),
            patch.object(Path, "exists", return_value=False),
            patch.object(
                Path,
                "rglob",
                autospec=True,
                side_effect=lambda path, pattern: (
                    [store / "manifests/model"] if path == store / "manifests" else []
                ),
            ),
            patch.object(Path, "is_file", return_value=True),
            patch.object(
                Path,
                "glob",
                side_effect=lambda pattern: [Path("/mnt/d")] if pattern == "*" else [],
            ),
            patch.object(
                Path,
                "is_dir",
                autospec=True,
                side_effect=lambda path: path in directories,
            ),
        ):
            self.assertEqual(FULL.model_root(), str(store))

    def test_model_root_skips_empty_snap_configured_store(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            empty = root / "snap-models"
            populated = root / "d/ai/Ollama/models"
            for directory in (empty, populated):
                (directory / "manifests").mkdir(parents=True)
                (directory / "blobs").mkdir()
            (populated / "manifests/model").write_text("manifest")
            with (
                patch.dict(FULL.os.environ, {}, clear=True),
                patch.object(Path, "exists", return_value=True),
                patch.object(
                    FULL.subprocess,
                    "run",
                    return_value=subprocess.CompletedProcess([], 0, str(empty)),
                ),
                patch.object(
                    Path,
                    "glob",
                    side_effect=lambda pattern: [root / "d"] if pattern == "*" else [],
                ),
            ):
                self.assertEqual(FULL.model_root(), str(populated))

    def test_unattended_directory_flags_override_defaults(self):
        with (
            patch.object(
                sys,
                "argv",
                [
                    "full.py",
                    "--dry-run",
                    "--provenance-working-dir",
                    "/operator with spaces",
                    "--model-storage-root",
                    "/models with spaces",
                ],
            ),
            patch.object(
                FULL,
                "load_environment",
                return_value={"OllamaEndpoint": "http://gpu:11434"},
            ),
            patch("builtins.input", side_effect=AssertionError("unexpected prompt")),
            patch.object(FULL, "start_local") as launch,
            patch.object(FULL, "print_summary"),
        ):
            FULL.main()
        provenance = launch.call_args_list[0]
        self.assertEqual(
            provenance.args[1], str(Path("/operator with spaces").resolve())
        )
        self.assertIn(str(Path("/models with spaces").resolve()), provenance.args[2])

    def test_unattended_stale_identity_fails_without_prompt_or_mutation(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            directory, saved, _ = self.setup_identities(root)
            before = saved.read_bytes()
            with (
                patch.object(FULL, "ROOT", root),
                patch.object(FULL.subprocess, "run") as run,
                patch("builtins.input") as prompt,
                self.assertRaisesRegex(RuntimeError, "RESET_IDENTITIES=1"),
            ):
                FULL.prepare_identities(
                    [("observer", "localhost", str(directory))], interactive=False
                )
            run.assert_not_called()
            prompt.assert_not_called()
            self.assertEqual(saved.read_bytes(), before)

    def test_unattended_invalid_endpoints_fail_before_side_effects(self):
        for values in (
            {"OllamaEndpoint": "file:///private"},
            {"OllamaEndpoint": "http://gpu:invalid"},
            {
                "OllamaEndpoint": "http://gpu:11434",
                "Hostname": "$(touch /tmp/sentinel)",
            },
            {"OllamaEndpoint": "http://gpu:11434", "DataHost": ""},
        ):
            with (
                self.subTest(values=values),
                patch.object(sys, "argv", ["full.py", "--start-gateway"]),
                patch.object(FULL, "load_environment", return_value=values),
                patch.object(FULL.subprocess, "run") as run,
                self.assertRaises(ValueError),
            ):
                FULL.main()
            run.assert_not_called()

    def test_unattended_start_passes_explicit_gateway_flags_and_disables_prompts(self):
        with (
            patch.object(sys, "argv", ["full.py", "--start-gateway"]),
            patch.object(
                FULL,
                "load_environment",
                return_value={
                    "Hostname": "dev.example",
                    "OllamaEndpoint": "http://gpu:11434",
                },
            ),
            patch.object(FULL.subprocess, "run") as run,
            patch.object(FULL, "prepare_identities") as identities,
            patch.object(FULL, "start_local"),
            patch.object(FULL, "report_workloads", return_value=False),
            patch.object(FULL, "print_summary"),
            patch("builtins.input", side_effect=AssertionError("unexpected prompt")),
        ):
            FULL.main()
        gateway = run.call_args_list[-1].args[0]
        self.assertEqual(gateway[1:4], ["gw", "start", "--quiet"])
        self.assertIn("https://dev.example:8443", gateway)
        self.assertFalse(identities.call_args.kwargs["interactive"])

    def test_ensemble_start_uses_this_python_and_shared_runtime(self):
        with patch.object(FULL, "start_local") as launch:
            FULL.start_ensemble()
        call = launch.call_args
        self.assertEqual(call.args[1], FULL.ROOT / ".local.dev/full/ensemble")
        self.assertEqual(call.args[2][:3], [sys.executable, "-m", "app.serve"])
        self.assertIn("https://g8e.local:8443", call.args[2])
        self.assertIn("wss://g8e.local:8443", call.args[2])

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
                [
                    sys.executable,
                    "-c",
                    "import time; print('ready', flush=True); time.sleep(30)",
                    "app.serve",
                ],
                stdout=subprocess.PIPE,
                text=True,
            )
            self.addCleanup(self.terminate_process, process)
            self.addCleanup(process.stdout.close)
            # Wait for exec/startup before checking the command line in stop_ensemble.
            self.assertEqual(process.stdout.readline().strip(), "ready")
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
            patch.object(
                FULL, "load_environment", return_value={"Hostname": "dev.example"}
            ),
            patch.object(FULL, "stop_ensemble") as stop,
            patch.object(FULL, "start_ensemble") as start,
        ):
            FULL.main()
        stop.assert_not_called()
        start.assert_called_once_with(True, "dev.example")

    def test_ensemble_action_skips_operator_prompts(self):
        with (
            patch.object(sys, "argv", ["full.py", "--ensemble-action", "restart"]),
            patch.object(
                FULL, "load_environment", return_value={"Hostname": "dev.example"}
            ),
            patch.object(FULL, "stop_ensemble") as stop,
            patch.object(FULL, "start_ensemble") as start,
            patch.object(FULL, "prepare_identities") as prepare,
            patch.object(FULL, "report_workloads", return_value=False),
            patch("builtins.input") as prompt,
        ):
            FULL.main()
        stop.assert_called_once()
        start.assert_called_once_with(False, "dev.example")
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
            try:
                saved.symlink_to(root / FULL.TRUST_PATH)
            except OSError as exc:
                self.skipTest(f"symlinks are unavailable: {exc}")
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
                self.assertEqual(
                    FULL.workload_state("observer", directory)[0],
                    "awaiting approval",
                )
                log.write_text(
                    old + "operator enrollment: gateway not yet bootstrapped\n"
                )
                self.assertEqual(
                    FULL.workload_state("observer", directory)[0],
                    "awaiting enrollment",
                )
                log.write_text(log.read_text() + old)
                self.assertEqual(
                    FULL.workload_state("observer", directory)[0], "connected"
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
            self.addCleanup(self.terminate_process, process)
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
            "localhost",
            "/tmp/data",
        ]
        with (
            patch.object(sys, "argv", ["full.py", "--setup", "--dry-run"]),
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
            patch.object(sys, "argv", ["full.py", "--setup", "--dry-run"]),
            patch("builtins.input", side_effect=answers),
            patch.object(FULL, "start_local") as launch,
            contextlib.redirect_stdout(output),
        ):
            FULL.main()
        self.assertEqual(
            [call.args[0] for call in launch.call_args_list],
            ["inference", "data", "g8ee"],
        )
        self.assertIn("http://192.168.1.2:11434", launch.call_args_list[0].args[2])
        self.assertEqual(
            launch.call_args_list[0].args[2][3:5], ["--endpoint", "g8e.local"]
        )
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
        prov = FULL.operator_args(
            "provenance", "localhost:8443", "/dir/prov", "/dir/models", ""
        )
        self.assertIn("--provenance-operator-enabled", prov)
        self.assertIn("--model-storage-root", prov)

        infer = FULL.operator_args(
            "inference", "localhost:8443", "/dir/infer", "", "http://localhost:11434"
        )
        self.assertIn("--inference-enabled", infer)
        self.assertIn("--inference-ollama-endpoint", infer)

        obs = FULL.operator_args("observer", "localhost:8443", "/dir/obs", "", "")
        self.assertIn("--provider-boundary-observer-enabled", obs)

        data = FULL.operator_args("data", "localhost:8443", "/dir/data", "", "")
        self.assertNotIn("--provenance-operator-enabled", data)
        self.assertNotIn("--inference-enabled", data)
        self.assertNotIn("--provider-boundary-observer-enabled", data)
        self.assertEqual(
            data,
            [
                "operator",
                "start",
                "--endpoint",
                "localhost:8443",
                "--working-dir",
                "/dir/data",
            ],
        )

    def test_down_stops_ensemble_operators_and_gateway(self):
        with (
            patch.object(sys, "argv", ["full.py", "--down"]),
            patch.object(FULL, "stop_ensemble") as stop_ens,
            patch.object(FULL, "stop_operators") as stop_ops,
            patch.object(FULL, "stop_gateway") as stop_gw,
        ):
            FULL.main()
        stop_ens.assert_called_once_with(False)
        stop_ops.assert_called_once_with(False)
        stop_gw.assert_called_once_with(False)

    def test_down_dry_run_does_not_stop_processes(self):
        with (
            patch.object(sys, "argv", ["full.py", "--down", "--dry-run"]),
            patch.object(FULL, "stop_ensemble") as stop_ens,
            patch.object(FULL, "stop_operators") as stop_ops,
            patch.object(FULL, "stop_gateway") as stop_gw,
        ):
            FULL.main()
        stop_ens.assert_called_once_with(True)
        stop_ops.assert_called_once_with(True)
        stop_gw.assert_called_once_with(True)

    def test_status_shows_platform_and_workloads(self):
        output = io.StringIO()
        with (
            patch.object(sys, "argv", ["full.py", "--status"]),
            patch.object(FULL, "check_gateway_health", return_value=True),
            patch.object(FULL, "check_g8ee_health", return_value=True),
            patch.object(
                FULL,
                "get_recorded_workloads",
                return_value=[
                    ("observer", Path("/fake/observer")),
                    ("g8ee", Path("/fake/ensemble")),
                ],
            ),
            patch.object(FULL, "ensemble_running", return_value=False),
            contextlib.redirect_stdout(output),
        ):
            FULL.main()
        text = output.getvalue()
        self.assertIn("Platform status", text)
        self.assertIn("Local workloads", text)
        self.assertIn("g8ee", text)
        self.assertIn("observer", text)

    def test_operator_action_stop(self):
        with (
            patch.object(sys, "argv", ["full.py", "--operator-action", "stop"]),
            patch.object(FULL, "stop_operators") as stop_ops,
        ):
            FULL.main()
        stop_ops.assert_called_once_with(False)

    def test_operator_action_status(self):
        output = io.StringIO()
        with (
            patch.object(sys, "argv", ["full.py", "--operator-action", "status"]),
            patch.object(
                FULL,
                "get_recorded_workloads",
                return_value=[
                    ("observer", Path("/fake/observer")),
                    ("g8ee", Path("/fake/ensemble")),
                ],
            ),
            contextlib.redirect_stdout(output),
        ):
            FULL.main()
        text = output.getvalue()
        self.assertIn("observer", text)
        self.assertNotIn("g8ee", text)

    def test_ensemble_action_status(self):
        output = io.StringIO()
        with (
            patch.object(sys, "argv", ["full.py", "--ensemble-action", "status"]),
            contextlib.redirect_stdout(output),
        ):
            FULL.main()
        text = output.getvalue()
        self.assertIn("g8ee", text)

    def test_stop_operator_cleans_stale_pid_without_signalling(self):
        with tempfile.TemporaryDirectory() as temp:
            directory = Path(temp)
            pid_file = directory / "full.pid"
            pid_file.write_text("99999999")
            with (
                patch.object(FULL, "ensemble_running", return_value=False),
                patch.object(FULL.os, "kill") as kill,
            ):
                FULL.stop_operator("data", directory)
            kill.assert_not_called()
            self.assertFalse(pid_file.exists())

    def test_stop_operator_terminates_process(self):
        with tempfile.TemporaryDirectory() as temp:
            directory = Path(temp)
            # Linux checks the command line before stopping an owned workload.
            script = directory / "g8e-operator-fixture.py"
            script.write_text(
                "import time; print('ready', flush=True); time.sleep(30)\n"
            )
            process = subprocess.Popen(
                [sys.executable, str(script)],
                stdout=subprocess.PIPE,
                text=True,
            )
            self.addCleanup(self.terminate_process, process)
            self.addCleanup(process.stdout.close)
            self.assertEqual(process.stdout.readline().strip(), "ready")
            pid_file = directory / "full.pid"
            pid_file.write_text(str(process.pid))
            FULL.stop_operator("data", directory)
            self.assertIsNotNone(process.poll())
            self.assertFalse(pid_file.exists())


if __name__ == "__main__":
    unittest.main()
