# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Lease lifecycle management for the unified eval tooling.

This module implements ``lease issue``, ``lease inspect``, ``lease stop``,
and ``lease expire`` with one-active-lease enforcement and owner-local
storage. The Go facade computes the runtime state (candidate identity,
model inventory digest, endpoint URL, app/operator identity) and passes
it as a typed JSON request. This module constructs the lease, enforces
one-active-lease, and stores it in the owner-local authority directory.

Leases are stored as JSON files named ``<lease_id>.json`` in the lease
store directory. Only one lease may be ACTIVE at a time; issuing a new
lease when an active lease exists fails with a typed error. Stop and
expire transition the active lease to a terminal status. Publication
retry does not reactivate a terminal lease.

The module performs direct file I/O in the lease store directory. The Go
facade resolves the store directory path through ``RuntimeFileService``
and path constants and passes it to this module.
"""

from __future__ import annotations

import uuid
from datetime import UTC, datetime, timedelta
from enum import StrEnum
from pathlib import Path
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.live_operations_authority import (
    ArtifactBinding,
    BudgetAuthority,
    CandidateIdentity,
    CommandFamily,
    LeaseStatus,
    LiveOperationLease,
    OperationKind,
    build_budget_authority,
    build_live_operation_lease,
    build_live_operation_lease_template,
    compute_request_digest,
    render_authority_json,
)
from g8e_evals.operation_config import (
    OperationConfigBase,
    load_operation_config,
)

LEASE_LIFECYCLE_SCHEMA_VERSION = "1.0.0"


class LeaseCommandFamily(StrEnum):
    """The command family for a lease issue request. Maps the unified
    eval tooling's operation kinds to the authority's CommandFamily."""

    CAMPAIGN_RUN = "campaign_run"
    CONTROLLER_RUN = "controller_run"


class LeaseIssueRequest(BaseModel):
    """Typed request the Go facade sends to issue a lease."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    config_path: str = Field(min_length=1)
    lease_store_dir: str = Field(min_length=1)
    command_family: LeaseCommandFamily
    command_version: str = Field(min_length=1)
    candidate: CandidateIdentity
    model_inventory_digest: str = Field(pattern=r"^[0-9a-f]{64}$")
    endpoint: str = Field(min_length=1)
    app_identity: str = Field(min_length=1)
    operator_session_identity: str = Field(min_length=1)
    expires_in_seconds: int = Field(gt=0)
    start_deadline_seconds: int = Field(default=300, gt=0)
    operation_kind: OperationKind
    required_runtime_authority_names: list[str] = Field(min_length=1)


class LeaseIssueResult(BaseModel):
    """Typed result returned by lease issue."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    lease_id: str
    lease_path: str
    status: LeaseStatus
    issued_at: datetime
    start_deadline: datetime
    expires_at: datetime
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    request_digest: str = Field(pattern=r"^[0-9a-f]{64}$")
    operation_config_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class LeaseTransitionRequest(BaseModel):
    """Typed request to stop, expire, or complete a lease."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    config_path: str = Field(min_length=1)
    lease_store_dir: str = Field(min_length=1)
    transition: Literal["stop", "expire", "complete"]


class LeaseTransitionResult(BaseModel):
    """Typed result returned by lease stop or expire."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    lease_id: str
    previous_status: LeaseStatus
    new_status: LeaseStatus
    transitioned_at: datetime


class LeaseInspectRequest(BaseModel):
    """Typed request to inspect the active lease for a config."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    config_path: str = Field(min_length=1)
    lease_store_dir: str = Field(min_length=1)


class LeaseInspectResult(BaseModel):
    """Typed result returned by lease inspect."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    found: bool
    lease: LiveOperationLease | None = None


class LeaseLifecycleError(Exception):
    """Raised when a lease lifecycle operation fails."""

    def __init__(self, code: str, detail: str) -> None:
        self.code = code
        self.detail = detail
        super().__init__(f"{code}: {detail}")


def _config_to_budget_authority(config: OperationConfigBase) -> BudgetAuthority:
    """Construct a BudgetAuthority from an operation config's BudgetCeilings."""
    ceilings = config.budget
    max_tokens_per_request = 0
    if ceilings.max_tokens > 0 and ceilings.max_requests > 0:
        max_tokens_per_request = ceilings.max_tokens // ceilings.max_requests
    if max_tokens_per_request == 0:
        max_tokens_per_request = 16_384
    min_free_disk_bytes = 0
    if ceilings.min_free_disk_gb is not None:
        min_free_disk_bytes = int(ceilings.min_free_disk_gb * 1_073_741_824)
    if min_free_disk_bytes == 0:
        min_free_disk_bytes = 1_073_741_824
    max_duration = int(config.stop_conditions.max_duration_s) if config.stop_conditions.max_duration_s else 3600
    return build_budget_authority(
        operation_id=config.operation_id,
        max_requests=ceilings.max_requests,
        max_tokens_per_request=max_tokens_per_request,
        max_tokens=ceilings.max_tokens,
        max_usd=ceilings.max_usd,
        concurrency=ceilings.concurrency,
        min_free_disk_bytes=min_free_disk_bytes,
        max_duration_seconds=max_duration,
        max_retries=ceilings.max_retries,
        max_replacements=0,
    )


def _config_to_authority_names(config: OperationConfigBase) -> list[str]:
    """Extract runtime authority names from the config's authority refs."""
    names = ["gold_set", "evidence_key"]
    if hasattr(config, "preregistration") and config.preregistration is not None:
        names.append("preregistration")
    if hasattr(config, "profile") and config.profile is not None:
        names.append("profile")
    if hasattr(config, "model_registry") and config.model_registry is not None:
        names.append("model_registry")
    return names


def _list_leases(lease_store_dir: Path) -> list[LiveOperationLease]:
    """List all leases in the store directory."""
    if not lease_store_dir.exists():
        return []
    leases: list[LiveOperationLease] = []
    for path in sorted(lease_store_dir.glob("*.json")):
        try:
            lease = LiveOperationLease.model_validate_json(path.read_text())
            leases.append(lease)
        except Exception:
            continue
    return leases


def _find_active_lease(leases: list[LiveOperationLease]) -> LiveOperationLease | None:
    """Find the active lease, if any."""
    active = [lease for lease in leases if lease.status == LeaseStatus.ACTIVE]
    if len(active) > 1:
        raise LeaseLifecycleError(
            "multiple_active_leases",
            f"found {len(active)} active leases; only one is permitted",
        )
    return active[0] if active else None


def _find_lease_for_config(
    leases: list[LiveOperationLease], config: OperationConfigBase
) -> LiveOperationLease | None:
    """Find the lease matching the config's operation_id and report_root."""
    for lease in leases:
        if lease.operation_identity == config.operation_id and lease.report_root == config.report_root:
            return lease
    return None


def issue_lease(request: LeaseIssueRequest) -> LeaseIssueResult:
    """Issue a new active lease for the given config and runtime state.

    Enforces one-active-lease: if an active lease already exists in the
    store, the operation fails with ``active_lease_exists``. The new
    lease is stored as ``<lease_id>.json`` in the lease store directory.
    """
    config = load_operation_config(Path(request.config_path))
    config_content_hash = config.content_hash or config.compute_content_hash()

    lease_store_dir = Path(request.lease_store_dir)
    existing_leases = _list_leases(lease_store_dir)
    active = _find_active_lease(existing_leases)
    if active is not None:
        raise LeaseLifecycleError(
            "active_lease_exists",
            f"active lease {active.lease_id} already exists; stop or expire it first",
        )

    matching = _find_lease_for_config(existing_leases, config)
    if matching is not None:
        raise LeaseLifecycleError(
            "lease_already_issued",
            f"a lease for {config.operation_id} at {config.report_root} already exists "
            f"with status {matching.status.value}",
        )

    budget = _config_to_budget_authority(config)
    authority_names = request.required_runtime_authority_names
    template = build_live_operation_lease_template(
        template_id=f"{config.operation_id}-{config.revision}",
        operation_kind=request.operation_kind,
        operation_authority_hash=config_content_hash,
        budget=budget,
        endpoint=request.endpoint,
        permitted_command_family=CommandFamily(request.command_family.value),
        required_runtime_authority_names=authority_names,
    )

    request_digest = compute_request_digest(
        operation_config_content_hash=config_content_hash,
        operation_id=config.operation_id,
        command_family=CommandFamily(request.command_family.value),
        command_version=request.command_version,
    )

    now = datetime.now(UTC).replace(microsecond=0)
    lease_id = f"{config.operation_id}-{now.strftime('%Y%m%d-%H%M%S')}-{uuid.uuid4().hex[:8]}"
    runtime_authorities = [
        ArtifactBinding(name=name, path=f"config/{name}.json", sha256=config_content_hash)
        for name in authority_names
    ]

    lease = build_live_operation_lease(
        lease_id=lease_id,
        template=template,
        candidate=request.candidate,
        model_inventory_digest=request.model_inventory_digest,
        operation_identity=config.operation_id,
        runtime_authorities=runtime_authorities,
        request_digest=request_digest,
        command_family=CommandFamily(request.command_family.value),
        command_version=request.command_version,
        report_root=config.report_root,
        app_identity=request.app_identity,
        operator_session_identity=request.operator_session_identity,
        issued_at=now,
        start_deadline=now + timedelta(seconds=request.start_deadline_seconds),
        expires_at=now + timedelta(seconds=request.expires_in_seconds),
        status=LeaseStatus.ACTIVE,
    )

    lease_store_dir.mkdir(parents=True, exist_ok=True)
    lease_path = lease_store_dir / f"{lease_id}.json"
    lease_path.write_bytes(render_authority_json(lease))

    return LeaseIssueResult(
        lease_id=lease_id,
        lease_path=str(lease_path),
        status=lease.status,
        issued_at=lease.issued_at,
        start_deadline=lease.start_deadline,
        expires_at=lease.expires_at,
        content_hash=lease.content_hash,
        request_digest=request_digest,
        operation_config_content_hash=config_content_hash,
    )


def inspect_lease(request: LeaseInspectRequest) -> LeaseInspectResult:
    """Inspect the lease matching the given config, if any."""
    config = load_operation_config(Path(request.config_path))
    lease_store_dir = Path(request.lease_store_dir)
    leases = _list_leases(lease_store_dir)
    lease = _find_lease_for_config(leases, config)
    if lease is None:
        return LeaseInspectResult(found=False, lease=None)
    return LeaseInspectResult(found=True, lease=lease)


def transition_lease(request: LeaseTransitionRequest) -> LeaseTransitionResult:
    """Stop, expire, or complete the active lease matching the given config.

    ``stop`` transitions to STOPPED (graceful or forced termination).
    ``expire`` transitions to EXPIRED (deadline passed). ``complete``
    transitions to COMPLETED (the finite command reached a typed terminal
    state). All three are terminal; the lease cannot be reactivated.
    Publication retry does not reactivate a terminal lease.
    """
    config = load_operation_config(Path(request.config_path))
    lease_store_dir = Path(request.lease_store_dir)
    leases = _list_leases(lease_store_dir)
    lease = _find_lease_for_config(leases, config)
    if lease is None:
        raise LeaseLifecycleError(
            "lease_not_found",
            f"no lease found for {config.operation_id} at {config.report_root}",
        )
    if lease.status != LeaseStatus.ACTIVE:
        raise LeaseLifecycleError(
            "lease_not_active",
            f"lease {lease.lease_id} status is {lease.status.value}, not active",
        )

    _TRANSITION_STATUS = {
        "stop": LeaseStatus.STOPPED,
        "expire": LeaseStatus.EXPIRED,
        "complete": LeaseStatus.COMPLETED,
    }
    new_status = _TRANSITION_STATUS[request.transition]
    now = datetime.now(UTC).replace(microsecond=0)
    updated = build_live_operation_lease(
        lease_id=lease.lease_id,
        template=lease.template,
        candidate=lease.candidate,
        model_inventory_digest=lease.model_inventory_digest,
        operation_identity=lease.operation_identity,
        runtime_authorities=lease.runtime_authorities,
        request_digest=lease.request_digest,
        command_family=lease.command_family,
        command_version=lease.command_version,
        report_root=lease.report_root,
        app_identity=lease.app_identity,
        operator_session_identity=lease.operator_session_identity,
        issued_at=lease.issued_at,
        start_deadline=lease.start_deadline,
        expires_at=lease.expires_at,
        status=new_status,
    )

    lease_path = lease_store_dir / f"{lease.lease_id}.json"
    lease_path.write_bytes(render_authority_json(updated))

    return LeaseTransitionResult(
        lease_id=lease.lease_id,
        previous_status=lease.status,
        new_status=new_status,
        transitioned_at=now,
    )


__all__ = [
    "LEASE_LIFECYCLE_SCHEMA_VERSION",
    "LeaseCommandFamily",
    "LeaseInspectRequest",
    "LeaseInspectResult",
    "LeaseIssueRequest",
    "LeaseIssueResult",
    "LeaseLifecycleError",
    "LeaseTransitionRequest",
    "LeaseTransitionResult",
    "inspect_lease",
    "issue_lease",
    "transition_lease",
]
