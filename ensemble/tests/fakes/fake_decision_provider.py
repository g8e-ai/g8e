# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from app.decision.provider import DecisionProvider
from app.decision.types import DecisionState, EvaluateResponse, Question


class FakeDecisionProvider(DecisionProvider):
    """Test double for DecisionProvider evaluate calls."""

    def __init__(self) -> None:
        super().__init__()
        self.responses: list[EvaluateResponse] = []
        self.last_request: dict[str, object] | None = None

    def add_response(self, response: EvaluateResponse) -> None:
        self.responses.append(response)

    async def evaluate(
        self,
        *,
        model: str,
        state: DecisionState,
        questions: dict[str, Question],
    ) -> EvaluateResponse:
        self.last_request = {
            "model": model,
            "state": state,
            "questions": questions,
        }
        if not self.responses:
            raise RuntimeError("FakeDecisionProvider has no configured responses")
        return self.responses.pop(0)

    @staticmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        return []
