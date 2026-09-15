# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Lease binding verification for provider-backed eval starts.

Every provider-backed ``./g8e eval`` start (diagnostic, campaign,
controller) must pass lease verification before report-root creation or
engine launch. Verification is fail-closed: any mismatch between the
lease and the actual runtime state produces a typed
``LeaseVerificationError`` with a stable ``LeaseVerificationFailureCode``
that the Go facade maps to a typed sentinel error.

Verification covers the complete binding surface: lease status and time
window, request digest against the loaded operation config, command
family and version, candidate identity (source tree, binary, images),
model inventory digest, report-root freshness, budget, endpoint, and
runtime authority bindings. The lease model already validates internal
consistency (budget matches template, endpoint matches template,
authorities match template); this module verifies the lease against
external runtime state that the lease claims to bind.

The module is pure: it takes a lease, a verification context, and
returns ``None`` on success or raises ``LeaseVerificationError`` on the
first failure. It performs no I/O; the caller supplies the actual
candidate identity, model inventory digest, and report-root existence as
inputs.
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum

from g8e_evals.live_operations_authority import (
    CandidateIdentity,
    CommandFamily,
    LeaseStatus,
    LiveOperationLease,
    compute_request_digest,
)


class LeaseVerificationFailureCode(StrEnum):
    """Centralized stable failure codes for lease binding verification."""

    LEASE_MISSING = "lease_missing"
    LEASE_INACTIVE = "lease_inactive"
    LEASE_EXPIRED = "lease_expired"
    LEASE_CONSUMED = "lease_consumed"
    LEASE_NOT_YET_VALID = "lease_not_yet_valid"
    LEASE_STOPPED = "lease_stopped"
    REQUEST_DIGEST_MISMATCH = "request_digest_mismatch"
    COMMAND_FAMILY_MISMATCH = "command_family_mismatch"
    COMMAND_VERSION_MISMATCH = "command_version_mismatch"
    OPERATION_IDENTITY_MISMATCH = "operation_identity_mismatch"
    CANDIDATE_SOURCE_TREE_DRIFT = "candidate_source_tree_drift"
    CANDIDATE_BINARY_DRIFT = "candidate_binary_drift"
    CANDIDATE_IMAGE_DRIFT = "candidate_image_drift"
    MODEL_INVENTORY_DRIFT = "model_inventory_drift"
    REPORT_ROOT_REUSED = "report_root_reused"
    BUDGET_MISMATCH = "budget_mismatch"
    ENDPOINT_MISMATCH = "endpoint_mismatch"
    AUTHORITY_DRIFT = "authority_drift"


class LeaseVerificationError(Exception):
    """Raised when lease binding verification fails.

    The ``code`` attribute is a stable ``LeaseVerificationFailureCode``
    that the Go facade maps to a typed sentinel error. The ``detail``
    string is safe for human output but must not be parsed for control
    flow.
    """

    def __init__(self, code: LeaseVerificationFailureCode, detail: str) -> None:
        self.code = code
        self.detail = detail
        super().__init__(f"{code.value}: {detail}")


@dataclass(frozen=True)
class LeaseVerificationContext:
    """The actual runtime state a lease is verified against.

    All fields are supplied by the caller after performing the necessary
    I/O (loading the operation config, computing the candidate identity,
    checking the model inventory, testing report-root existence). The
    verifier itself performs no I/O.
    """

    operation_config_content_hash: str
    operation_id: str
    command_family: CommandFamily
    command_version: str
    candidate: CandidateIdentity
    model_inventory_digest: str
    report_root_exists: bool
    now: datetime


def verify_lease_for_start(
    lease: LiveOperationLease | None,
    ctx: LeaseVerificationContext,
) -> None:
    """Verify that ``lease`` authorizes the start described by ``ctx``.

    Raises ``LeaseVerificationError`` on the first binding failure. The
    caller must call this before report-root creation or engine launch;
    a returned ``None`` means the lease is valid, active, within its time
    window, and every binding matches the actual runtime state.

    The checks are ordered so that the cheapest and most diagnostic
    failures surface first: lease presence, status, time window, request
    digest, command family/version, operation identity, candidate
    identity, model inventory, report-root freshness, and finally
    budget/endpoint/authority bindings.
    """
    if lease is None:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_MISSING,
            "no lease supplied; run './g8e eval lease issue' before start",
        )

    _verify_status(lease, ctx)
    _verify_time_window(lease, ctx)
    _verify_command_family(lease, ctx)
    _verify_command_version(lease, ctx)
    _verify_operation_identity(lease, ctx)
    _verify_request_digest(lease, ctx)
    _verify_candidate(lease, ctx)
    _verify_model_inventory(lease, ctx)
    _verify_report_root(lease, ctx)
    _verify_budget(lease, ctx)
    _verify_endpoint(lease, ctx)
    _verify_authorities(lease, ctx)


def _verify_status(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if lease.status == LeaseStatus.COMPLETED:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_CONSUMED,
            f"lease {lease.lease_id} already completed",
        )
    if lease.status == LeaseStatus.STOPPED:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_STOPPED,
            f"lease {lease.lease_id} stopped",
        )
    if lease.status == LeaseStatus.EXPIRED:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_EXPIRED,
            f"lease {lease.lease_id} expired",
        )
    if lease.status == LeaseStatus.DRAFT:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_INACTIVE,
            f"lease {lease.lease_id} is draft, not active",
        )
    if lease.status != LeaseStatus.ACTIVE:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_INACTIVE,
            f"lease {lease.lease_id} status is {lease.status.value}, not active",
        )


def _verify_time_window(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if ctx.now < lease.issued_at:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_NOT_YET_VALID,
            f"lease {lease.lease_id} issued at {lease.issued_at.isoformat()}, "
            f"now is {ctx.now.isoformat()}",
        )
    if ctx.now > lease.expires_at:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_EXPIRED,
            f"lease {lease.lease_id} expired at {lease.expires_at.isoformat()}, "
            f"now is {ctx.now.isoformat()}",
        )
    if ctx.now > lease.start_deadline:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.LEASE_EXPIRED,
            f"lease {lease.lease_id} start deadline {lease.start_deadline.isoformat()} passed, "
            f"now is {ctx.now.isoformat()}",
        )


def _verify_request_digest(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    expected = compute_request_digest(
        operation_config_content_hash=ctx.operation_config_content_hash,
        operation_id=ctx.operation_id,
        command_family=ctx.command_family,
        command_version=ctx.command_version,
    )
    if lease.request_digest != expected:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.REQUEST_DIGEST_MISMATCH,
            f"lease request digest {lease.request_digest} does not match "
            f"computed {expected} for config {ctx.operation_config_content_hash}",
        )


def _verify_command_family(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if lease.command_family != ctx.command_family:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.COMMAND_FAMILY_MISMATCH,
            f"lease command family {lease.command_family.value} does not match "
            f"requested {ctx.command_family.value}",
        )


def _verify_command_version(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if lease.command_version != ctx.command_version:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.COMMAND_VERSION_MISMATCH,
            f"lease command version {lease.command_version} does not match "
            f"requested {ctx.command_version}",
        )


def _verify_operation_identity(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if lease.operation_identity != ctx.operation_id:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.OPERATION_IDENTITY_MISMATCH,
            f"lease operation identity {lease.operation_identity} does not match "
            f"requested {ctx.operation_id}",
        )


def _verify_candidate(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if lease.candidate.source_tree_hash != ctx.candidate.source_tree_hash:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.CANDIDATE_SOURCE_TREE_DRIFT,
            f"lease source tree hash {lease.candidate.source_tree_hash} does not match "
            f"actual {ctx.candidate.source_tree_hash}",
        )
    if lease.candidate.binary_sha256 != ctx.candidate.binary_sha256:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.CANDIDATE_BINARY_DRIFT,
            f"lease binary hash {lease.candidate.binary_sha256} does not match "
            f"actual {ctx.candidate.binary_sha256}",
        )
    if sorted(lease.candidate.image_ids) != sorted(ctx.candidate.image_ids):
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.CANDIDATE_IMAGE_DRIFT,
            f"lease image ids {lease.candidate.image_ids} do not match "
            f"actual {ctx.candidate.image_ids}",
        )


def _verify_model_inventory(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if lease.model_inventory_digest != ctx.model_inventory_digest:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.MODEL_INVENTORY_DRIFT,
            f"lease model inventory digest {lease.model_inventory_digest} does not match "
            f"actual {ctx.model_inventory_digest}",
        )


def _verify_report_root(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if ctx.report_root_exists:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.REPORT_ROOT_REUSED,
            f"report root {lease.report_root} already exists; lease requires a fresh report root",
        )


def _verify_budget(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if lease.budget.content_hash != lease.template.budget.content_hash:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.BUDGET_MISMATCH,
            f"lease budget {lease.budget.content_hash} does not match "
            f"template {lease.template.budget.content_hash}",
        )


def _verify_endpoint(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    if lease.endpoint != lease.template.endpoint:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.ENDPOINT_MISMATCH,
            f"lease endpoint {lease.endpoint} does not match "
            f"template {lease.template.endpoint}",
        )


def _verify_authorities(lease: LiveOperationLease, ctx: LeaseVerificationContext) -> None:
    lease_names = [authority.name for authority in lease.runtime_authorities]
    template_names = lease.template.required_runtime_authority_names
    if lease_names != template_names:
        raise LeaseVerificationError(
            LeaseVerificationFailureCode.AUTHORITY_DRIFT,
            f"lease runtime authorities {lease_names} do not match "
            f"template requirements {template_names}",
        )


__all__ = [
    "LeaseVerificationContext",
    "LeaseVerificationError",
    "LeaseVerificationFailureCode",
    "verify_lease_for_start",
]
