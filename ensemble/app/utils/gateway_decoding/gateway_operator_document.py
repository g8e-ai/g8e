# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Parse Gateway-owned operator documents into g8ee read models."""

from __future__ import annotations

from typing import Any

from app.models.operators import OperatorDocument


def operator_document_from_gateway(doc: dict[str, Any]) -> OperatorDocument:
    """Convert a Gateway operator DTO into the g8ee read-side OperatorDocument."""
    normalized = dict(doc)
    operator_id = normalized.get("id") or normalized.get("operator_id")
    if operator_id is not None and "id" not in normalized:
        normalized["id"] = operator_id
    return OperatorDocument.model_validate(normalized)
