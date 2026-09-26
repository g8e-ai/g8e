# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from abc import ABC, abstractmethod
from contextvars import ContextVar

from app.decision.types import DecisionState, EvaluateResponse, Question
from app.llm.model_evidence import model_boundary_privacy_attestation
from app.models.model_telemetry import ModelBoundaryPrivacyAttestation


class DecisionProvider(ABC):
    """Abstract base class for structured decision providers."""

    def __init__(self) -> None:
        self._is_cached_singleton = False
        self._input_artifact_hash: ContextVar[str] = ContextVar(
            f"{type(self).__name__}_input_artifact_hash_{id(self)}", default=""
        )
        self._model_boundary_privacy: ContextVar[ModelBoundaryPrivacyAttestation | None] = (
            ContextVar(
                f"{type(self).__name__}_model_boundary_privacy_{id(self)}",
                default=None,
            )
        )

    @property
    def input_artifact_hash(self) -> str:
        return self._input_artifact_hash.get()

    @property
    def model_boundary_privacy(self) -> ModelBoundaryPrivacyAttestation | None:
        return self._model_boundary_privacy.get()

    def clear_input_artifact_hash(self) -> None:
        self._input_artifact_hash.set("")
        self._model_boundary_privacy.set(None)

    def _record_model_boundary(self, payload: object) -> str:
        attestation = model_boundary_privacy_attestation(payload)
        self._input_artifact_hash.set(attestation.input_artifact_hash)
        self._model_boundary_privacy.set(attestation)
        return attestation.input_artifact_hash

    @abstractmethod
    async def evaluate(
        self,
        *,
        model: str,
        state: DecisionState,
        questions: dict[str, Question],
    ) -> EvaluateResponse:
        """Evaluate typed questions against the given state."""

    @staticmethod
    @abstractmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        """Validate provider-specific configuration requirements."""

    async def _close_resources(self) -> None:  # noqa: B027
        """Close provider-specific resources."""

    async def close(self) -> None:
        """Close provider resources when not cached as a singleton."""
        if not self._is_cached_singleton:
            await self._close_resources()

    async def force_close(self) -> None:
        """Force close provider resources even if cached as a singleton."""
        await self._close_resources()

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.close()
        return False
