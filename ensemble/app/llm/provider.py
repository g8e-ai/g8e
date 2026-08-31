# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Abstract LLM Provider Interface

All provider implementations must implement this interface.
"""

from abc import ABC, abstractmethod
from collections.abc import AsyncGenerator
from contextvars import ContextVar

from app.llm.llm_types import (
    AssistantLLMSettings,
    Content,
    GenerateContentResponse,
    LiteLLMSettings,
    PrimaryLLMSettings,
    StreamChunkFromModel,
)
from app.llm.model_evidence import model_boundary_privacy_attestation
from app.models.model_telemetry import ModelBoundaryPrivacyAttestation


class LLMProvider(ABC):
    """Abstract base class for LLM providers."""

    def __init__(self):
        self._is_cached_singleton = False
        self._input_artifact_hash: ContextVar[str] = ContextVar(
            f"{type(self).__name__}_input_artifact_hash_{id(self)}", default=""
        )
        self._model_boundary_privacy: ContextVar[ModelBoundaryPrivacyAttestation | None] = ContextVar(
            f"{type(self).__name__}_model_boundary_privacy_{id(self)}", default=None
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
    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from the primary LLM (agent main loop)."""
        yield

    @abstractmethod
    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from the primary LLM."""
        raise NotImplementedError(
            "LLMProvider.generate_content_primary must be implemented by subclasses"
        )

    @abstractmethod
    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from the assistant LLM (analysis, memory, title)."""
        yield

    @abstractmethod
    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from the assistant LLM."""
        raise NotImplementedError(
            "LLMProvider.generate_content_assistant must be implemented by subclasses"
        )

    @abstractmethod
    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from the lite LLM (triage, eval)."""
        yield

    @abstractmethod
    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from the lite LLM."""
        raise NotImplementedError(
            "LLMProvider.generate_content_lite must be implemented by subclasses"
        )

    async def close(self):
        """Clean up provider resources (e.g., close HTTP clients).

        For cached singleton providers, this is a no-op to allow reuse.
        Use force_close() to actually close a cached provider.
        """
        if not self._is_cached_singleton:
            await self._close_resources()

    async def _close_resources(self):  # noqa: B027
        """Internal method to actually close resources. Override in subclasses."""

    async def force_close(self):
        """Force close provider resources even if it's a cached singleton."""
        await self._close_resources()

    async def __aenter__(self):
        """Async context manager entry."""
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Async context manager exit - ensures cleanup for non-cached providers."""
        await self.close()
        return False

    @staticmethod
    @abstractmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        """Validate provider-specific configuration requirements.

        Returns a list of error messages. Empty list indicates valid configuration.

        Args:
            api_key: The API key for the provider (if applicable)
            endpoint: The endpoint URL for the provider (if applicable)

        Returns:
            List of validation error messages. Empty if configuration is valid.
        """
