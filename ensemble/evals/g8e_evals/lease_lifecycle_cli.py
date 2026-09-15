# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Narrow CLI entry point for lease lifecycle operations invoked by the
Go facade.

The Go ``./g8e eval lease`` commands write a JSON request file and invoke::

    <eval-venv-python> -m g8e_evals.lease_lifecycle_cli <request-path>

This module reads the request, dispatches to the lease lifecycle
function, and emits the result as a single JSON object on stdout. It is
not a public CLI; operators discover lease lifecycle through ``./g8e
eval``.
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import Literal

from pydantic import BaseModel, ConfigDict, ValidationError

from g8e_evals.lease_lifecycle import (
    LeaseInspectRequest,
    LeaseIssueRequest,
    LeaseLifecycleError,
    LeaseTransitionRequest,
    inspect_lease,
    issue_lease,
    transition_lease,
)


class LeaseLifecycleCLIRequest(BaseModel):
    """Typed request envelope for the lease lifecycle CLI."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    operation: Literal["issue", "inspect", "stop", "expire", "complete"]
    issue: LeaseIssueRequest | None = None
    inspect: LeaseInspectRequest | None = None
    transition: LeaseTransitionRequest | None = None


def run_lease_lifecycle(request_path: str) -> int:
    """Load a lease lifecycle request, dispatch, and emit the result."""
    path = Path(request_path)
    if not path.is_file():
        sys.stderr.write(f"request file not found: {request_path}\n")
        return 1

    try:
        req = LeaseLifecycleCLIRequest.model_validate_json(path.read_bytes())
    except ValidationError as exc:
        sys.stderr.write(f"invalid request: {exc}\n")
        return 1

    try:
        result: BaseModel
        if req.operation == "issue":
            if req.issue is None:
                sys.stderr.write("issue request missing\n")
                return 1
            result = issue_lease(req.issue)
        elif req.operation == "inspect":
            if req.inspect is None:
                sys.stderr.write("inspect request missing\n")
                return 1
            result = inspect_lease(req.inspect)
        elif req.operation in ("stop", "expire", "complete"):
            if req.transition is None:
                sys.stderr.write("transition request missing\n")
                return 1
            result = transition_lease(req.transition)
        else:
            sys.stderr.write(f"unknown operation: {req.operation!r}\n")
            return 1
    except (LeaseLifecycleError, ValidationError, ValueError) as exc:
        sys.stderr.write(f"lease lifecycle failed: {exc}\n")
        return 1

    sys.stdout.write(result.model_dump_json(exclude_none=True))
    sys.stdout.write("\n")
    sys.stdout.flush()
    return 0


def main(argv: list[str] | None = None) -> int:
    """Entry point for ``python -m g8e_evals.lease_lifecycle_cli <request-path>``."""
    args = argv if argv is not None else sys.argv[1:]
    if len(args) != 1:
        sys.stderr.write("usage: python -m g8e_evals.lease_lifecycle_cli <request-path>\n")
        sys.stderr.write(f"got {len(args)} argument(s)\n")
        return 1
    return run_lease_lifecycle(args[0])


if __name__ == "__main__":
    sys.exit(main())
