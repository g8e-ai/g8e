# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Regression tests for the Tribunal command generator."""

import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.constants import (
    CommandGenerationOutcome,
    LLMProvider,
    TribunalFallbackReason,
    TribunalMember,
    TRIBUNAL_MEMBER_TEMPERATURES,
    OLLAMA_DEFAULT_MODEL,
    OPENAI_DEFAULT_MODEL,
    ANTHROPIC_DEFAULT_MODEL,
    GEMINI_DEFAULT_MODEL,
)
from app.llm.llm_types import Content, Role
from app.models.settings import LLMSettings, G8eeUserSettings
from app.models.agents.tribunal import (
    TribunalFallbackPayload,
    TribunalGenerationFailedError,
    TribunalProviderUnavailableError,
    TribunalSessionStartedPayload,
    TribunalSystemError,
    TribunalVerifierFailedError,
)
from app.services.ai.command_generator import (
    _build_and_emit_result,
    _is_system_error,
    _MAX_TOKENS_GENERATION,
    _MAX_TOKENS_VERIFIER,
    _member_for_pass,
    _resolve_model,
    _run_generation_pass,
    _run_generation_stage,
    _run_verification_stage,
    _run_verifier,
    _run_voting_stage,
    _temperature_for_pass,
    generate_command,
    TribunalEmitter,
)


def _make_mock_provider(generate_content_lite_side_effect=None, generate_content_lite_return=None):
    """Create a mock LLM provider that supports async context manager protocol."""
    mock_provider = MagicMock()
    if generate_content_lite_side_effect is not None:
        mock_provider.generate_content_lite = AsyncMock(side_effect=generate_content_lite_side_effect)
    elif generate_content_lite_return is not None:
        mock_provider.generate_content_lite = AsyncMock(return_value=generate_content_lite_return)
    mock_provider.__aenter__ = AsyncMock(return_value=mock_provider)
    mock_provider.__aexit__ = AsyncMock(return_value=False)
    return mock_provider


class TestResolveModel:
    """_resolve_model returns a concrete model string with proper fallback chain."""

    def test_returns_assistant_model_when_set(self):
        llm = LLMSettings(assistant_model="custom-assistant")
        assert _resolve_model(llm) == "custom-assistant"

    def test_falls_back_to_primary_model_when_assistant_is_none(self):
        llm = LLMSettings(primary_model="custom-primary")
        assert llm.assistant_model is None
        assert _resolve_model(llm) == "custom-primary"

    def test_falls_back_to_provider_default_when_both_none(self):
        llm = LLMSettings(provider=LLMProvider.OLLAMA)
        assert llm.assistant_model is None
        assert llm.primary_model is None
        assert _resolve_model(llm) == OLLAMA_DEFAULT_MODEL

    def test_provider_default_openai(self):
        llm = LLMSettings(provider=LLMProvider.OPENAI)
        assert _resolve_model(llm) == OPENAI_DEFAULT_MODEL

    def test_provider_default_anthropic(self):
        llm = LLMSettings(provider=LLMProvider.ANTHROPIC)
        assert _resolve_model(llm) == ANTHROPIC_DEFAULT_MODEL

    def test_provider_default_gemini(self):
        llm = LLMSettings(provider=LLMProvider.GEMINI)
        assert _resolve_model(llm) == GEMINI_DEFAULT_MODEL

    def test_assistant_takes_priority_over_primary(self):
        llm = LLMSettings(primary_model="primary", assistant_model="assistant")
        assert _resolve_model(llm) == "assistant"

    def test_result_is_always_str_never_none(self):
        for provider in LLMProvider:
            llm = LLMSettings(provider=provider)
            result = _resolve_model(llm)
            assert isinstance(result, str)
            assert len(result) > 0


class TestTribunalSessionStartedPayloadRegression:
    """TribunalSessionStartedPayload must never receive None for model."""

    def test_payload_rejects_none_model(self):
        with pytest.raises(Exception):
            TribunalSessionStartedPayload(
                original_command="ls",
                model=None,
                num_passes=3,
                members=[],
                os_name="linux",
                shell="bash",
            )

    def test_payload_accepts_resolved_model(self):
        llm = LLMSettings(provider=LLMProvider.OLLAMA)
        model = _resolve_model(llm)
        payload = TribunalSessionStartedPayload(
            original_command="ls",
            model=model,
            num_passes=3,
            members=[],
            os_name="linux",
            shell="bash",
        )
        assert payload.model == OLLAMA_DEFAULT_MODEL


class TestRoleImportRegression:
    """Regression: command_generator must use Role.USER, not types.Role.USER.

    Before the fix, both _run_generation_pass and _run_verifier referenced
    `types.Role.USER` without importing `types`, causing NameError on every
    Tribunal pass and silently falling back to the original command.
    """

    def test_role_user_is_importable_from_llm_types(self):
        assert Role.USER == "user"

    @pytest.mark.asyncio
    async def test_generation_pass_uses_role_user_in_content(self):
        mock_response = MagicMock()
        mock_response.text = "ls -la"

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            intent="list files",
            original_command="ls",
            os_name="linux",
            shell="bash",
            working_directory="/tmp",
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
        )

        assert result == "ls -la"
        assert pass_errors == []
        call_kwargs = mock_provider.generate_content_lite.call_args
        contents = call_kwargs.kwargs.get("contents") or call_kwargs[1].get("contents")
        assert len(contents) == 1
        assert contents[0].role == Role.USER

    @pytest.mark.asyncio
    async def test_verifier_uses_role_user_in_content(self):
        mock_response = MagicMock()
        mock_response.text = "ok"

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)

        passed, revision = await _run_verifier(
            provider=mock_provider,
            model="test-model",
            intent="list files",
            candidate_command="ls -la",
            os_name="linux",
            emitter=emitter,
        )

        assert passed is True
        assert revision is None
        call_kwargs = mock_provider.generate_content_lite.call_args
        contents = call_kwargs.kwargs.get("contents") or call_kwargs[1].get("contents")
        assert len(contents) == 1
        assert contents[0].role == Role.USER


class TestIsSystemError:
    """_is_system_error classifies error messages into system vs. model errors."""

    def test_auth_errors(self):
        assert _is_system_error("401 Unauthorized")
        assert _is_system_error("403 Forbidden")
        assert _is_system_error("Invalid API key provided")
        assert _is_system_error("Authentication failed for endpoint")

    def test_network_errors(self):
        assert _is_system_error("Connection refused")
        assert _is_system_error("ConnectionError: cannot reach host")
        assert _is_system_error("Timeout waiting for response")
        assert _is_system_error("DNS name resolution failed")
        assert _is_system_error("SSL certificate verify failed")
        assert _is_system_error("ECONNREFUSED 127.0.0.1:11434")

    def test_config_errors(self):
        assert _is_system_error("Unsupported LLM provider: foo")

    def test_model_errors_are_not_system(self):
        assert not _is_system_error("Model returned empty response")
        assert not _is_system_error("Invalid JSON in response")
        assert not _is_system_error("Unexpected response format")
        assert not _is_system_error("Content filter triggered")

    def test_empty_string_is_not_system(self):
        assert not _is_system_error("")


class TestTribunalSystemError:
    """TribunalSystemError carries pass_errors and original_command."""

    def test_attributes(self):
        errors = ["401 Unauthorized", "Connection refused"]
        exc = TribunalSystemError(pass_errors=errors, original_command="ls -la")
        assert exc.pass_errors == errors
        assert exc.original_command == "ls -la"
        assert "401 Unauthorized" in str(exc)

    def test_is_exception(self):
        exc = TribunalSystemError(pass_errors=["err"], original_command="ls")
        assert isinstance(exc, Exception)


class TestPassErrorsCollection:
    """_run_generation_pass appends errors to the pass_errors list."""

    @pytest.mark.asyncio
    async def test_exception_appends_to_pass_errors(self):
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(
            side_effect=RuntimeError("Connection refused")
        )
        emitter = TribunalEmitter(None, None)
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            intent="list files",
            original_command="ls",
            os_name="linux",
            shell="bash",
            working_directory="/tmp",
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
        )

        assert result is None
        assert len(pass_errors) == 1
        assert "Connection refused" in pass_errors[0]

    @pytest.mark.asyncio
    async def test_empty_response_appends_to_pass_errors(self):
        mock_response = MagicMock()
        mock_response.text = ""

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            intent="list files",
            original_command="ls",
            os_name="linux",
            shell="bash",
            working_directory="/tmp",
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
        )

        assert result is None
        assert len(pass_errors) == 1
        assert "empty response" in pass_errors[0]

    @pytest.mark.asyncio
    async def test_success_does_not_append_to_pass_errors(self):
        mock_response = MagicMock()
        mock_response.text = "ls -la"

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            intent="list files",
            original_command="ls",
            os_name="linux",
            shell="bash",
            working_directory="/tmp",
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
        )

        assert result == "ls -la"
        assert pass_errors == []


class TestTribunalFallbackPayloadPassErrors:
    """TribunalFallbackPayload supports the pass_errors field."""

    def test_accepts_pass_errors(self):
        payload = TribunalFallbackPayload(
            reason=TribunalFallbackReason.ALL_PASSES_FAILED,
            original_command="ls",
            final_command="ls",
            pass_errors=["err1", "err2"],
        )
        assert payload.pass_errors == ["err1", "err2"]

    def test_defaults_to_none(self):
        payload = TribunalFallbackPayload(
            reason=TribunalFallbackReason.DISABLED,
            original_command="ls",
            final_command="ls",
        )
        assert payload.pass_errors is None


class TestGenerateCommandOutcomes:
    """End-to-end outcomes for generate_command."""

    @pytest.mark.asyncio
    async def test_returns_disabled_outcome_when_tribunal_is_not_enabled(self):
        llm = LLMSettings(llm_command_gen_enabled=False)
        settings = G8eeUserSettings(llm=llm)

        result = await generate_command(
            original_command="ls",
            intent="list files",
            os_name="linux",
            shell="bash",
            working_directory="/tmp",
            g8ed_event_service=AsyncMock(),
            web_session_id="ws-1",
            user_id="user-1",
            case_id="case-1",
            investigation_id="inv-1",
            settings=settings,
        )

        assert result.final_command == "ls"
        assert result.outcome == CommandGenerationOutcome.DISABLED


class TestGenerateCommandSystemError:
    """generate_command raises TribunalSystemError on all-system-error passes."""

    @pytest.mark.asyncio
    async def test_raises_on_all_system_errors(self):
        llm = LLMSettings(
            provider=LLMProvider.OLLAMA,
            assistant_model="gemma3:1b",
        )
        settings = G8eeUserSettings(llm=llm)

        mock_provider = _make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("401 Unauthorized")
        )

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalSystemError) as exc_info:
                await generate_command(
                    original_command="ls",
                    intent="list files",
                    os_name="linux",
                    shell="bash",
                    working_directory="/tmp",
                    g8ed_event_service=MagicMock(),
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                )

            assert len(exc_info.value.pass_errors) > 0
            assert all("401" in e for e in exc_info.value.pass_errors)

    @pytest.mark.asyncio
    async def test_raises_generation_failed_error_on_non_system_errors(self):
        """Non-system errors now raise TribunalGenerationFailedError instead of silent fallback."""
        llm = LLMSettings(
            provider=LLMProvider.OLLAMA,
            assistant_model="gemma3:1b",
        )
        settings = G8eeUserSettings(llm=llm)

        mock_provider = _make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("Model returned gibberish")
        )

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalGenerationFailedError) as exc_info:
                await generate_command(
                    original_command="ls",
                    intent="list files",
                    os_name="linux",
                    shell="bash",
                    working_directory="/tmp",
                    g8ed_event_service=MagicMock(),
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                )

            assert exc_info.value.original_command == "ls"
            assert len(exc_info.value.pass_errors) > 0

    @pytest.mark.asyncio
    async def test_provider_routing_uses_settings_provider(self):
        llm = LLMSettings(
            provider=LLMProvider.GEMINI,
            assistant_model="gemma3:1b",
        )
        settings = G8eeUserSettings(llm=llm)

        call_count = 0

        async def mock_generate_content_lite(**kwargs):
            nonlocal call_count
            call_count += 1
            mock_response = MagicMock()
            if call_count == 1:
                mock_response.text = "ls -la"
            else:
                mock_response.text = "ok"
            return mock_response

        mock_provider = _make_mock_provider(generate_content_lite_side_effect=mock_generate_content_lite)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ) as mock_factory:
            result = await generate_command(
                original_command="ls",
                intent="list files",
                os_name="linux",
                shell="bash",
                working_directory="/tmp",
                g8ed_event_service=MagicMock(),
                web_session_id="ws-1",
                user_id="user-1",
                case_id="case-1",
                investigation_id="inv-1",
                settings=settings,
            )

            mock_factory.assert_called_once_with(llm)
            assert result.final_command == "ls -la"


class TestMixedErrorFallback:
    """Mixed system + non-system errors raise TribunalGenerationFailedError, not TribunalSystemError."""

    @pytest.mark.asyncio
    async def test_mixed_errors_raise_generation_failed_error(self):
        """1 system error + 2 non-system errors must raise TribunalGenerationFailedError."""
        llm = LLMSettings(
            provider=LLMProvider.OLLAMA,
            assistant_model="gemma3:1b",
            llm_command_gen_passes=3,
        )
        settings = G8eeUserSettings(llm=llm)

        call_count = 0

        async def mixed_side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                raise RuntimeError("401 Unauthorized")
            raise RuntimeError("Model returned gibberish")

        mock_provider = _make_mock_provider(generate_content_lite_side_effect=mixed_side_effect)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalGenerationFailedError) as exc_info:
                await generate_command(
                    original_command="ls",
                    intent="list files",
                    os_name="linux",
                    shell="bash",
                    working_directory="/tmp",
                    g8ed_event_service=MagicMock(),
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                )

            assert exc_info.value.original_command == "ls"
            assert len(exc_info.value.pass_errors) == 3


class TestNewEnumValues:
    """New enum values exist and are distinct from existing ones."""

    def test_command_generation_outcome_disabled(self):
        assert CommandGenerationOutcome.DISABLED == "disabled"
        assert CommandGenerationOutcome.DISABLED != CommandGenerationOutcome.FALLBACK

    def test_command_generation_outcome_system_error(self):
        assert CommandGenerationOutcome.SYSTEM_ERROR == "system_error"
        assert CommandGenerationOutcome.SYSTEM_ERROR != CommandGenerationOutcome.FALLBACK

    def test_tribunal_fallback_reason_system_error(self):
        assert TribunalFallbackReason.SYSTEM_ERROR == "system_error"
        assert TribunalFallbackReason.SYSTEM_ERROR != TribunalFallbackReason.ALL_PASSES_FAILED


class TestTribunalProviderUnavailableError:
    """TribunalProviderUnavailableError raised when provider cannot be initialized."""

    @pytest.mark.asyncio
    async def test_raises_on_provider_init_failure(self):
        """Provider init failure raises TribunalProviderUnavailableError instead of silent fallback."""
        llm = LLMSettings(
            provider=LLMProvider.OLLAMA,
            assistant_model="gemma3:1b",
        )
        settings = G8eeUserSettings(llm=llm)

        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            side_effect=RuntimeError("Unsupported LLM provider: foo"),
        ):
            with pytest.raises(TribunalProviderUnavailableError) as exc_info:
                await generate_command(
                    original_command="ls",
                    intent="list files",
                    os_name="linux",
                    shell="bash",
                    working_directory="/tmp",
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                )

            assert exc_info.value.provider == "ollama"
            assert exc_info.value.original_command == "ls"
            assert "Unsupported LLM provider" in exc_info.value.error


class TestTribunalVerifierFailedError:
    """TribunalVerifierFailedError raised when verifier fails and cannot validate candidate."""

    @pytest.mark.asyncio
    async def test_raises_on_empty_verifier_response(self):
        """Empty verifier response raises TribunalVerifierFailedError instead of treating as passed."""
        mock_response = MagicMock()
        mock_response.text = None

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)

        with pytest.raises(TribunalVerifierFailedError) as exc_info:
            await _run_verifier(
                provider=mock_provider,
                model="test-model",
                intent="list files",
                candidate_command="ls -la",
                os_name="linux",
                emitter=emitter,
            )

        assert exc_info.value.reason == "empty_response"
        assert exc_info.value.original_command == "ls -la"

    @pytest.mark.asyncio
    async def test_raises_on_no_valid_revision(self):
        """Non-ok answer without valid revision raises TribunalVerifierFailedError instead of treating as passed."""
        mock_response = MagicMock()
        # Response that's not "ok" and normalizes to the same command (not a valid revision)
        mock_response.text = "ls -la"

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)

        with pytest.raises(TribunalVerifierFailedError) as exc_info:
            await _run_verifier(
                provider=mock_provider,
                model="test-model",
                intent="list files",
                candidate_command="ls -la",
                os_name="linux",
                emitter=emitter,
            )

        assert exc_info.value.reason == "no_valid_revision"
        assert exc_info.value.original_command == "ls -la"

    @pytest.mark.asyncio
    async def test_raises_on_verifier_exception(self):
        """Verifier exception raises TribunalVerifierFailedError instead of treating as passed."""
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(
            side_effect=RuntimeError("Verifier API timeout")
        )
        emitter = TribunalEmitter(None, None)

        with pytest.raises(TribunalVerifierFailedError) as exc_info:
            await _run_verifier(
                provider=mock_provider,
                model="test-model",
                intent="list files",
                candidate_command="ls -la",
                os_name="linux",
                emitter=emitter,
            )

        assert exc_info.value.reason == "exception"
        assert "timeout" in exc_info.value.error
        assert exc_info.value.original_command == "ls -la"


class TestRunGenerationStage:
    """_run_generation_stage returns candidates or raises on total failure."""

    @pytest.mark.asyncio
    async def test_returns_candidates_on_success(self):
        mock_response = MagicMock()
        mock_response.text = "ls -la"
        mock_provider = _make_mock_provider(generate_content_lite_return=mock_response)
        emitter = TribunalEmitter(None, None)

        candidates = await _run_generation_stage(
            provider=mock_provider, model="test-model", intent="list files",
            original_command="ls", os_name="linux", shell="bash",
            working_directory="/tmp", num_passes=3, emitter=emitter,
        )

        assert len(candidates) == 3
        assert all(c.command == "ls -la" for c in candidates)

    @pytest.mark.asyncio
    async def test_raises_system_error_on_all_system_failures(self):
        mock_provider = _make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("401 Unauthorized")
        )
        emitter = TribunalEmitter(None, None)

        with pytest.raises(TribunalSystemError):
            await _run_generation_stage(
                provider=mock_provider, model="test-model", intent="list files",
                original_command="ls", os_name="linux", shell="bash",
                working_directory="/tmp", num_passes=3, emitter=emitter,
            )

    @pytest.mark.asyncio
    async def test_raises_generation_failed_on_non_system_failures(self):
        mock_provider = _make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("Model returned gibberish")
        )
        emitter = TribunalEmitter(None, None)

        with pytest.raises(TribunalGenerationFailedError):
            await _run_generation_stage(
                provider=mock_provider, model="test-model", intent="list files",
                original_command="ls", os_name="linux", shell="bash",
                working_directory="/tmp", num_passes=2, emitter=emitter,
            )

    @pytest.mark.asyncio
    async def test_partial_failures_return_successful_candidates(self):
        call_count = 0

        async def partial_side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 2:
                raise RuntimeError("Model failed")
            mock_resp = MagicMock()
            mock_resp.text = "ls -la"
            return mock_resp

        mock_provider = _make_mock_provider(generate_content_lite_side_effect=partial_side_effect)
        emitter = TribunalEmitter(None, None)

        candidates = await _run_generation_stage(
            provider=mock_provider, model="test-model", intent="list files",
            original_command="ls", os_name="linux", shell="bash",
            working_directory="/tmp", num_passes=3, emitter=emitter,
        )

        assert len(candidates) == 2


class TestRunVotingStage:
    """_run_voting_stage computes weighted vote and emits consensus event."""

    @pytest.mark.asyncio
    async def test_returns_winner_and_score(self):
        from app.models.agents.tribunal import CandidateCommand

        candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM),
            CandidateCommand(command="ls -la", pass_index=1, member=TribunalMember.CONCORD),
            CandidateCommand(command="ls -l", pass_index=2, member=TribunalMember.VARIANCE),
        ]
        emitter = TribunalEmitter(None, None)

        winner, score = await _run_voting_stage(
            candidates=candidates, original_command="ls", emitter=emitter,
        )

        assert winner == "ls -la"
        assert score > 0.5

    @pytest.mark.asyncio
    async def test_single_candidate_wins(self):
        from app.models.agents.tribunal import CandidateCommand

        candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM),
        ]
        emitter = TribunalEmitter(None, None)

        winner, score = await _run_voting_stage(
            candidates=candidates, original_command="ls", emitter=emitter,
        )

        assert winner == "ls -la"
        assert score == 1.0


class TestRunVerificationStage:
    """_run_verification_stage determines final command and outcome."""

    @pytest.mark.asyncio
    async def test_verifier_disabled_returns_consensus(self):
        final_cmd, outcome, passed, revision = await _run_verification_stage(
            provider=MagicMock(), model="test-model", intent="list files",
            vote_winner="ls -la", os_name="linux", verifier_enabled=False,
            emitter=TribunalEmitter(None, None),
        )

        assert final_cmd == "ls -la"
        assert outcome == CommandGenerationOutcome.CONSENSUS
        assert passed is True
        assert revision is None

    @pytest.mark.asyncio
    async def test_verifier_approves_returns_verified(self):
        mock_response = MagicMock()
        mock_response.text = "ok"
        mock_provider = _make_mock_provider(generate_content_lite_return=mock_response)

        final_cmd, outcome, passed, revision = await _run_verification_stage(
            provider=mock_provider, model="test-model", intent="list files",
            vote_winner="ls -la", os_name="linux", verifier_enabled=True,
            emitter=TribunalEmitter(None, None),
        )

        assert final_cmd == "ls -la"
        assert outcome == CommandGenerationOutcome.VERIFIED
        assert passed is True
        assert revision is None

    @pytest.mark.asyncio
    async def test_verifier_revision_returns_verification_failed(self):
        mock_response = MagicMock()
        mock_response.text = "ls -la --color=auto"
        mock_provider = _make_mock_provider(generate_content_lite_return=mock_response)

        final_cmd, outcome, passed, revision = await _run_verification_stage(
            provider=mock_provider, model="test-model", intent="list files",
            vote_winner="ls -la", os_name="linux", verifier_enabled=True,
            emitter=TribunalEmitter(None, None),
        )

        assert final_cmd == "ls -la --color=auto"
        assert outcome == CommandGenerationOutcome.VERIFICATION_FAILED
        assert passed is False
        assert revision == "ls -la --color=auto"

    @pytest.mark.asyncio
    async def test_verifier_exception_raises_verifier_failed_error(self):
        mock_provider = _make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("timeout")
        )

        with pytest.raises(TribunalVerifierFailedError):
            await _run_verification_stage(
                provider=mock_provider, model="test-model", intent="list files",
                vote_winner="ls -la", os_name="linux", verifier_enabled=True,
                emitter=TribunalEmitter(None, None),
            )


class TestBuildAndEmitResult:
    """_build_and_emit_result assembles the result model correctly."""

    @pytest.mark.asyncio
    async def test_builds_complete_result(self):
        from app.models.agents.tribunal import CandidateCommand

        candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM),
        ]
        emitter = TribunalEmitter(None, None)

        result = await _build_and_emit_result(
            original_command="ls", final_command="ls -la",
            outcome=CommandGenerationOutcome.VERIFIED, candidates=candidates,
            vote_winner="ls -la", vote_score=1.0, verifier_passed=True,
            verifier_revision=None, emitter=emitter,
        )

        assert result.original_command == "ls"
        assert result.final_command == "ls -la"
        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.vote_winner == "ls -la"
        assert result.vote_score == 1.0
        assert result.verifier_passed is True
        assert result.verifier_revision is None


class TestMaxTokensConstants:
    """_MAX_TOKENS constants are used by generation passes and verifier."""

    @pytest.mark.asyncio
    async def test_generation_pass_uses_max_tokens(self):
        mock_response = MagicMock()
        mock_response.text = "ls -la"
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)

        await _run_generation_pass(
            provider=mock_provider, model="test-model", intent="list files",
            original_command="ls", os_name="linux", shell="bash",
            working_directory="/tmp", pass_index=0, emitter=emitter,
            pass_errors=[],
        )

        call_kwargs = mock_provider.generate_content_lite.call_args
        settings = call_kwargs.kwargs.get("lite_llm_settings")
        assert settings.max_output_tokens == _MAX_TOKENS_GENERATION

    @pytest.mark.asyncio
    async def test_verifier_uses_max_tokens_and_zero_temperature(self):
        mock_response = MagicMock()
        mock_response.text = "ok"
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)

        await _run_verifier(
            provider=mock_provider, model="test-model", intent="list files",
            candidate_command="ls -la", os_name="linux", emitter=emitter,
        )

        call_kwargs = mock_provider.generate_content_lite.call_args
        settings = call_kwargs.kwargs.get("lite_llm_settings")
        assert settings.max_output_tokens == _MAX_TOKENS_VERIFIER
        assert settings.temperature == 0.0


class TestTribunalMemberCycling:
    """Tribunal member assignment cycles correctly through AXIOM, CONCORD, VARIANCE."""

    def test_member_for_pass_cycles_correctly(self):
        """_member_for_pass returns members in order: AXIOM (0), CONCORD (1), VARIANCE (2), then repeats."""
        assert _member_for_pass(0) == TribunalMember.AXIOM
        assert _member_for_pass(1) == TribunalMember.CONCORD
        assert _member_for_pass(2) == TribunalMember.VARIANCE
        assert _member_for_pass(3) == TribunalMember.AXIOM
        assert _member_for_pass(4) == TribunalMember.CONCORD
        assert _member_for_pass(5) == TribunalMember.VARIANCE

    def test_temperature_for_pass_returns_canonical_values(self):
        """_temperature_for_pass returns canonical temperatures for each member."""
        assert _temperature_for_pass(0) == 0.0  # AXIOM
        assert _temperature_for_pass(1) == 0.4  # CONCORD
        assert _temperature_for_pass(2) == 0.8  # VARIANCE
        assert _temperature_for_pass(3) == 0.0  # AXIOM (cycles)
        assert _temperature_for_pass(4) == 0.4  # CONCORD (cycles)

    def test_temperatures_match_shared_constants(self):
        """Temperature values match the canonical TRIBUNAL_MEMBER_TEMPERATURES dict."""
        assert TRIBUNAL_MEMBER_TEMPERATURES[TribunalMember.AXIOM] == 0.0
        assert TRIBUNAL_MEMBER_TEMPERATURES[TribunalMember.CONCORD] == 0.4
        assert TRIBUNAL_MEMBER_TEMPERATURES[TribunalMember.VARIANCE] == 0.8
