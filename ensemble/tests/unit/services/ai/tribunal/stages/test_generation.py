# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock
import pytest
from app.llm.llm_types import Role
from app.models.agents.tribunal import (
    TribunalSystemError,
    TribunalGenerationFailedError,
)
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.generation import (
    _run_generation_pass,
    _run_generation_stage,
)


@pytest.mark.asyncio
class TestRunGenerationPass:
    async def test_generation_pass_uses_role_user_in_content(
        self, make_mock_provider, mock_g8e_context, mock_operator_context
    ):
        mock_response = MagicMock()
        mock_response.text = "ls -la"

        mock_provider = make_mock_provider(generate_content_lite_return=mock_response)
        emitter = TribunalEmitter(None, mock_g8e_context)
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            operator_context=mock_operator_context,
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        )

        assert result == "ls -la"
        assert pass_errors == []
        call_kwargs = mock_provider.generate_content_lite.call_args
        contents = call_kwargs.kwargs.get("contents") or call_kwargs[1].get("contents")
        assert len(contents) == 1
        assert contents[0].role == Role.USER

    async def test_generation_pass_emits_per_call_model_telemetry(
        self, make_mock_provider, mock_g8e_context, mock_operator_context
    ):
        mock_response = MagicMock()
        mock_response.text = "ls -la"
        mock_response.usage_metadata.prompt_token_count = 13
        mock_response.usage_metadata.candidates_token_count = 5
        mock_response.usage_metadata.thinking_token_count = 2
        mock_response.candidates = [MagicMock(finish_reason="stop")]
        mock_provider = make_mock_provider(generate_content_lite_return=mock_response)
        event_service = MagicMock()
        event_service.publish = AsyncMock()
        emitter = TribunalEmitter(event_service, mock_g8e_context)

        await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            operator_context=mock_operator_context,
            pass_index=0,
            emitter=emitter,
            pass_errors=[],
            command_constraints_message="No whitelist or blacklist constraints are active.",
        )

        event = event_service.publish.await_args.args[0]
        assert event.payload.model == "test-model"
        assert event.payload.input_tokens == 13
        assert event.payload.output_tokens == 5
        assert event.payload.thinking_tokens == 2
        assert len(event.payload.input_artifact_hash) == 64
        assert len(event.payload.output_artifact_hash) == 64
        assert event.payload.monotonic_end >= event.payload.monotonic_start

    async def test_exception_appends_to_pass_errors(
        self, make_mock_provider, mock_g8e_context, mock_operator_context
    ):
        mock_provider = make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("Connection refused")
        )
        emitter = TribunalEmitter(None, mock_g8e_context)
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            operator_context=mock_operator_context,
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        )

        assert result is None
        assert len(pass_errors) == 1
        assert "Connection refused" in pass_errors[0]

    async def test_empty_response_appends_to_pass_errors(
        self, make_mock_provider, mock_g8e_context, mock_operator_context
    ):
        mock_response = MagicMock()
        mock_response.text = ""

        mock_provider = make_mock_provider(generate_content_lite_return=mock_response)
        emitter = TribunalEmitter(None, mock_g8e_context)
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            operator_context=mock_operator_context,
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        )

        assert result is None
        assert len(pass_errors) == 1
        assert "empty response" in pass_errors[0]


@pytest.mark.asyncio
class TestRunGenerationStage:
    async def test_returns_candidates_on_success(
        self, make_mock_provider, mock_g8e_context, mock_operator_context
    ):
        mock_response = MagicMock()
        mock_response.text = "ls -la"
        mock_provider = make_mock_provider(generate_content_lite_return=mock_response)
        emitter = TribunalEmitter(None, mock_g8e_context)

        candidates = await _run_generation_stage(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            operator_context=mock_operator_context,
            num_passes=3,
            emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        )

        assert len(candidates) == 3
        assert all(c.command == "ls -la" for c in candidates)

    async def test_raises_system_error_on_all_system_failures(
        self, make_mock_provider, mock_g8e_context, mock_operator_context
    ):
        mock_provider = make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("401 Unauthorized")
        )
        emitter = TribunalEmitter(None, mock_g8e_context)

        with pytest.raises(TribunalSystemError):
            await _run_generation_stage(
                provider=mock_provider,
                model="test-model",
                request="list files",
                guidelines="",
                operator_context=mock_operator_context,
                num_passes=3,
                emitter=emitter,
                command_constraints_message="No whitelist or blacklist constraints are active.",
            )

    async def test_raises_generation_failed_on_non_system_failures(
        self, make_mock_provider, mock_g8e_context, mock_operator_context
    ):
        mock_provider = make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("Model returned gibberish")
        )
        emitter = TribunalEmitter(None, mock_g8e_context)

        with pytest.raises(TribunalGenerationFailedError):
            await _run_generation_stage(
                provider=mock_provider,
                model="test-model",
                request="list files",
                guidelines="",
                operator_context=mock_operator_context,
                num_passes=2,
                emitter=emitter,
                command_constraints_message="No whitelist or blacklist constraints are active.",
            )

    async def test_partial_failures_return_successful_candidates(
        self, make_mock_provider, mock_g8e_context, mock_operator_context
    ):
        call_count = 0

        async def partial_side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 2:
                raise RuntimeError("Model failed")
            mock_resp = MagicMock()
            mock_resp.text = "ls -la"
            return mock_resp

        mock_provider = make_mock_provider(generate_content_lite_side_effect=partial_side_effect)
        emitter = TribunalEmitter(None, mock_g8e_context)

        candidates = await _run_generation_stage(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            operator_context=mock_operator_context,
            num_passes=3,
            emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        )

        assert len(candidates) == 2
