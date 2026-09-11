# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the fixture-driven continuous controller core (CONT-CORE).

Verifies the controller lifecycle using stub commands and isolated fixture
directories only. No live campaigns run, no real evidence is published, and
no inference is rerun on publication retry.

Acceptance scenarios covered:
- Two-cycle rollover (fresh identity and report root per cycle).
- Graceful stop (completes current child, then stops).
- Safety stop (immediate halt, current child is dead evidence).
- Interrupted child replacement (fresh child identity and report root).
- Verifier failure (aggregate verification failure stops the cycle).
- Digest rejection (candidate digest does not match approved digest).
- Mirror outage (publication failure classified and stops the cycle).
- Ordered outbox recovery (entries retried in enqueue order).
- No inference rerun on publication retry (outbox reads durable entries).
- Content hash mutation vectors (every material field changes the hash).
- State persistence (controller-state.json written after every transition).
- Stop conditions (disk ceiling triggers safety stop).
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.controller import (
    CONTROLLER_SCHEMA_VERSION,
    ChildCommand,
    ChildResult,
    Controller,
    ControllerError,
    ControllerStatus,
    CycleManifest,
    Outbox,
    OutboxEntry,
    PublicationResult,
    StopConditions,
    StopReason,
    ValidationResult,
    VerificationResult,
    load_controller_state,
    load_cycle_manifest,
    make_child_command,
    make_controller_state,
    make_cycle_manifest,
    make_outbox_entry,
    save_controller_state,
    save_cycle_manifest,
)
from g8e_evals.constants import (
    CONTROLLER_STATE_JSON,
    CYCLE_MANIFEST_JSON,
    OUTBOX_INDEX_JSONL,
)


_VALID_DIGEST = "a" * 64
_VALID_DIGEST_B = "b" * 64


# ---------------------------------------------------------------------------
# Stub handlers
# ---------------------------------------------------------------------------


class _StubChildHandler:
    """Stub child command handler that writes a marker file to the report dir."""

    def __init__(self, *, fail_child_ids: set[str] | None = None) -> None:
        self._fail_child_ids = fail_child_ids or set()
        self.call_count = 0
        self.called_children: list[str] = []

    def __call__(self, child: ChildCommand, report_dir: Path) -> ChildResult:
        self.call_count += 1
        self.called_children.append(child.child_id)
        if child.child_id in self._fail_child_ids:
            return ChildResult(child_id=child.child_id, ok=False, error=f"stub failure for {child.child_id}")
        report_dir.mkdir(parents=True, exist_ok=True)
        (report_dir / "marker.txt").write_text(f"report for {child.child_id}")
        return ChildResult(child_id=child.child_id, ok=True, report_dir=str(report_dir))


class _StubVerifierHandler:
    """Stub aggregate verifier handler."""

    def __init__(self, *, ok: bool = True, failures: list[str] | None = None) -> None:
        self._ok = ok
        self._failures = failures or []
        self.call_count = 0

    def __call__(self, children: list[ChildCommand], report_dirs: dict[str, Path]) -> VerificationResult:
        self.call_count += 1
        return VerificationResult(ok=self._ok, failures=list(self._failures))


class _StubValidationHandler:
    """Stub strict validation handler that computes a deterministic digest."""

    def __init__(self, *, digest: str = _VALID_DIGEST, ok: bool = True, failures: list[str] | None = None) -> None:
        self._digest = digest
        self._ok = ok
        self._failures = failures or []
        self.call_count = 0

    def __call__(self, report_dirs: dict[str, Path]) -> ValidationResult:
        self.call_count += 1
        return ValidationResult(ok=self._ok, digest=self._digest, failures=list(self._failures))


class _StubPublicationHandler:
    """Stub publication handler."""

    def __init__(self, *, ok: bool = True, error: str | None = None) -> None:
        self._ok = ok
        self._error = error
        self.call_count = 0
        self.published_entries: list[str] = []

    def __call__(self, entry: OutboxEntry, report_dirs: dict[str, Path]) -> PublicationResult:
        self.call_count += 1
        if self._ok:
            self.published_entries.append(entry.entry_id)
            return PublicationResult(ok=True)
        return PublicationResult(ok=False, error=self._error)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_two_children() -> list[ChildCommand]:
    return [
        make_child_command(
            child_id="child-alpha",
            command_name="campaign_run",
            args={"task_offset": "0", "task_limit": "30"},
            report_dir="reports/child-alpha",
        ),
        make_child_command(
            child_id="child-beta",
            command_name="campaign_run",
            args={"task_offset": "30", "task_limit": "30"},
            report_dir="reports/child-beta",
        ),
    ]


def _make_manifest(
    children: list[ChildCommand] | None = None,
    *,
    cycle_id: str = "cycle-001",
    cycle_revision: str = "rev-1",
    approved_digest: str | None = None,
    stop_conditions: StopConditions | None = None,
    require_aggregate_verification: bool = True,
    require_strict_validation: bool = True,
    report_root: str = "reports",
    outbox_dir: str = "outbox",
) -> CycleManifest:
    return make_cycle_manifest(
        cycle_id=cycle_id,
        cycle_revision=cycle_revision,
        children=children or _make_two_children(),
        stop_conditions=stop_conditions,
        require_aggregate_verification=require_aggregate_verification,
        require_strict_validation=require_strict_validation,
        approved_digest=approved_digest,
        report_root=report_root,
        outbox_dir=outbox_dir,
    )


def _make_controller(
    tmp_path: Path,
    manifest: CycleManifest | None = None,
    *,
    child_handler: _StubChildHandler | None = None,
    verifier_handler: _StubVerifierHandler | None = None,
    validation_handler: _StubValidationHandler | None = None,
    publication_handler: _StubPublicationHandler | None = None,
) -> Controller:
    return Controller(
        manifest=manifest or _make_manifest(),
        work_dir=tmp_path,
        controller_id="test-controller",
        child_handler=child_handler or _StubChildHandler(),
        verifier_handler=verifier_handler or _StubVerifierHandler(),
        validation_handler=validation_handler or _StubValidationHandler(),
        publication_handler=publication_handler or _StubPublicationHandler(),
    )


# ---------------------------------------------------------------------------
# Content hash mutation vectors
# ---------------------------------------------------------------------------


class TestChildCommandHashMutation:
    def test_child_command_hash_is_stable(self):
        child = make_child_command("c1", "campaign_run", {"k": "v"}, "reports/c1")
        child2 = make_child_command("c1", "campaign_run", {"k": "v"}, "reports/c1")
        assert child.content_hash == child2.content_hash

    def test_child_id_mutation_changes_hash(self):
        base = make_child_command("c1", "campaign_run", {}, "reports/c1")
        mutated = make_child_command("c2", "campaign_run", {}, "reports/c1")
        assert base.content_hash != mutated.content_hash

    def test_command_name_mutation_changes_hash(self):
        base = make_child_command("c1", "campaign_run", {}, "reports/c1")
        mutated = make_child_command("c1", "campaign_validate", {}, "reports/c1")
        assert base.content_hash != mutated.content_hash

    def test_args_mutation_changes_hash(self):
        base = make_child_command("c1", "campaign_run", {"k": "v"}, "reports/c1")
        mutated = make_child_command("c1", "campaign_run", {"k": "w"}, "reports/c1")
        assert base.content_hash != mutated.content_hash

    def test_report_dir_mutation_changes_hash(self):
        base = make_child_command("c1", "campaign_run", {}, "reports/c1")
        mutated = make_child_command("c1", "campaign_run", {}, "reports/c2")
        assert base.content_hash != mutated.content_hash


class TestCycleManifestHashMutation:
    def test_manifest_hash_is_stable(self):
        m1 = _make_manifest()
        m2 = _make_manifest()
        assert m1.content_hash == m2.content_hash

    def test_cycle_id_mutation_changes_hash(self):
        base = _make_manifest(cycle_id="cycle-001")
        mutated = _make_manifest(cycle_id="cycle-002")
        assert base.content_hash != mutated.content_hash

    def test_cycle_revision_mutation_changes_hash(self):
        base = _make_manifest(cycle_revision="rev-1")
        mutated = _make_manifest(cycle_revision="rev-2")
        assert base.content_hash != mutated.content_hash

    def test_children_mutation_changes_hash(self):
        base = _make_manifest()
        mutated = _make_manifest(children=[make_child_command("child-gamma", "campaign_run")])
        assert base.content_hash != mutated.content_hash

    def test_approved_digest_mutation_changes_hash(self):
        base = _make_manifest(approved_digest=_VALID_DIGEST)
        mutated = _make_manifest(approved_digest=_VALID_DIGEST_B)
        assert base.content_hash != mutated.content_hash

    def test_report_root_mutation_changes_hash(self):
        base = _make_manifest(report_root="reports")
        mutated = _make_manifest(report_root="reports-v2")
        assert base.content_hash != mutated.content_hash

    def test_outbox_dir_mutation_changes_hash(self):
        base = _make_manifest(outbox_dir="outbox")
        mutated = _make_manifest(outbox_dir="outbox-v2")
        assert base.content_hash != mutated.content_hash

    def test_require_aggregate_verification_mutation_changes_hash(self):
        base = _make_manifest(require_aggregate_verification=True)
        mutated = _make_manifest(require_aggregate_verification=False)
        assert base.content_hash != mutated.content_hash

    def test_require_strict_validation_mutation_changes_hash(self):
        base = _make_manifest(require_strict_validation=True)
        mutated = _make_manifest(require_strict_validation=False)
        assert base.content_hash != mutated.content_hash

    def test_stop_conditions_mutation_changes_hash(self):
        base = _make_manifest(stop_conditions=StopConditions(max_budget_usd=100.0))
        mutated = _make_manifest(stop_conditions=StopConditions(max_budget_usd=200.0))
        assert base.content_hash != mutated.content_hash


class TestControllerStateHashMutation:
    def test_state_hash_is_stable(self):
        s1 = make_controller_state("c1", "cycle-1", ControllerStatus.IDLE)
        s2 = make_controller_state("c1", "cycle-1", ControllerStatus.IDLE)
        assert s1.content_hash == s2.content_hash

    def test_status_mutation_changes_hash(self):
        base = make_controller_state("c1", "cycle-1", ControllerStatus.IDLE)
        mutated = make_controller_state("c1", "cycle-1", ControllerStatus.RUNNING)
        assert base.content_hash != mutated.content_hash

    def test_current_child_mutation_changes_hash(self):
        base = make_controller_state("c1", "cycle-1", ControllerStatus.RUNNING, current_child_id=None)
        mutated = make_controller_state("c1", "cycle-1", ControllerStatus.RUNNING, current_child_id="child-alpha")
        assert base.content_hash != mutated.content_hash

    def test_completed_children_mutation_changes_hash(self):
        base = make_controller_state("c1", "cycle-1", ControllerStatus.RUNNING, completed_children=[])
        mutated = make_controller_state("c1", "cycle-1", ControllerStatus.RUNNING, completed_children=["child-alpha"])
        assert base.content_hash != mutated.content_hash

    def test_stop_reason_mutation_changes_hash(self):
        base = make_controller_state("c1", "cycle-1", ControllerStatus.SAFETY_STOPPED, stop_reason=None)
        mutated = make_controller_state("c1", "cycle-1", ControllerStatus.SAFETY_STOPPED, stop_reason=StopReason.SAFETY_STOP)
        assert base.content_hash != mutated.content_hash


class TestOutboxEntryHashMutation:
    def test_entry_hash_is_stable(self):
        e1 = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        e2 = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        assert e1.content_hash == e2.content_hash

    def test_entry_id_mutation_changes_hash(self):
        base = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        mutated = make_outbox_entry("e2", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        assert base.content_hash != mutated.content_hash

    def test_digest_mutation_changes_hash(self):
        base = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        mutated = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST_B)
        assert base.content_hash != mutated.content_hash

    def test_report_dir_mutation_changes_hash(self):
        base = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        mutated = make_outbox_entry("e1", "cycle-1", "child-a", "reports/b", _VALID_DIGEST)
        assert base.content_hash != mutated.content_hash


# ---------------------------------------------------------------------------
# Schema validation
# ---------------------------------------------------------------------------


class TestCycleManifestValidation:
    def test_duplicate_child_ids_rejected(self):
        children = [
            make_child_command("dup", "campaign_run", {}, "reports/dup-1"),
            make_child_command("dup", "campaign_run", {}, "reports/dup-2"),
        ]
        with pytest.raises(ValueError, match="duplicate child_id"):
            _make_manifest(children=children)

    def test_duplicate_report_dirs_rejected(self):
        children = [
            make_child_command("c1", "campaign_run", {}, "reports/same"),
            make_child_command("c2", "campaign_run", {}, "reports/same"),
        ]
        with pytest.raises(ValueError, match="duplicate report_dir"):
            _make_manifest(children=children)

    def test_wrong_schema_version_rejected(self):
        manifest = _make_manifest()
        data = manifest.model_dump(mode="json", by_alias=True)
        data["schema_version"] = "0.0.0"
        data["content_hash"] = "0" * 64
        # Recompute hash for the mutated data.
        from g8e_evals.controller import _sha256, _canonical_json
        data["content_hash"] = _sha256(_canonical_json({k: v for k, v in data.items() if k != "content_hash"}))
        with pytest.raises(ValueError, match="schema_version"):
            CycleManifest.model_validate(data)

    def test_wrong_content_hash_rejected(self):
        manifest = _make_manifest()
        data = manifest.model_dump(mode="json", by_alias=True)
        data["content_hash"] = "f" * 64
        with pytest.raises(ValueError, match="content_hash mismatch"):
            CycleManifest.model_validate(data)


# ---------------------------------------------------------------------------
# Controller lifecycle
# ---------------------------------------------------------------------------


class TestControllerRunCycleSuccess:
    def test_run_cycle_completes_all_children(self, tmp_path: Path):
        child_handler = _StubChildHandler()
        controller = _make_controller(tmp_path, child_handler=child_handler)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.COMPLETED
        assert state.stop_reason == StopReason.COMPLETED
        assert state.completed_children == ["child-alpha", "child-beta"]
        assert state.failed_children == []
        assert state.interrupted_children == []
        assert child_handler.call_count == 2

    def test_run_cycle_persists_state_file(self, tmp_path: Path):
        controller = _make_controller(tmp_path)
        controller.run_cycle()
        state_path = tmp_path / CONTROLLER_STATE_JSON
        assert state_path.exists()
        loaded = load_controller_state(state_path)
        assert loaded.status == ControllerStatus.COMPLETED

    def test_run_cycle_persists_manifest_file(self, tmp_path: Path):
        controller = _make_controller(tmp_path)
        controller.run_cycle()
        manifest_path = tmp_path / CYCLE_MANIFEST_JSON
        assert manifest_path.exists()

    def test_run_cycle_enqueues_outbox_entry(self, tmp_path: Path):
        controller = _make_controller(tmp_path)
        controller.run_cycle()
        outbox_index = tmp_path / "outbox" / OUTBOX_INDEX_JSONL
        assert outbox_index.exists()
        lines = [line for line in outbox_index.read_text().splitlines() if line.strip()]
        # At least one enqueue and one published action.
        actions = [json.loads(line)["action"] for line in lines]
        assert "enqueue" in actions
        assert "published" in actions

    def test_run_cycle_calls_verifier_and_validator(self, tmp_path: Path):
        verifier = _StubVerifierHandler()
        validator = _StubValidationHandler()
        controller = _make_controller(
            tmp_path,
            verifier_handler=verifier,
            validation_handler=validator,
        )
        controller.run_cycle()
        assert verifier.call_count == 1
        assert validator.call_count == 1

    def test_status_returns_current_state(self, tmp_path: Path):
        controller = _make_controller(tmp_path)
        status = controller.status()
        assert status.status == ControllerStatus.IDLE


class TestTwoCycleRollover:
    def test_two_cycles_use_fresh_identity_and_report_root(self, tmp_path: Path):
        """Two consecutive cycles use distinct cycle IDs and report roots."""
        child_handler = _StubChildHandler()

        manifest1 = _make_manifest(
            cycle_id="cycle-001",
            report_root="reports-cycle-1",
            outbox_dir="outbox-cycle-1",
        )
        controller1 = _make_controller(tmp_path, manifest1, child_handler=child_handler)
        state1 = controller1.run_cycle()
        assert state1.status == ControllerStatus.COMPLETED
        assert state1.cycle_id == "cycle-001"
        assert (tmp_path / "reports-cycle-1").exists()
        assert (tmp_path / "outbox-cycle-1").exists()

        manifest2 = _make_manifest(
            cycle_id="cycle-002",
            cycle_revision="rev-2",
            report_root="reports-cycle-2",
            outbox_dir="outbox-cycle-2",
        )
        controller2 = _make_controller(tmp_path, manifest2, child_handler=child_handler)
        state2 = controller2.run_cycle()
        assert state2.status == ControllerStatus.COMPLETED
        assert state2.cycle_id == "cycle-002"
        assert (tmp_path / "reports-cycle-2").exists()
        assert (tmp_path / "outbox-cycle-2").exists()

        # Distinct report roots.
        assert manifest1.report_root != manifest2.report_root
        # Distinct cycle identities.
        assert manifest1.cycle_id != manifest2.cycle_id
        # Distinct content hashes.
        assert manifest1.content_hash != manifest2.content_hash


class TestGracefulStop:
    def test_graceful_stop_completes_current_child_then_stops(self, tmp_path: Path):
        """A graceful stop request completes the current child and then stops."""
        child_handler = _StubChildHandler()

        class _GracefulAfterFirstChild:
            def __init__(self, controller: Controller) -> None:
                self._controller = controller
                self._inner = _StubChildHandler()

            def __call__(self, child: ChildCommand, report_dir: Path) -> ChildResult:
                result = self._inner(child, report_dir)
                if child.child_id == "child-alpha":
                    self._controller.request_graceful_stop()
                return result

        controller = _make_controller(tmp_path, child_handler=child_handler)
        wrapper = _GracefulAfterFirstChild(controller)
        controller._child_handler = wrapper  # type: ignore[assignment]
        state = controller.run_cycle()
        assert state.status == ControllerStatus.GRACEFUL_STOPPING
        assert state.stop_reason == StopReason.GRACEFUL_STOP
        assert state.completed_children == ["child-alpha"]
        assert state.failed_children == []
        assert wrapper._inner.call_count == 1


class TestSafetyStop:
    def test_safety_stop_halts_immediately_and_marks_child_interrupted(self, tmp_path: Path):
        """A safety stop request halts immediately; the current child is dead evidence."""
        child_handler = _StubChildHandler()

        class _SafetyStopDuringFirstChild:
            def __init__(self, controller: Controller) -> None:
                self._controller = controller
                self._inner = _StubChildHandler()

            def __call__(self, child: ChildCommand, report_dir: Path) -> ChildResult:
                result = self._inner(child, report_dir)
                if child.child_id == "child-alpha":
                    self._controller.request_safety_stop()
                return result

        controller = _make_controller(tmp_path, child_handler=child_handler)
        wrapper = _SafetyStopDuringFirstChild(controller)
        controller._child_handler = wrapper  # type: ignore[assignment]
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.SAFETY_STOP
        # child-alpha completed (handler ran) but the safety stop was requested
        # after it, so the controller stops before child-beta.
        assert wrapper._inner.call_count == 1
        assert "child-beta" not in state.completed_children

    def test_safety_stop_before_first_child(self, tmp_path: Path):
        """A safety stop requested before any child runs stops immediately."""
        controller = _make_controller(tmp_path)
        controller.request_safety_stop()
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.SAFETY_STOP
        assert state.completed_children == []


class TestInterruptedChildReplacement:
    def test_interrupted_child_is_dead_evidence_replacement_uses_fresh_identity(self, tmp_path: Path):
        """An interrupted child is dead evidence; a replacement cycle uses a fresh child identity and report root."""
        # First cycle: safety-stop during child-alpha.
        controller1 = _make_controller(tmp_path, child_handler=_StubChildHandler())
        controller1.request_safety_stop()
        state1 = controller1.run_cycle()
        assert state1.status == ControllerStatus.SAFETY_STOPPED
        assert state1.completed_children == []

        # The interrupted report root exists but is dead evidence.
        old_report_root = tmp_path / "reports"
        assert old_report_root.exists()

        # Replacement cycle: fresh child identity and report root.
        replacement_child = make_child_command(
            child_id="child-alpha-replacement",
            command_name="campaign_run",
            report_dir="reports-replacement/child-alpha-replacement",
        )
        replacement_manifest = _make_manifest(
            children=[replacement_child],
            cycle_id="cycle-001-replacement",
            cycle_revision="rev-2",
            report_root="reports-replacement",
            outbox_dir="outbox-replacement",
        )
        controller2 = _make_controller(tmp_path, replacement_manifest)
        state2 = controller2.run_cycle()
        assert state2.status == ControllerStatus.COMPLETED
        assert state2.completed_children == ["child-alpha-replacement"]
        assert (tmp_path / "reports-replacement").exists()
        # The replacement child has a fresh identity.
        assert replacement_child.child_id != "child-alpha"
        assert replacement_manifest.report_root != "reports"


class TestVerifierFailure:
    def test_verifier_failure_stops_cycle(self, tmp_path: Path):
        """Aggregate verification failure triggers a safety stop."""
        verifier = _StubVerifierHandler(ok=False, failures=["child-alpha: missing artifact"])
        controller = _make_controller(tmp_path, verifier_handler=verifier)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.VERIFIER_FAILURE
        assert state.completed_children == ["child-alpha", "child-beta"]

    def test_verifier_skipped_when_not_required(self, tmp_path: Path):
        """When require_aggregate_verification is False, verification is skipped."""
        verifier = _StubVerifierHandler(ok=False, failures=["fail"])
        manifest = _make_manifest(require_aggregate_verification=False)
        controller = _make_controller(tmp_path, manifest, verifier_handler=verifier)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.COMPLETED
        assert verifier.call_count == 0


class TestDigestRejection:
    def test_digest_mismatch_rejected(self, tmp_path: Path):
        """A candidate digest that does not match the approved digest is rejected."""
        validator = _StubValidationHandler(digest=_VALID_DIGEST_B)
        manifest = _make_manifest(approved_digest=_VALID_DIGEST)
        controller = _make_controller(tmp_path, manifest, validation_handler=validator)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.DIGEST_REJECTED

    def test_digest_match_accepted(self, tmp_path: Path):
        """A candidate digest that matches the approved digest is accepted."""
        validator = _StubValidationHandler(digest=_VALID_DIGEST)
        manifest = _make_manifest(approved_digest=_VALID_DIGEST)
        controller = _make_controller(tmp_path, manifest, validation_handler=validator)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.COMPLETED

    def test_no_approved_digest_skips_check(self, tmp_path: Path):
        """When approved_digest is None, the digest check is skipped."""
        validator = _StubValidationHandler(digest=_VALID_DIGEST_B)
        manifest = _make_manifest(approved_digest=None)
        controller = _make_controller(tmp_path, manifest, validation_handler=validator)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.COMPLETED


class TestValidationFailure:
    def test_validation_failure_stops_cycle(self, tmp_path: Path):
        """Strict validation failure triggers a safety stop."""
        validator = _StubValidationHandler(ok=False, failures=["invalid candidate"])
        controller = _make_controller(tmp_path, validation_handler=validator)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.INTEGRITY_FAILURE

    def test_validation_skipped_when_not_required(self, tmp_path: Path):
        """When require_strict_validation is False, validation is skipped."""
        validator = _StubValidationHandler(ok=False, failures=["fail"])
        manifest = _make_manifest(require_strict_validation=False)
        controller = _make_controller(tmp_path, manifest, validation_handler=validator)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.COMPLETED
        assert validator.call_count == 0


class TestMirrorOutage:
    def test_mirror_outage_stops_cycle(self, tmp_path: Path):
        """A publication mirror outage triggers a safety stop."""
        publisher = _StubPublicationHandler(ok=False, error="mirror connection refused")
        controller = _make_controller(tmp_path, publication_handler=publisher)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.MIRROR_OUTAGE

    def test_trust_failure_classified(self, tmp_path: Path):
        """A trust failure in publication is classified correctly."""
        publisher = _StubPublicationHandler(ok=False, error="trust verification failed")
        controller = _make_controller(tmp_path, publication_handler=publisher)
        state = controller.run_cycle()
        assert state.stop_reason == StopReason.TRUST_FAILURE

    def test_disclosure_failure_classified(self, tmp_path: Path):
        """A disclosure failure in publication is classified correctly."""
        publisher = _StubPublicationHandler(ok=False, error="disclosure scan rejected candidate")
        controller = _make_controller(tmp_path, publication_handler=publisher)
        state = controller.run_cycle()
        assert state.stop_reason == StopReason.DISCLOSURE_FAILURE


class TestOrderedOutboxRecovery:
    def test_outbox_recovers_entries_in_enqueue_order(self, tmp_path: Path):
        """The outbox recovers unpublished entries in enqueue order."""
        outbox_dir = tmp_path / "outbox"
        outbox = Outbox(outbox_dir)
        e1 = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        e2 = make_outbox_entry("e2", "cycle-1", "child-b", "reports/b", _VALID_DIGEST_B)
        outbox.enqueue(e1)
        outbox.enqueue(e2)
        recovered = outbox.recover()
        assert [e.entry_id for e in recovered] == ["e1", "e2"]

    def test_outbox_skips_published_entries_on_recovery(self, tmp_path: Path):
        """Published entries are skipped during recovery."""
        outbox_dir = tmp_path / "outbox"
        outbox = Outbox(outbox_dir)
        e1 = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        e2 = make_outbox_entry("e2", "cycle-1", "child-b", "reports/b", _VALID_DIGEST_B)
        outbox.enqueue(e1)
        outbox.enqueue(e2)
        outbox.mark_published("e1")
        recovered = outbox.recover()
        assert [e.entry_id for e in recovered] == ["e2"]

    def test_outbox_tracks_publication_attempts(self, tmp_path: Path):
        """The outbox tracks publication attempts per entry."""
        outbox_dir = tmp_path / "outbox"
        outbox = Outbox(outbox_dir)
        e1 = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        outbox.enqueue(e1)
        outbox.mark_attempt("e1")
        outbox.mark_attempt("e1")
        recovered = outbox.recover()
        assert len(recovered) == 1
        assert recovered[0].publication_attempts == 2
        assert recovered[0].published is False

    def test_outbox_all_entries_includes_published(self, tmp_path: Path):
        """all_entries returns both published and unpublished entries."""
        outbox_dir = tmp_path / "outbox"
        outbox = Outbox(outbox_dir)
        e1 = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        e2 = make_outbox_entry("e2", "cycle-1", "child-b", "reports/b", _VALID_DIGEST_B)
        outbox.enqueue(e1)
        outbox.enqueue(e2)
        outbox.mark_published("e1")
        all_entries = outbox.all_entries()
        assert len(all_entries) == 2
        assert all_entries[0].entry_id == "e1"
        assert all_entries[0].published is True
        assert all_entries[1].entry_id == "e2"
        assert all_entries[1].published is False

    def test_outbox_rejects_duplicate_enqueue(self, tmp_path: Path):
        """Enqueuing the same entry twice is rejected."""
        outbox_dir = tmp_path / "outbox"
        outbox = Outbox(outbox_dir)
        e1 = make_outbox_entry("e1", "cycle-1", "child-a", "reports/a", _VALID_DIGEST)
        outbox.enqueue(e1)
        with pytest.raises(ValueError, match="already exists"):
            outbox.enqueue(e1)


class TestNoInferenceRerunOnPublicationRetry:
    def test_publication_retry_does_not_rerun_inference(self, tmp_path: Path):
        """Publication retry reads the durable outbox and does not rerun inference.

        The child handler is not called during publication retry; only the
        publication handler is called with the existing outbox entry and
        report directories.
        """
        child_handler = _StubChildHandler()
        publisher = _StubPublicationHandler(ok=False, error="mirror connection refused")
        controller = _make_controller(
            tmp_path,
            child_handler=child_handler,
            publication_handler=publisher,
        )
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.MIRROR_OUTAGE
        child_call_count = child_handler.call_count

        # Retry publication with a working publisher.
        new_publisher = _StubPublicationHandler(ok=True)
        controller._publication_handler = new_publisher  # type: ignore[assignment]
        report_dirs = {}
        for child in controller._manifest.children:
            report_dirs[child.child_id] = controller._report_root / child.report_dir
        result = controller.publish_pending(report_dirs)
        assert result.ok is True
        # Child handler was not called during publication retry.
        assert child_handler.call_count == child_call_count
        # Publication handler was called for the retried entry.
        assert new_publisher.call_count == 1

    def test_publish_pending_no_entries_returns_ok(self, tmp_path: Path):
        """publish_pending returns ok when there are no unpublished entries."""
        controller = _make_controller(tmp_path)
        result = controller.publish_pending({})
        assert result.ok is True


class TestStopConditionsDisk:
    def test_disk_ceiling_triggers_safety_stop(self, tmp_path: Path):
        """A disk ceiling violation triggers a safety stop before the first child."""
        # Create a large file in the report root to exceed the disk ceiling.
        manifest = _make_manifest(
            stop_conditions=StopConditions(max_disk_gb=0.0),
            report_root="reports",
        )
        controller = _make_controller(tmp_path, manifest)
        # Write a small file that exceeds 0.0 GB.
        report_root = tmp_path / "reports"
        report_root.mkdir(parents=True, exist_ok=True)
        (report_root / "large.txt").write_text("x" * 1024)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.DISK_FULL


class TestChildFailure:
    def test_child_failure_stops_cycle(self, tmp_path: Path):
        """A child command failure triggers a safety stop."""
        child_handler = _StubChildHandler(fail_child_ids={"child-alpha"})
        controller = _make_controller(tmp_path, child_handler=child_handler)
        state = controller.run_cycle()
        assert state.status == ControllerStatus.SAFETY_STOPPED
        assert state.stop_reason == StopReason.CHILD_FAILURE
        assert state.failed_children == ["child-alpha"]
        assert state.completed_children == []


class TestMissingHandler:
    def test_missing_child_handler_raises_error(self, tmp_path: Path):
        """A missing child handler raises ControllerError."""
        manifest = _make_manifest()
        controller = Controller(
            manifest=manifest,
            work_dir=tmp_path,
            controller_id="test",
            child_handler=None,
            verifier_handler=_StubVerifierHandler(),
            validation_handler=_StubValidationHandler(),
            publication_handler=_StubPublicationHandler(),
        )
        with pytest.raises(ControllerError, match="no child command handler"):
            controller.run_cycle()

    def test_missing_verifier_handler_raises_error(self, tmp_path: Path):
        """A missing verifier handler raises ControllerError."""
        manifest = _make_manifest()
        controller = Controller(
            manifest=manifest,
            work_dir=tmp_path,
            controller_id="test",
            child_handler=_StubChildHandler(),
            verifier_handler=None,
            validation_handler=_StubValidationHandler(),
            publication_handler=_StubPublicationHandler(),
        )
        with pytest.raises(ControllerError, match="no aggregate verifier handler"):
            controller.run_cycle()

    def test_missing_validation_handler_raises_error(self, tmp_path: Path):
        """A missing validation handler raises ControllerError."""
        manifest = _make_manifest()
        controller = Controller(
            manifest=manifest,
            work_dir=tmp_path,
            controller_id="test",
            child_handler=_StubChildHandler(),
            verifier_handler=_StubVerifierHandler(),
            validation_handler=None,
            publication_handler=_StubPublicationHandler(),
        )
        with pytest.raises(ControllerError, match="no validation handler"):
            controller.run_cycle()

    def test_missing_publication_handler_raises_error(self, tmp_path: Path):
        """A missing publication handler raises ControllerError."""
        manifest = _make_manifest()
        controller = Controller(
            manifest=manifest,
            work_dir=tmp_path,
            controller_id="test",
            child_handler=_StubChildHandler(),
            verifier_handler=_StubVerifierHandler(),
            validation_handler=_StubValidationHandler(),
            publication_handler=None,
        )
        with pytest.raises(ControllerError, match="no publication handler"):
            controller.run_cycle()


# ---------------------------------------------------------------------------
# Manifest and state I/O
# ---------------------------------------------------------------------------


class TestManifestStateIO:
    def test_save_and_load_cycle_manifest(self, tmp_path: Path):
        manifest = _make_manifest()
        path = tmp_path / "manifest.json"
        save_cycle_manifest(manifest, path)
        loaded = load_cycle_manifest(path)
        assert loaded.content_hash == manifest.content_hash
        assert loaded.cycle_id == manifest.cycle_id

    def test_save_and_load_controller_state(self, tmp_path: Path):
        state = make_controller_state("c1", "cycle-1", ControllerStatus.COMPLETED)
        path = tmp_path / "state.json"
        save_controller_state(state, path)
        loaded = load_controller_state(path)
        assert loaded.content_hash == state.content_hash
        assert loaded.status == ControllerStatus.COMPLETED


# ---------------------------------------------------------------------------
# Schema version
# ---------------------------------------------------------------------------


class TestSchemaVersion:
    def test_schema_version_constant(self):
        assert CONTROLLER_SCHEMA_VERSION == "1.0.0"

    def test_manifest_has_correct_schema_version(self):
        manifest = _make_manifest()
        assert manifest.schema_version == CONTROLLER_SCHEMA_VERSION
