# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Shared helpers for per-tool modules.

Kept tiny on purpose: each tool module is self-contained, and only two
helpers are genuinely shared.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from app.models.investigations import EnrichedInvestigationContext


def convert_args_to_payload[T](
    args_dict: dict[str, object],
    payload_cls: type[T],
    execution_id: str,
    investigation: EnrichedInvestigationContext | None = None,
    **extra_fields: object,
) -> T:
    """Convert raw LLM tool args into a downstream Payload, injecting ``execution_id``.

    Centralises the Args -> Payload conversion so the ``execution_id`` hand-off
    cannot be forgotten when adding new tools.

    When target_operators is not provided and there's exactly one bound operator,
    injects ["all"] for ergonomic single-operator usage.
    """
    payload_dict = {**args_dict, "execution_id": execution_id, **extra_fields}

    # Inject ["all"] for single-operator ergonomics when target_operators is not provided
    if investigation and len(investigation.bound_operators) == 1:
        if "target_operators" not in payload_dict or payload_dict["target_operators"] is None:
            payload_dict["target_operators"] = ["all"]

    return payload_cls.model_validate(payload_dict)
