# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Narrow CLI entry point for lease start verification invoked by the
Go facade before a provider-backed eval start.

The Go ``./g8e eval`` start commands (diagnostic start, campaign start,
controller run) write a JSON request file and invoke::

    <eval-venv-python> -m g8e_evals.lease_start_verification_cli <request-path>

This module loads the operation config, finds the lease bound to it,
builds a ``LeaseVerificationContext`` from the actual runtime state
supplied by the Go facade, and calls ``verify_lease_for_start``. The
result is emitted as a single JSON object on stdout. On verification
failure the result carries a stable ``LeaseVerificationFailureCode``
that the Go facade maps to a typed sentinel error.

This module is not a public CLI; operators discover start through
``./g8e eval``.
"""

from __future__ import annotations

import sys
from datetime import UTC, datetime
from pathlib import Path

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from g8e_evals.lease_verification import (
    LeaseVerificationContext,
    LeaseVerificationError,
    verify_lease_for_start,
)
from g8e_evals.live_operations_authority import (
    CandidateIdentity,
    CommandFamily,
)
from g8e_evals.lease_lifecycle import _find_lease_for_config, _list_leases
from g8e_evals.operation_config import load_operation_config


class LeaseStartVerificationRequest(BaseModel):
    """Typed request the Go facade sends to verify a lease before start."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    config_path: str = Field(min_length=1)
    lease_store_dir: str = Field(min_length=1)
    repository_root: str = Field(min_length=1)
    candidate: CandidateIdentity
    model_inventory_digest: str = Field(pattern=r"^[0-9a-f]{64}$")
    command_family: str = Field(min_length=1)
    command_version: str = Field(min_length=1)


class LeaseStartVerificationResult(BaseModel):
    """Typed result emitted by lease start verification."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    verified: bool
    failure_code: str = ""
    failure_detail: str = ""
    operation_id: str = ""
    revision: str = ""
    report_root: str = ""
    lease_id: str = ""
    lease_path: str = ""
    content_hash: str = ""


def verify_lease_for_start_request(
    request: LeaseStartVerificationRequest,
) -> LeaseStartVerificationResult:
    """Load the config and lease, verify the lease, and return the result.

    On success the result carries the operation_id, revision, report_root,
    lease_id, lease_path, and config content_hash so the Go facade can
    construct an ``EvalEngineRequest`` without a second round-trip. On
    failure the result carries the stable ``LeaseVerificationFailureCode``
    value and a safe detail string.
    """
    config = load_operation_config(Path(request.config_path))
    config_content_hash = config.content_hash or config.compute_content_hash()

    lease_store_dir = Path(request.lease_store_dir)
    leases = _list_leases(lease_store_dir)
    lease = _find_lease_for_config(leases, config)

    report_root_abs = Path(request.repository_root) / config.report_root
    report_root_exists = report_root_abs.exists()

    ctx = LeaseVerificationContext(
        operation_config_content_hash=config_content_hash,
        operation_id=config.operation_id,
        command_family=CommandFamily(request.command_family),
        command_version=request.command_version,
        candidate=request.candidate,
        model_inventory_digest=request.model_inventory_digest,
        report_root_exists=report_root_exists,
        now=datetime.now(UTC).replace(microsecond=0),
        operation_max_duration_s=config.stop_conditions.max_duration_s,
    )

    try:
        verify_lease_for_start(lease, ctx)
    except LeaseVerificationError as exc:
        return LeaseStartVerificationResult(
            verified=False,
            failure_code=exc.code.value,
            failure_detail=exc.detail,
        )

    assert lease is not None  # verify_lease_for_start raises if None
    lease_path = lease_store_dir / f"{lease.lease_id}.json"
    return LeaseStartVerificationResult(
        verified=True,
        operation_id=config.operation_id,
        revision=config.revision,
        report_root=config.report_root,
        lease_id=lease.lease_id,
        lease_path=str(lease_path),
        content_hash=config_content_hash,
    )


def run_lease_start_verification(request_path: str) -> int:
    """Load a lease start verification request, verify, and emit the result."""
    path = Path(request_path)
    if not path.is_file():
        sys.stderr.write(f"request file not found: {request_path}\n")
        return 1

    try:
        req = LeaseStartVerificationRequest.model_validate_json(path.read_bytes())
    except ValidationError as exc:
        sys.stderr.write(f"invalid request: {exc}\n")
        return 1

    try:
        result = verify_lease_for_start_request(req)
    except (ValidationError, ValueError) as exc:
        sys.stderr.write(f"lease start verification failed: {exc}\n")
        return 1

    sys.stdout.write(result.model_dump_json(exclude_none=True))
    sys.stdout.write("\n")
    sys.stdout.flush()
    return 0


def main(argv: list[str] | None = None) -> int:
    """Entry point for ``python -m g8e_evals.lease_start_verification_cli <request-path>``."""
    args = argv if argv is not None else sys.argv[1:]
    if len(args) != 1:
        sys.stderr.write("usage: python -m g8e_evals.lease_start_verification_cli <request-path>\n")
        sys.stderr.write(f"got {len(args)} argument(s)\n")
        return 1
    return run_lease_start_verification(args[0])


if __name__ == "__main__":
    sys.exit(main())
