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

from __future__ import annotations

import time
from abc import ABC, abstractmethod
from collections.abc import AsyncGenerator, AsyncIterable, Iterable
from contextvars import ContextVar
from typing import TYPE_CHECKING, TypeVar

from google.protobuf import json_format
from google.protobuf.message import Message

from app.llm.llm_types import (
    AssistantLLMSettings,
    Content,
    GenerateContentResponse,
    LiteLLMSettings,
    PrimaryLLMSettings,
    StreamChunkFromModel,
)
from app.llm.model_evidence import model_boundary_privacy_attestation
from app.models.base import BaseModel
from app.models.model_telemetry import ModelBoundaryPrivacyAttestation, ModelResponseArtifact

TResponse = TypeVar("TResponse")

if TYPE_CHECKING:
    from app.models.http_context import G8eHttpContext


class LLMProvider(ABC):
    """Abstract base class for LLM providers."""

    def __init__(self):
        self._is_cached_singleton = False
        self._input_artifact_hash: ContextVar[str] = ContextVar(
            f"{type(self).__name__}_input_artifact_hash_{id(self)}", default=""
        )
        self._model_boundary_privacy: ContextVar[ModelBoundaryPrivacyAttestation | None] = (
            ContextVar(f"{type(self).__name__}_model_boundary_privacy_{id(self)}", default=None)
        )
        self._declared_tool_names: ContextVar[tuple[str, ...] | None] = ContextVar(
            f"{type(self).__name__}_declared_tool_names_{id(self)}", default=None
        )
        self._response_artifact: ContextVar[ModelResponseArtifact | None] = ContextVar(
            f"{type(self).__name__}_response_artifact_{id(self)}", default=None
        )
        self._response_received_at: ContextVar[tuple[float, ...]] = ContextVar(
            f"{type(self).__name__}_response_received_at_{id(self)}", default=()
        )

    @property
    def response_artifact(self) -> ModelResponseArtifact | None:
        artifact = self._response_artifact.get()
        return artifact.model_copy(deep=True) if artifact is not None else None

    def record_processed_response(self, response: str) -> None:
        artifact = self._response_artifact.get()
        if artifact is not None:
            artifact.processed_response = response

    def record_processing_error(self, error: Exception) -> None:
        artifact = self._response_artifact.get()
        if artifact is not None:
            artifact.processing_error = type(error).__name__

    def _record_response(self, response: object, *, complete: bool = False) -> None:
        """Capture provider JSON once, before any application interpretation."""
        artifact = self._response_artifact.get()
        if artifact is None:
            artifact = ModelResponseArtifact()
            self._response_artifact.set(artifact)
            self._response_received_at.set(())
        self._response_received_at.set((*self._response_received_at.get(), time.monotonic()))
        if isinstance(response, BaseModel):
            artifact.raw_frames.append(response.model_dump_json())
        elif isinstance(response, Message):
            artifact.raw_frames.append(json_format.MessageToJson(response))
        artifact.received_complete = complete

    async def _receive_stream(self, stream: AsyncIterable[TResponse]) -> list[TResponse]:
        """Finish receipt before parsing; a parser cannot truncate the raw output."""
        responses = []
        async for response in stream:
            self._record_response(response)
            responses.append(response)
        artifact = self._response_artifact.get()
        if artifact is not None:
            artifact.received_complete = True
        return responses

    @property
    def input_artifact_hash(self) -> str:
        return self._input_artifact_hash.get()

    @property
    def model_boundary_privacy(self) -> ModelBoundaryPrivacyAttestation | None:
        return self._model_boundary_privacy.get()

    @property
    def declared_tool_names(self) -> list[str] | None:
        """Tool names as sent on the most recent provider call, in order.

        ``None`` means no call has reported its declarations (a provider that
        does not record them, or the evidence was cleared); ``[]`` means the
        call genuinely declared no tools.
        """
        names = self._declared_tool_names.get()
        return list(names) if names is not None else None

    def clear_input_artifact_hash(self) -> None:
        self._input_artifact_hash.set("")
        self._model_boundary_privacy.set(None)
        self._response_artifact.set(None)
        self._response_received_at.set(())

    def clear_declared_tools(self) -> None:
        self._declared_tool_names.set(None)

    def mark_cached_singleton(self) -> None:
        """Mark this provider as owned by the process-wide provider cache."""
        self._is_cached_singleton = True

    def _record_declared_tools(self, names: Iterable[str]) -> None:
        """Record the tool names crossing the provider boundary on this call."""
        self._declared_tool_names.set(tuple(names))

    def set_g8e_context(self, context: G8eHttpContext | None) -> None:  # noqa: B027
        """Record the turn's HTTP context for providers that need it.

        No-op on the base class. Providers that propagate application
        context into their requests (e.g. the governed-dispatch provider)
        override this to store the context on a request-scoped ContextVar.
        """

    def set_provider_retry_count(self, retry_count: int) -> None:  # noqa: B027
        """Record the zero-based retry ordinal for the next provider call."""

    def _record_model_boundary(self, payload: object) -> str:
        self._response_artifact.set(ModelResponseArtifact())
        self._response_received_at.set(())
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
        if False:
            yield StreamChunkFromModel()

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
        if False:
            yield StreamChunkFromModel()

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
        if False:
            yield StreamChunkFromModel()

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
