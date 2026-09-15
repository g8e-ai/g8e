# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Content-hashed, identity-bound eval stop requests.

``./g8e eval <noun> stop`` writes a durable ``eval-stop-request.json``
inside the operation's report root. The running diagnostic task loop and
campaign assignment loop consume the request at safe unit boundaries:
each boundary check re-validates the request's content hash and verifies
the operation, revision, config, and launch identity before honoring it.
A request that fails validation is treated as evidence tampering and
raises ``StopRequestError``; it is never silently honored or ignored.

The request binds to the launch record's content hash so a stop request
issued for one launch can never stop a different launch that reuses the
same operation identity.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

from g8e_evals.serialization import content_hash_of, provisional_hash


class EvalStopRequest(BaseModel):
    """The durable stop request document written into a report root.

    ``content_hash`` is SHA-256 over canonical JSON of every field except
    ``content_hash`` itself; it is verified on load so a tampered request
    fails closed.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_id: str = Field(min_length=1)
    revision: str = Field(min_length=1)
    config_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    launch_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    immediate: bool
    requested_at: str = Field(min_length=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def _validate_content_hash(self) -> Self:
        expected = content_hash_of(self)
        if self.content_hash != expected:
            raise ValueError(
                f"stop request content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class StopRequestIdentity(BaseModel):
    """The identity a stop request must bind to be honored by a running
    operation. Derived from the operation config and the launch record
    the engine wrote at start."""

    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_id: str = Field(min_length=1)
    revision: str = Field(min_length=1)
    config_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    launch_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class StopRequestError(Exception):
    """Raised when a stop request exists but fails validation or does not
    match the running operation's identity. Fail-closed: a tampered or
    foreign stop request is evidence corruption, not a stop signal."""


def build_stop_request(
    identity: StopRequestIdentity,
    *,
    immediate: bool,
    requested_at: str,
) -> EvalStopRequest:
    """Construct a content-hashed stop request bound to ``identity``.

    The content hash is computed over the canonical JSON of the request
    excluding ``content_hash`` itself. The two-phase construction is
    handled by ``provisional_hash``: it builds a provisional model with a
    placeholder hash, computes the real hash, and the final model is
    constructed via normal validation with the correct hash.
    """
    content_hash = provisional_hash(
        EvalStopRequest,
        operation_id=identity.operation_id,
        revision=identity.revision,
        config_content_hash=identity.config_content_hash,
        launch_content_hash=identity.launch_content_hash,
        immediate=immediate,
        requested_at=requested_at,
    )
    return EvalStopRequest(
        operation_id=identity.operation_id,
        revision=identity.revision,
        config_content_hash=identity.config_content_hash,
        launch_content_hash=identity.launch_content_hash,
        immediate=immediate,
        requested_at=requested_at,
        content_hash=content_hash,
    )


def load_stop_request(path: Path, identity: StopRequestIdentity) -> EvalStopRequest | None:
    """Load and verify the stop request at ``path``.

    Returns ``None`` when no request exists. Raises ``StopRequestError``
    when the file is malformed, fails content-hash self-consistency, or
    does not match ``identity``.
    """
    if not path.is_file():
        return None
    try:
        request = EvalStopRequest.model_validate_json(path.read_text())
    except Exception as exc:
        raise StopRequestError(f"invalid stop request {path.name}: {exc}") from exc
    if (
        request.operation_id != identity.operation_id
        or request.revision != identity.revision
        or request.config_content_hash != identity.config_content_hash
        or request.launch_content_hash != identity.launch_content_hash
    ):
        raise StopRequestError(
            f"stop request {path.name} does not match the running operation identity"
        )
    return request


@dataclass
class StopRequestConsumer:
    """Boundary consumer shared between the engine handler and the
    running unit loop. ``poll`` is called at each safe unit boundary;
    the first verified request is recorded in ``consumed`` so the
    handler can distinguish an early stop from a complete run."""

    request_path: Path
    identity: StopRequestIdentity
    consumed: EvalStopRequest | None = field(default=None)

    def poll(self) -> EvalStopRequest | None:
        if self.consumed is not None:
            return self.consumed
        request = load_stop_request(self.request_path, self.identity)
        if request is not None:
            self.consumed = request
        return request


__all__ = [
    "EvalStopRequest",
    "StopRequestConsumer",
    "StopRequestError",
    "StopRequestIdentity",
    "build_stop_request",
    "load_stop_request",
]
