from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

import pytest
from click.testing import CliRunner

from g8e_evals import controller_cli
from g8e_evals.candidate_digest import compute_candidate_tree_digest
from g8e_evals.cli import main
from g8e_evals.constants import CONTROLLER_STATE_JSON, CONTROLLER_STOP_REQUEST_JSON, METRICS_JSONL
from g8e_evals.controller import (
    ChildCommand,
    ChildResult,
    Controller,
    ControllerStatus,
    PublicationResult,
    ValidationResult,
    VerificationResult,
    make_child_command,
    make_controller_state,
    make_cycle_manifest,
    request_controller_stop,
    save_controller_state,
)
from g8e_evals.controller_cli import AttendedChildHandler

pytestmark = pytest.mark.integration


def _write_active_state(work_dir: Path, *, current_child_id: str | None = None) -> None:
    save_controller_state(
        make_controller_state(
            "controller-a",
            "cycle-a",
            ControllerStatus.RUNNING,
            current_child_id=current_child_id,
        ),
        work_dir / CONTROLLER_STATE_JSON,
    )


def test_controller_cli_exposes_attended_run_status_stop_and_recover_commands() -> None:
    result = CliRunner().invoke(main, ["controller", "--help"])

    assert result.exit_code == 0
    for command in ("run", "status", "stop", "recover"):
        assert command in result.output


def test_controller_status_emits_typed_current_state(tmp_path: Path) -> None:
    _write_active_state(tmp_path)

    result = CliRunner().invoke(main, ["controller", "status", "--work-dir", str(tmp_path)])

    assert result.exit_code == 0
    payload = json.loads(result.output)
    assert payload["controller_id"] == "controller-a"
    assert payload["status"] == "running"


def test_controller_stop_writes_durable_identity_bound_safety_request(tmp_path: Path) -> None:
    _write_active_state(tmp_path)

    result = CliRunner().invoke(main, ["controller", "stop", "--work-dir", str(tmp_path), "--immediate"])

    assert result.exit_code == 0
    request = json.loads((tmp_path / CONTROLLER_STOP_REQUEST_JSON).read_text())
    assert request["controller_id"] == "controller-a"
    assert request["cycle_id"] == "cycle-a"
    assert request["immediate"] is True


def test_controller_recover_marks_current_child_interrupted(tmp_path: Path) -> None:
    _write_active_state(tmp_path, current_child_id="child-a")

    result = CliRunner().invoke(main, ["controller", "recover", "--work-dir", str(tmp_path)])

    assert result.exit_code == 0
    payload = json.loads(result.output)
    assert payload["status"] == "safety_stopped"
    assert payload["interrupted_children"] == ["child-a"]


def test_controller_recover_publication_retries_outbox_without_child_execution(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    report_root = tmp_path / "reports"
    report_root.mkdir()
    digest = compute_candidate_tree_digest(report_root)
    child = make_child_command("child-a", "campaign_run", report_dir="child-a")
    manifest = make_cycle_manifest("cycle-a", "revision-a", [child], approved_digest=digest)
    child_calls = 0

    def child_handler(child: ChildCommand, report_dir: Path) -> ChildResult:
        nonlocal child_calls
        child_calls += 1
        return ChildResult(child_id="child-a", ok=True, report_dir=str(report_dir))

    controller = Controller(
        manifest=manifest,
        work_dir=tmp_path,
        controller_id="controller-a",
        child_handler=child_handler,
        verifier_handler=lambda children, report_dirs: VerificationResult(ok=True),
        validation_handler=lambda report_dirs: ValidationResult(ok=True, digest=digest),
        publication_handler=lambda entry, report_dirs: PublicationResult(ok=False, error="mirror unavailable"),
    )
    assert controller.run_cycle().status == ControllerStatus.SAFETY_STOPPED
    original_child_calls = child_calls
    commands: list[list[str]] = []

    def run(command: list[str], **kwargs: object) -> subprocess.CompletedProcess[str]:
        commands.append(command)
        return subprocess.CompletedProcess(command, 0, "", "")

    monkeypatch.setattr(controller_cli.subprocess, "run", run)
    result = CliRunner().invoke(
        main,
        [
            "controller",
            "recover",
            "--work-dir",
            str(tmp_path),
            "--publication",
            "--g8e-cli",
            sys.executable,
        ],
    )

    assert result.exit_code == 0, result.output
    assert json.loads(result.output)["status"] == "completed"
    assert child_calls == original_child_calls == 1
    assert commands == [[sys.executable, "public", "push"]]


def test_attended_child_handler_terminates_subprocess_on_immediate_stop(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _write_active_state(tmp_path, current_child_id="child-a")
    request_controller_stop(tmp_path, immediate=True)
    terminated = False

    class RunningProcess:
        returncode = -15

        def poll(self) -> int | None:
            return None

        def terminate(self) -> None:
            nonlocal terminated
            terminated = True

        def wait(self, timeout: int | None = None) -> int:
            return self.returncode

        def kill(self) -> None:
            raise AssertionError("graceful subprocess termination should succeed")

    monkeypatch.setattr(controller_cli.subprocess, "Popen", lambda arguments, cwd: RunningProcess())
    handler = AttendedChildHandler(tmp_path, tmp_path / CONTROLLER_STOP_REQUEST_JSON, "controller-a", "cycle-a")

    result = handler(make_child_command("child-a", "verify", report_dir="child-a"), tmp_path / "reports" / "child-a")

    assert result.ok is False
    assert result.error == "immediate safety stop"
    assert terminated is True


def test_attended_child_handler_sums_only_eligible_provider_cost_metrics(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    report_dir = tmp_path / "reports" / "child-a"
    report_dir.mkdir(parents=True)
    metrics = [
        {
            "metric_id": "provider_cost_usd",
            "attempt_id": "attempt-a",
            "run_id": "run-a",
            "arm_id": "direct",
            "task_id": "task-a",
            "value": 1.25,
            "eligible": True,
        },
        {
            "metric_id": "provider_cost_usd",
            "attempt_id": "attempt-b",
            "run_id": "run-a",
            "arm_id": "direct",
            "task_id": "task-b",
            "value": 10.0,
            "eligible": False,
        },
        {
            "metric_id": "receipt_integrity",
            "attempt_id": "attempt-c",
            "run_id": "run-a",
            "arm_id": "direct",
            "task_id": "task-c",
            "value": 1.0,
            "eligible": True,
        },
    ]
    (report_dir / METRICS_JSONL).write_text("\n".join(json.dumps(metric) for metric in metrics) + "\n")

    class CompletedProcess:
        returncode = 0

        def poll(self) -> int:
            return 0

    monkeypatch.setattr(controller_cli.subprocess, "Popen", lambda arguments, cwd: CompletedProcess())
    handler = AttendedChildHandler(tmp_path, tmp_path / CONTROLLER_STOP_REQUEST_JSON, "controller-a", "cycle-a")

    result = handler(make_child_command("child-a", "verify", report_dir="child-a"), report_dir)

    assert result.ok is True
    assert result.provider_cost_usd == 1.25
