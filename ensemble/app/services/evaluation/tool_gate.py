# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from g8e.models.internal_api import EvaluationInferenceContext

from app.models.evaluation_trace import ToolGate


def resolve_tool_gate(evaluation_context: EvaluationInferenceContext | None) -> ToolGate:
    """Decide which authority governs the declared tool set for a request.

    A request carrying an ``evaluation_context`` bypasses the static
    ``supports_tools`` registry so a scored model is never pre-judged
    (INV-EVAL-CAMP-07). The decision reads only the context object: never an
    environment variable, capability probe, or recorded observation. Production
    chat (no evaluation context) keeps the registry gate.

    This is the one place the decision is made; ``AIToolService.get_tools``
    applies it and the evaluation trace records its result.
    """
    if evaluation_context is not None:
        return ToolGate.BYPASSED_FOR_EVAL
    return ToolGate.REGISTRY
