from __future__ import annotations

import hashlib
import json
from itertools import pairwise
from pathlib import Path

import pytest

from g8e_evals.candidate_digest import compute_candidate_tree_digest
from g8e_evals.constants import CONTROLLER_STOP_REQUEST_JSON, CONTROLLER_TRANSITIONS_JSONL
from g8e_evals.controller import (
    ChildCommand,
    ChildResult,
    Controller,
    ControllerError,
    ControllerStatus,
    OutboxEntry,
    PublicationResult,
    StopConditions,
    StopReason,
    ValidationResult,
    VerificationResult,
    load_controller_state,
    make_child_command,
    make_cycle_manifest,
    make_replacement_child_command,
    recover_interrupted_controller,
    request_controller_stop,
    retry_controller_publication,
)
from _set_plan_fixture import make_campaign_set_plan, make_replacement_rule

pytestmark = pytest.mark.integration


class _ChildHandler:
    def __init__(self, provider_cost_usd: float = 0.0) -> None:
        self.calls = 0
        self.provider_cost_usd = provider_cost_usd

    def __call__(self, child: ChildCommand, report_dir: Path) -> ChildResult:
        self.calls += 1
        report_dir.mkdir(parents=True, exist_ok=True)
        return ChildResult(child_id=child.child_id, ok=True, report_dir=str(report_dir), provider_cost_usd=self.provider_cost_usd)


def _controller(
    work_dir: Path,
    controller_id: str = "controller-a",
    *,
    stop_conditions: StopConditions | None = None,
    provider_cost_usd: float = 0.0,
) -> tuple[Controller, _ChildHandler]:
    child = make_child_command("child-a", "campaign_run", report_dir="child-a")
    manifest = make_cycle_manifest(
        "cycle-a",
        "revision-a",
        [child],
        approved_digest="a" * 64,
        stop_conditions=stop_conditions,
    )
    handler = _ChildHandler(provider_cost_usd)
    controller = Controller(
        manifest=manifest,
        work_dir=work_dir,
        controller_id=controller_id,
        child_handler=handler,
        verifier_handler=lambda children, report_dirs: VerificationResult(ok=True),
        validation_handler=lambda report_dirs: ValidationResult(ok=True, digest="a" * 64),
        publication_handler=lambda entry, report_dirs: PublicationResult(ok=True),
    )
    return controller, handler


def _stopped_publication_controller(work_dir: Path) -> tuple[Controller, _ChildHandler]:
    child = make_child_command("child-a", "campaign_run", report_dir="child-a")
    report_root = work_dir / "reports"
    (report_root / "child-a").mkdir(parents=True)
    digest = compute_candidate_tree_digest(report_root)
    manifest = make_cycle_manifest("cycle-a", "revision-a", [child], approved_digest=digest)
    child_handler = _ChildHandler()
    controller = Controller(
        manifest=manifest,
        work_dir=work_dir,
        controller_id="controller-a",
        child_handler=child_handler,
        verifier_handler=lambda children, report_dirs: VerificationResult(ok=True),
        validation_handler=lambda report_dirs: ValidationResult(ok=True, digest=digest),
        publication_handler=lambda entry, report_dirs: PublicationResult(ok=False, error="mirror unavailable"),
    )
    state = controller.run_cycle()
    assert state.status == ControllerStatus.SAFETY_STOPPED
    assert state.stop_reason == StopReason.MIRROR_OUTAGE
    return controller, child_handler


def test_controller_rejects_second_active_instance_and_records_hash_chained_transitions(tmp_path: Path) -> None:
    controller, _ = _controller(tmp_path)

    with pytest.raises(ControllerError, match="active controller"):
        _controller(tmp_path, controller_id="controller-b")

    state = controller.run_cycle()
    assert state.status == ControllerStatus.COMPLETED
    transitions = [json.loads(line) for line in (tmp_path / CONTROLLER_TRANSITIONS_JSONL).read_text().splitlines()]
    assert [transition["sequence"] for transition in transitions] == list(range(1, len(transitions) + 1))
    assert transitions[0]["previous_transition_hash"] == "0" * 64
    for previous, current in pairwise(transitions):
        assert current["previous_transition_hash"] == previous["content_hash"]
    assert transitions[-1]["state_hash"] == state.content_hash


def test_controller_rejects_reusing_completed_cycle_identity(tmp_path: Path) -> None:
    controller, _ = _controller(tmp_path)
    assert controller.run_cycle().status == ControllerStatus.COMPLETED

    with pytest.raises(ControllerError, match="fresh cycle identity"):
        _controller(tmp_path, controller_id="controller-b")


def test_controller_authority_rejects_unsafe_runtime_paths() -> None:
    with pytest.raises(ValueError, match="safe relative path"):
        make_child_command("child-a", "campaign_run", report_dir="../child-a")
    child = make_child_command("child-a", "campaign_run", report_dir="child-a")
    with pytest.raises(ValueError, match="safe relative path"):
        make_cycle_manifest("cycle-a", "revision-a", [child], report_root="/tmp/reports")
    with pytest.raises(ValueError, match="safe relative path"):
        make_cycle_manifest("cycle-a", "revision-a", [child], outbox_dir="../outbox")


def test_controller_stops_when_observed_provider_cost_exceeds_cycle_budget(tmp_path: Path) -> None:
    controller, handler = _controller(
        tmp_path,
        stop_conditions=StopConditions(max_budget_usd=0.5),
        provider_cost_usd=1.0,
    )

    state = controller.run_cycle()

    assert state.status == ControllerStatus.SAFETY_STOPPED
    assert state.stop_reason == StopReason.BUDGET_EXHAUSTED
    assert state.budget_spent_usd == 1.0
    assert handler.calls == 1


def test_controller_enforces_minimum_free_disk_reserve_before_child(tmp_path: Path) -> None:
    controller, handler = _controller(
        tmp_path,
        stop_conditions=StopConditions(min_free_disk_bytes=10**30),
    )

    state = controller.run_cycle()

    assert state.status == ControllerStatus.SAFETY_STOPPED
    assert state.stop_reason == StopReason.DISK_FULL
    assert handler.calls == 0


def test_external_graceful_stop_is_durable_and_prevents_next_child(tmp_path: Path) -> None:
    controller, handler = _controller(tmp_path)
    request_controller_stop(tmp_path, immediate=False)

    state = controller.run_cycle()

    assert state.status == ControllerStatus.GRACEFUL_STOPPING
    assert state.stop_reason == StopReason.GRACEFUL_STOP
    assert handler.calls == 0


def test_recovery_marks_inflight_child_as_dead_evidence_without_rerunning_it(tmp_path: Path) -> None:
    controller, handler = _controller(tmp_path)
    controller._transition(ControllerStatus.RUNNING, current_child_id="child-a")
    report_dir = tmp_path / "reports" / "child-a"
    report_dir.mkdir(parents=True)
    marker = report_dir / "partial.marker"
    marker.write_text("dead evidence")

    state = recover_interrupted_controller(tmp_path)

    assert state.status == ControllerStatus.SAFETY_STOPPED
    assert state.stop_reason == StopReason.SAFETY_STOP
    assert state.interrupted_children == ["child-a"]
    assert marker.read_text() == "dead evidence"
    assert handler.calls == 0
    assert load_controller_state(tmp_path / "controller-state.json") == state


def test_publication_recovery_validates_durable_state_and_never_reruns_child(tmp_path: Path) -> None:
    controller, child_handler = _stopped_publication_controller(tmp_path)
    publication_calls = 0
    child_calls = child_handler.calls

    def recovered_publication(entry: OutboxEntry, report_dirs: dict[str, Path]) -> PublicationResult:
        nonlocal publication_calls
        publication_calls += 1
        return PublicationResult(ok=True)

    recovered = retry_controller_publication(tmp_path, recovered_publication)

    assert recovered.status == ControllerStatus.COMPLETED
    assert recovered.stop_reason == StopReason.COMPLETED
    assert child_handler.calls == child_calls == 1
    assert publication_calls == 1
    entries = controller.outbox.all_entries()
    assert len(entries) == 1
    assert entries[0].published is True
    assert entries[0].publication_attempts == 2
    transitions = [json.loads(line) for line in (tmp_path / CONTROLLER_TRANSITIONS_JSONL).read_text().splitlines()]
    assert transitions[-1]["state_hash"] == recovered.content_hash


def test_publication_recovery_rejects_candidate_changed_after_validation(tmp_path: Path) -> None:
    _, child_handler = _stopped_publication_controller(tmp_path)
    (tmp_path / "reports" / "changed.json").write_text("changed")

    with pytest.raises(ControllerError, match="candidate digest does not match"):
        retry_controller_publication(tmp_path, lambda entry, report_dirs: PublicationResult(ok=True))

    assert child_handler.calls == 1


def test_publication_recovery_rejects_corrupt_transition_chain(tmp_path: Path) -> None:
    _, child_handler = _stopped_publication_controller(tmp_path)
    transitions_path = tmp_path / CONTROLLER_TRANSITIONS_JSONL
    transitions = transitions_path.read_text().splitlines()
    last = json.loads(transitions[-1])
    last["state_hash"] = "f" * 64
    transitions[-1] = json.dumps(last)
    transitions_path.write_text("\n".join(transitions) + "\n")

    with pytest.raises(ValueError, match="transition content_hash mismatch"):
        retry_controller_publication(tmp_path, lambda entry, report_dirs: PublicationResult(ok=True))

    assert child_handler.calls == 1


def test_controller_rejects_corrupt_stop_request_before_child_execution(tmp_path: Path) -> None:
    controller, handler = _controller(tmp_path)
    (tmp_path / CONTROLLER_STOP_REQUEST_JSON).write_text("not-json")

    with pytest.raises(ValueError, match="Invalid JSON"):
        controller.run_cycle()

    assert handler.calls == 0


def test_controller_rejects_stop_request_for_different_identity_before_child_execution(tmp_path: Path) -> None:
    controller, handler = _controller(tmp_path)
    request = {
        "controller_id": "controller-b",
        "cycle_id": "cycle-a",
        "immediate": True,
        "requested_at": "2026-09-13T00:00:00Z",
    }
    canonical = json.dumps(request, allow_nan=False, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    request["content_hash"] = hashlib.sha256(canonical.encode()).hexdigest()
    (tmp_path / CONTROLLER_STOP_REQUEST_JSON).write_text(json.dumps(request))

    with pytest.raises(ControllerError, match="identity mismatch"):
        controller.run_cycle()

    assert handler.calls == 0


def test_replacement_command_identity_is_derived_from_frozen_rule() -> None:
    plan = make_campaign_set_plan()
    rule = make_replacement_rule(plan=plan)
    original = make_child_command(
        plan.child_plans[0].child_id,
        "campaign_run",
        args={"campaign-set-plan": "authorities/p12-plan.json"},
        report_dir="cycle-a/child-a",
    )

    replacement = make_replacement_child_command(
        original,
        rule,
        1,
        "authorities/replacement-rule.json",
        "cycle-b/child-a-replacement",
    )

    assert replacement.child_id != original.child_id
    assert replacement.args["campaign-id"] == replacement.child_id
    assert replacement.args["replacement-rule"] == "authorities/replacement-rule.json"
    assert replacement.report_dir != original.report_dir


def test_replacement_cycle_rejects_reusing_prior_report_root(tmp_path: Path) -> None:
    controller, _ = _controller(tmp_path)
    assert controller.run_cycle().status == ControllerStatus.COMPLETED
    replacement = make_cycle_manifest(
        "cycle-b",
        "revision-b",
        [make_child_command("child-b", "campaign_run", report_dir="child-b")],
        report_root="reports",
        outbox_dir="outbox-b",
    )

    with pytest.raises(ControllerError, match="fresh report root"):
        Controller(manifest=replacement, work_dir=tmp_path, controller_id="controller-b")


def test_two_finite_cycles_preserve_replacement_and_retry_publication_without_rerunning_child(tmp_path: Path) -> None:
    plan = make_campaign_set_plan()
    rule = make_replacement_rule(plan=plan)
    original = make_child_command(
        plan.child_plans[0].child_id,
        "campaign_run",
        args={"campaign-set-plan": "authorities/p12-plan.json"},
        report_dir="original-child",
    )
    first_report_root = tmp_path / "reports-cycle-a"
    first_report_root.mkdir()
    first_digest = compute_candidate_tree_digest(first_report_root)
    first_manifest = make_cycle_manifest(
        "cycle-a",
        "revision-a",
        [original],
        approved_digest=first_digest,
        report_root="reports-cycle-a",
        outbox_dir="outbox-cycle-a",
    )
    first_handler = _ChildHandler()
    first = Controller(
        manifest=first_manifest,
        work_dir=tmp_path,
        controller_id="controller-a",
        child_handler=first_handler,
        verifier_handler=lambda children, report_dirs: VerificationResult(ok=True),
        validation_handler=lambda report_dirs: ValidationResult(ok=True, digest=first_digest),
        publication_handler=lambda entry, report_dirs: PublicationResult(ok=True),
    )

    first_state = first.run_cycle()

    assert first_state.status == ControllerStatus.COMPLETED
    assert first_handler.calls == 1
    replacement = make_replacement_child_command(
        original,
        rule,
        1,
        "authorities/replacement-rule.json",
        "replacement-child",
    )
    second_report_root = tmp_path / "reports-cycle-b"
    second_report_root.mkdir()
    second_digest = compute_candidate_tree_digest(second_report_root)
    second_manifest = make_cycle_manifest(
        "cycle-b",
        "revision-b",
        [replacement],
        approved_digest=second_digest,
        report_root="reports-cycle-b",
        outbox_dir="outbox-cycle-b",
    )
    second_handler = _ChildHandler()
    second = Controller(
        manifest=second_manifest,
        work_dir=tmp_path,
        controller_id="controller-b",
        child_handler=second_handler,
        verifier_handler=lambda children, report_dirs: VerificationResult(ok=True),
        validation_handler=lambda report_dirs: ValidationResult(ok=True, digest=second_digest),
        publication_handler=lambda entry, report_dirs: PublicationResult(ok=False, error="mirror unavailable"),
    )

    stopped = second.run_cycle()

    assert stopped.status == ControllerStatus.SAFETY_STOPPED
    assert stopped.stop_reason == StopReason.MIRROR_OUTAGE
    assert second_handler.calls == 1
    second_child_calls = second_handler.calls
    publication_order: list[str] = []

    def publish(entry: OutboxEntry, report_dirs: dict[str, Path]) -> PublicationResult:
        publication_order.append(entry.entry_id)
        return PublicationResult(ok=True)

    recovered = retry_controller_publication(tmp_path, publish)

    assert recovered.status == ControllerStatus.COMPLETED
    assert second_handler.calls == second_child_calls == 1
    assert publication_order == ["entry-cycle-b"]
    assert replacement.child_id != original.child_id
    assert first_report_root.is_dir()
    assert second_report_root.is_dir()
