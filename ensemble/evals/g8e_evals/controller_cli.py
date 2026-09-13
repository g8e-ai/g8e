from __future__ import annotations

import json
import re
import subprocess
import sys
import time
from pathlib import Path

import click

from g8e_evals.candidate_digest import CandidateDigestError, compute_candidate_tree_digest
from g8e_evals.constants import CONTROLLER_STATE_JSON, CONTROLLER_STOP_REQUEST_JSON, METRICS_JSONL
from g8e_evals.controller import (
    ChildCommand,
    ChildResult,
    Controller,
    ControllerError,
    ControllerStatus,
    OutboxEntry,
    PublicationResult,
    ValidationResult,
    VerificationResult,
    load_controller_state,
    load_cycle_manifest,
    recover_interrupted_controller,
    request_controller_stop,
    retry_controller_publication,
)
from g8e_evals.report.validate import validate_standalone_report
from g8e_evals.schema import MetricObservation

_COMMANDS: dict[str, tuple[str, ...]] = {
    "run": ("run",),
    "campaign_run": ("campaign", "run"),
    "campaign_set_verify": ("campaign-set", "verify"),
    "bundle": ("bundle",),
    "verify": ("verify",),
}
_OPTION_NAME = re.compile(r"^[a-z][a-z0-9-]*$")


class AttendedChildHandler:
    def __init__(self, work_dir: Path, stop_request_path: Path, controller_id: str, cycle_id: str) -> None:
        self._work_dir = work_dir
        self._stop_request_path = stop_request_path
        self._controller_id = controller_id
        self._cycle_id = cycle_id

    def __call__(self, child: ChildCommand, report_dir: Path) -> ChildResult:
        command = _COMMANDS.get(child.command_name)
        if command is None:
            return ChildResult(child_id=child.child_id, ok=False, error=f"unsupported child command: {child.command_name}")
        arguments = [sys.executable, "-m", "g8e_evals.cli", *command]
        for name, value in child.args.items():
            if _OPTION_NAME.fullmatch(name) is None:
                return ChildResult(child_id=child.child_id, ok=False, error=f"invalid child option: {name}")
            arguments.extend((f"--{name}", value))
        if child.command_name in {"run", "campaign_run"} and "output-dir" not in child.args:
            arguments.extend(("--output-dir", str(report_dir)))
        process = subprocess.Popen(arguments, cwd=self._work_dir)
        while process.poll() is None:
            try:
                immediate_stop = self._immediate_stop_requested()
            except (ControllerError, ValueError, OSError) as error:
                self._terminate(process)
                return ChildResult(child_id=child.child_id, ok=False, error=f"stop request validation failed: {error}")
            if immediate_stop:
                self._terminate(process)
                return ChildResult(child_id=child.child_id, ok=False, error="immediate safety stop")
            time.sleep(0.2)
        if process.returncode != 0:
            return ChildResult(child_id=child.child_id, ok=False, error=f"child command exited with status {process.returncode}")
        try:
            provider_cost_usd = self._read_provider_cost(report_dir)
        except (ValueError, OSError) as error:
            return ChildResult(child_id=child.child_id, ok=False, error=f"provider cost accounting failed: {error}")
        return ChildResult(
            child_id=child.child_id,
            ok=True,
            report_dir=str(report_dir),
            provider_cost_usd=provider_cost_usd,
        )

    def _terminate(self, process: subprocess.Popen[bytes]) -> None:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()

    def _read_provider_cost(self, report_dir: Path) -> float:
        metrics_path = report_dir / METRICS_JSONL
        if not metrics_path.exists():
            return 0.0
        total = 0.0
        for line in metrics_path.read_text().splitlines():
            if not line.strip():
                continue
            metric = MetricObservation.model_validate(json.loads(line))
            if metric.metric_id == "provider_cost_usd" and metric.eligible and metric.value is not None:
                total += metric.value
        return total

    def _immediate_stop_requested(self) -> bool:
        if not self._stop_request_path.exists():
            return False
        from g8e_evals.controller import ControllerStopRequest

        request = ControllerStopRequest.model_validate_json(self._stop_request_path.read_text())
        if request.controller_id != self._controller_id or request.cycle_id != self._cycle_id:
            raise ControllerError("controller stop request identity mismatch")
        return request.immediate


class StandaloneAggregateVerifier:
    def __call__(self, children: list[ChildCommand], report_dirs: dict[str, Path]) -> VerificationResult:
        failures: list[str] = []
        for child in children:
            report_dir = report_dirs.get(child.child_id)
            if report_dir is None:
                failures.append(f"{child.child_id}: report directory missing")
                continue
            result = validate_standalone_report(report_dir)
            failures.extend(f"{child.child_id}: {failure}" for failure in result.failures)
        return VerificationResult(ok=not failures, failures=sorted(failures))


class CandidateTreeValidator:
    def __init__(self, report_root: Path) -> None:
        self._report_root = report_root

    def __call__(self, report_dirs: dict[str, Path]) -> ValidationResult:
        try:
            digest = compute_candidate_tree_digest(self._report_root)
        except CandidateDigestError as error:
            return ValidationResult(ok=False, digest="0" * 64, failures=[str(error)])
        return ValidationResult(ok=True, digest=digest)


class PublicOutboxPublisher:
    def __init__(self, g8e_cli: Path, work_dir: Path) -> None:
        self._g8e_cli = g8e_cli
        self._work_dir = work_dir

    def __call__(self, entry: OutboxEntry, report_dirs: dict[str, Path]) -> PublicationResult:
        result = subprocess.run(
            [str(self._g8e_cli), "public", "push"],
            cwd=self._work_dir,
            check=False,
            capture_output=True,
            text=True,
        )
        if result.returncode == 0:
            return PublicationResult(ok=True)
        return PublicationResult(ok=False, error=f"mirror publication exited with status {result.returncode}")


@click.group(name="controller")
def controller_cmd() -> None:
    """Run and inspect finite attended evaluation controllers."""


@controller_cmd.command(name="run")
@click.option("--manifest", type=click.Path(exists=True, dir_okay=False, path_type=Path), required=True)
@click.option("--work-dir", type=click.Path(exists=True, file_okay=False, path_type=Path), required=True)
@click.option("--controller-id", required=True)
@click.option("--g8e-cli", type=click.Path(exists=True, dir_okay=False, executable=True, path_type=Path), required=True)
def controller_run(manifest: Path, work_dir: Path, controller_id: str, g8e_cli: Path) -> None:
    """Run one owner-approved finite cycle manifest."""
    try:
        authority = load_cycle_manifest(manifest)
        if not authority.require_aggregate_verification or not authority.require_strict_validation or authority.approved_digest is None:
            raise ControllerError("controller run requires aggregate verification, strict validation, and an approved digest")
        report_root = work_dir / authority.report_root
        controller = Controller(
            manifest=authority,
            work_dir=work_dir,
            controller_id=controller_id,
            child_handler=AttendedChildHandler(work_dir, work_dir / CONTROLLER_STOP_REQUEST_JSON, controller_id, authority.cycle_id),
            verifier_handler=StandaloneAggregateVerifier(),
            validation_handler=CandidateTreeValidator(report_root),
            publication_handler=PublicOutboxPublisher(g8e_cli, work_dir),
        )
        state = controller.run_cycle()
    except (ControllerError, ValueError, OSError) as error:
        raise click.ClickException(str(error)) from error
    click.echo(state.model_dump_json())
    if state.status != ControllerStatus.COMPLETED:
        raise click.ClickException(f"controller stopped: {state.stop_reason}")


@controller_cmd.command(name="status")
@click.option("--work-dir", type=click.Path(exists=True, file_okay=False, path_type=Path), required=True)
def controller_status(work_dir: Path) -> None:
    """Read the durable controller state."""
    try:
        state = load_controller_state(work_dir / CONTROLLER_STATE_JSON)
    except (ValueError, OSError) as error:
        raise click.ClickException(str(error)) from error
    click.echo(state.model_dump_json())


@controller_cmd.command(name="stop")
@click.option("--work-dir", type=click.Path(exists=True, file_okay=False, path_type=Path), required=True)
@click.option("--immediate", is_flag=True, help="Terminate the active child and mark it as dead evidence.")
def controller_stop(work_dir: Path, immediate: bool) -> None:
    """Request a durable graceful or immediate stop."""
    try:
        request = request_controller_stop(work_dir, immediate=immediate)
    except (ControllerError, ValueError, OSError) as error:
        raise click.ClickException(str(error)) from error
    click.echo(request.model_dump_json())


@controller_cmd.command(name="recover")
@click.option("--work-dir", type=click.Path(exists=True, file_okay=False, path_type=Path), required=True)
@click.option("--publication", is_flag=True, help="Retry durable publication without executing a child.")
@click.option("--g8e-cli", type=click.Path(exists=True, dir_okay=False, executable=True, path_type=Path))
def controller_recover(work_dir: Path, publication: bool, g8e_cli: Path | None) -> None:
    """Recover interrupted execution or retry terminal publication."""
    try:
        if publication:
            if g8e_cli is None:
                raise ControllerError("publication recovery requires --g8e-cli")
            state = retry_controller_publication(work_dir, PublicOutboxPublisher(g8e_cli, work_dir))
        else:
            if g8e_cli is not None:
                raise ControllerError("--g8e-cli requires --publication")
            state = recover_interrupted_controller(work_dir)
    except (ControllerError, ValueError, OSError) as error:
        raise click.ClickException(str(error)) from error
    click.echo(state.model_dump_json())
    if publication and state.status != ControllerStatus.COMPLETED:
        raise click.ClickException(f"controller publication remains stopped: {state.stop_reason}")


__all__ = ["controller_cmd"]
