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
    OLLAMA_DEFAULT_MODEL,
    OPENAI_DEFAULT_MODEL,
    ANTHROPIC_DEFAULT_MODEL,
    GEMINI_DEFAULT_MODEL,
    EventType,
)
from app.llm.llm_types import Content, Role
from app.models.model_configs import LLMModelConfig
from app.models.settings import LLMSettings, G8eeUserSettings
from app.models.agent import OperatorContext
from app.models.agents.tribunal import (
    TribunalFallbackPayload,
    TribunalGenerationFailedError,
    TribunalModelNotConfiguredError,
    TribunalProviderUnavailableError,
    TribunalSessionStartedPayload,
    TribunalSystemError,
    TribunalVerifierFailedError,
)
from app.services.ai.command_generator import (
    _build_and_emit_result,
    _build_operator_context_string,
    _format_forbidden_patterns_message,
    _is_system_error,
    _MAX_TOKENS_GENERATION,
    _MAX_TOKENS_VERIFIER,
    _member_for_pass,
    _prompt_fields,
    _resolve_model,
    _run_generation_pass,
    _run_generation_stage,
    _run_verification_stage,
    _run_verifier,
    _run_voting_stage,
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


def _make_mock_operator_context(
    os="linux",
    shell="bash",
    username="testuser",
    uid=1000,
    working_directory="/home/testuser",
    hostname="testhost",
    architecture="x86_64",
) -> OperatorContext:
    """Create a mock OperatorContext for tests."""
    return OperatorContext(
        operator_id="test-operator",
        os=os,
        shell=shell,
        username=username,
        uid=uid,
        working_directory=working_directory,
        hostname=hostname,
        architecture=architecture,
    )


class TestResolveModel:
    """_resolve_model returns a concrete model string with proper fallback chain."""

    def test_returns_assistant_model_when_set(self):
        llm = LLMSettings(assistant_model="custom-assistant")
        assert _resolve_model(llm) == "custom-assistant"

    def test_falls_back_to_primary_model_when_assistant_is_none(self):
        llm = LLMSettings(primary_model="custom-primary")
        assert llm.assistant_model is None
        assert _resolve_model(llm) == "custom-primary"

    def test_raises_when_both_models_none(self):
        llm = LLMSettings(provider=LLMProvider.OLLAMA)
        assert llm.assistant_model is None
        assert llm.primary_model is None
        with pytest.raises(TribunalModelNotConfiguredError):
            _resolve_model(llm)

    def test_raises_for_openai_when_no_model_configured(self):
        llm = LLMSettings(provider=LLMProvider.OPENAI)
        with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
            _resolve_model(llm)
        assert exc_info.value.provider == "openai"

    def test_raises_for_anthropic_when_no_model_configured(self):
        llm = LLMSettings(provider=LLMProvider.ANTHROPIC)
        with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
            _resolve_model(llm)
        assert exc_info.value.provider == "anthropic"

    def test_raises_for_gemini_when_no_model_configured(self):
        llm = LLMSettings(provider=LLMProvider.GEMINI)
        with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
            _resolve_model(llm)
        assert exc_info.value.provider == "gemini"

    def test_assistant_takes_priority_over_primary(self):
        llm = LLMSettings(primary_model="primary", assistant_model="assistant")
        assert _resolve_model(llm) == "assistant"


class TestTribunalSessionStartedPayloadRegression:
    """TribunalSessionStartedPayload must never receive None for model."""

    def test_payload_rejects_none_model(self):
        with pytest.raises(Exception):
            TribunalSessionStartedPayload(
                original_command="ls",
                model=None,
                num_passes=3,
                members=[],
                )

    def test_payload_accepts_resolved_model(self):
        llm = LLMSettings(provider=LLMProvider.OLLAMA, assistant_model="gemma3:1b")
        model = _resolve_model(llm)
        payload = TribunalSessionStartedPayload(
            original_command="ls",
            model=model,
            num_passes=3,
            members=[],
        )
        assert payload.model == "gemma3:1b"


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
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user"),
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
            operator_context=_make_mock_operator_context(os="linux"),
            emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        
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
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user"),
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        
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
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user"),
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        
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
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user"),
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
            command_constraints_message="No whitelist or blacklist constraints are active.",
        
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
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/tmp", username="root", uid=0),
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
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
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
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
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
            primary_provider=LLMProvider.GEMINI,
            assistant_provider=LLMProvider.OLLAMA,
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
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=MagicMock(),
                web_session_id="ws-1",
                user_id="user-1",
                case_id="case-1",
                investigation_id="inv-1",
                settings=settings,
            )

            mock_factory.assert_called_once_with(settings.llm, is_assistant=True)
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
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
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
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
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


class TestTribunalModelNotConfiguredError:
    """TribunalModelNotConfiguredError raised when no model is configured."""

    @pytest.mark.asyncio
    async def test_raises_on_no_model_configured(self):
        """No model configured raises TribunalModelNotConfiguredError with fallback event."""
        llm = LLMSettings(
            provider=LLMProvider.OLLAMA,
        )
        settings = G8eeUserSettings(llm=llm)

        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
        ):
            with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
                await generate_command(
                    original_command="ls",
                    intent="list files",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                )

            assert exc_info.value.provider == "ollama"
            assert exc_info.value.original_command == "ls"
            assert "model not configured" in str(exc_info.value).lower()

            mock_event_service.publish.assert_called()
            call_args = mock_event_service.publish.call_args
            event = call_args[0][0]
            assert event.event_type == EventType.TRIBUNAL_SESSION_FALLBACK_TRIGGERED
            assert event.payload.reason == TribunalFallbackReason.NO_MODEL_CONFIGURED


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
            operator_context=_make_mock_operator_context(os="linux"),
            emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            
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
            operator_context=_make_mock_operator_context(os="linux"),
            emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            
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
            operator_context=_make_mock_operator_context(os="linux"),
            emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            
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
            original_command="ls",
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
            num_passes=3, emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
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
                original_command="ls",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                num_passes=3, emitter=emitter,
                command_constraints_message="No whitelist or blacklist constraints are active.",
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
                original_command="ls",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                num_passes=2, emitter=emitter,
                command_constraints_message="No whitelist or blacklist constraints are active.",
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
            original_command="ls",
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
            num_passes=3, emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
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
            CandidateCommand(command="ls -la", pass_index=3, member=TribunalMember.PRAGMA),
            CandidateCommand(command="ls -l", pass_index=4, member=TribunalMember.NEMESIS),
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
            vote_winner="ls -la", operator_context=_make_mock_operator_context(os="linux", username="user", uid=1000),
            verifier_enabled=False, emitter=TribunalEmitter(None, None),
            command_constraints_message="No whitelist or blacklist constraints are active.",
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
            vote_winner="ls -la", operator_context=_make_mock_operator_context(os="linux", username="user", uid=1000),
            verifier_enabled=True, emitter=TribunalEmitter(None, None),
            command_constraints_message="No whitelist or blacklist constraints are active.",
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
            vote_winner="ls -la", operator_context=_make_mock_operator_context(os="linux", username="user", uid=1000),
            verifier_enabled=True, emitter=TribunalEmitter(None, None),
            command_constraints_message="No whitelist or blacklist constraints are active.",
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
                vote_winner="ls -la", operator_context=_make_mock_operator_context(os="linux", username="user", uid=1000),
                verifier_enabled=True, emitter=TribunalEmitter(None, None),
                command_constraints_message="No whitelist or blacklist constraints are active.",
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


class TestGenerateCommandHappyPath:
    """Full happy-path integration tests for generate_command.

    Each test exercises the complete pipeline (generation -> voting ->
    verification -> result) with a mocked LLM provider, validating the
    returned CommandGenerationResult and SSE event emissions.
    """

    @staticmethod
    def _settings(
        provider=LLMProvider.OLLAMA,
        assistant_model="gemma3:1b",
        passes=3,
        verifier=True,
    ):
        llm = LLMSettings(
            provider=provider,
            assistant_model=assistant_model,
            llm_command_gen_passes=passes,
            llm_command_gen_verifier=verifier,
        )
        return G8eeUserSettings(llm=llm)

    @staticmethod
    def _provider_returning(generation_text, verifier_text=None, *, passes=3):
        """Build a mock provider for a full pipeline run.

        The first ``passes`` calls return ``generation_text`` (concurrent
        generation stage). Subsequent calls return ``verifier_text`` (or
        repeat ``generation_text`` when ``verifier_text`` is ``None``).
        """
        call_count = 0

        async def _side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            resp = MagicMock()
            if call_count <= passes:
                resp.text = generation_text
            else:
                resp.text = verifier_text if verifier_text is not None else generation_text
            return resp

        return _make_mock_provider(generate_content_lite_side_effect=_side_effect)

    @pytest.mark.asyncio
    async def test_consensus_path_verifier_disabled(self):
        """All passes agree, verifier disabled -> CONSENSUS outcome."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("ls -la")
        settings = self._settings(verifier=False)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                original_command="ls",
                intent="list files with details",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/tmp", username="root", uid=0),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-1",
                user_id="user-happy-1",
                case_id="case-happy-1",
                investigation_id="inv-happy-1",
                settings=settings,
            )

        assert result.outcome == CommandGenerationOutcome.CONSENSUS
        assert result.final_command == "ls -la"
        assert result.original_command == "ls"
        assert len(result.candidates) == 3
        assert result.vote_winner == "ls -la"
        assert result.vote_score == 1.0
        assert result.verifier_passed is True
        assert result.verifier_revision is None

        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        from app.constants import EventType
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert emitted_types.count(EventType.TRIBUNAL_VOTING_PASS_COMPLETED) == 3
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_STARTED not in emitted_types

    @pytest.mark.asyncio
    async def test_verified_path_verifier_approves(self):
        """All passes agree, verifier says 'ok' -> VERIFIED outcome."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("find /var/log -name '*.log'", "ok")
        settings = self._settings(verifier=True)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                original_command="find logs",
                intent="find all log files under /var/log",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-2",
                user_id="user-happy-2",
                case_id="case-happy-2",
                investigation_id="inv-happy-2",
                settings=settings,
            )

        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.final_command == "find /var/log -name '*.log'"
        assert result.vote_winner == "find /var/log -name '*.log'"
        assert result.verifier_passed is True
        assert result.verifier_revision is None
        assert len(result.candidates) == 3

        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        from app.constants import EventType
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert emitted_types.count(EventType.TRIBUNAL_VOTING_PASS_COMPLETED) == 3
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED in emitted_types

    @pytest.mark.asyncio
    async def test_verification_failed_path_verifier_revises(self):
        """All passes agree, verifier revises -> VERIFICATION_FAILED outcome with revised command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("grep -r TODO .", "grep -rn TODO .")
        settings = self._settings(verifier=True)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                original_command="grep TODO",
                intent="find all TODO comments recursively with line numbers",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user/project", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-3",
                user_id="user-happy-3",
                case_id="case-happy-3",
                investigation_id="inv-happy-3",
                settings=settings,
            )

        assert result.outcome == CommandGenerationOutcome.VERIFICATION_FAILED
        assert result.final_command == "grep -rn TODO ."
        assert result.vote_winner == "grep -r TODO ."
        assert result.verifier_passed is False
        assert result.verifier_revision == "grep -rn TODO ."

        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        from app.constants import EventType
        assert EventType.TRIBUNAL_VOTING_REVIEW_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED in emitted_types

    @pytest.mark.asyncio
    async def test_partial_failure_surviving_candidates_reach_consensus(self):
        """1 of 3 passes fails, surviving 2 agree -> VERIFIED outcome."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()
        call_count = 0

        async def _side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 2:
                raise RuntimeError("Model overloaded")
            resp = MagicMock()
            if call_count <= 3:
                resp.text = "df -h"
            else:
                resp.text = "ok"
            return resp

        mock_provider = _make_mock_provider(generate_content_lite_side_effect=_side_effect)
        settings = self._settings(verifier=True, passes=3)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                original_command="df",
                intent="show disk usage in human-readable format",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-4",
                user_id="user-happy-4",
                case_id="case-happy-4",
                investigation_id="inv-happy-4",
                settings=settings,
            )

        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.final_command == "df -h"
        assert len(result.candidates) == 2
        assert result.vote_winner == "df -h"
        assert result.verifier_passed is True

    @pytest.mark.asyncio
    async def test_single_pass_verifier_approved(self):
        """Single-pass configuration (passes=1) still exercises all four stages."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("whoami", "ok", passes=1)
        settings = self._settings(verifier=True, passes=1)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                original_command="who",
                intent="show current user name",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-5",
                user_id="user-happy-5",
                case_id="case-happy-5",
                investigation_id="inv-happy-5",
                settings=settings,
            )

        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.final_command == "whoami"
        assert len(result.candidates) == 1
        assert result.vote_score == 1.0
        assert result.verifier_passed is True

    @pytest.mark.asyncio
    async def test_event_emission_order(self):
        """Events are emitted in pipeline stage order."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("uptime", "ok", passes=2)
        settings = self._settings(verifier=True, passes=2)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            await generate_command(
                original_command="up",
                intent="show system uptime",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-6",
                user_id="user-happy-6",
                case_id="case-happy-6",
                investigation_id="inv-happy-6",
                settings=settings,
            )

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]

        started_idx = emitted_types.index(EventType.TRIBUNAL_SESSION_STARTED)
        pass_indices = [i for i, t in enumerate(emitted_types) if t == EventType.TRIBUNAL_VOTING_PASS_COMPLETED]
        consensus_idx = emitted_types.index(EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED)
        review_started_idx = emitted_types.index(EventType.TRIBUNAL_VOTING_REVIEW_STARTED)
        review_completed_idx = emitted_types.index(EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED)
        completed_idx = emitted_types.index(EventType.TRIBUNAL_SESSION_COMPLETED)

        assert started_idx < min(pass_indices)
        assert max(pass_indices) < consensus_idx
        assert consensus_idx < review_started_idx
        assert review_started_idx < review_completed_idx
        assert review_completed_idx < completed_idx

    @pytest.mark.asyncio
    async def test_result_model_fields_fully_populated(self):
        """All CommandGenerationResult fields are populated after a full pipeline run."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("ls -la /tmp", "ok")
        settings = self._settings(verifier=True, passes=3)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                original_command="ls /tmp",
                intent="list files in /tmp with details",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-7",
                user_id="user-happy-7",
                case_id="case-happy-7",
                investigation_id="inv-happy-7",
                settings=settings,
            )

        assert result.original_command == "ls /tmp"
        assert result.final_command == "ls -la /tmp"
        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert len(result.candidates) == 3
        for i, c in enumerate(result.candidates):
            assert c.command == "ls -la /tmp"
            assert c.pass_index == i
            assert c.member == _member_for_pass(i)
        assert result.vote_winner == "ls -la /tmp"
        assert result.vote_score is not None
        assert 0.0 <= result.vote_score <= 1.0
        assert result.verifier_passed is True
        assert result.verifier_revision is None

    @pytest.mark.asyncio
    async def test_refined_command_differs_from_original(self):
        """When the pipeline refines a command, final_command differs from original_command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("cat /etc/hostname", "ok")
        settings = self._settings(verifier=True, passes=3)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                original_command="hostname",
                intent="show the system hostname from /etc/hostname",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-8",
                user_id="user-happy-8",
                case_id="case-happy-8",
                investigation_id="inv-happy-8",
                settings=settings,
            )

        assert result.final_command != result.original_command
        assert result.final_command == "cat /etc/hostname"

        from app.constants import EventType
        completed_calls = [
            call for call in mock_event_service.publish.call_args_list
            if call.args[0].event_type == EventType.TRIBUNAL_SESSION_COMPLETED
        ]
        assert len(completed_calls) == 1
        payload = completed_calls[0].args[0].payload
        assert payload.refined is True

    @pytest.mark.asyncio
    async def test_unchanged_command_marks_refined_false(self):
        """When final_command equals original_command, the completed event has refined=False."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("ls", "ok")
        settings = self._settings(verifier=True, passes=3)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                original_command="ls",
                intent="list files",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-9",
                user_id="user-happy-9",
                case_id="case-happy-9",
                investigation_id="inv-happy-9",
                settings=settings,
            )

        assert result.final_command == result.original_command

        from app.constants import EventType
        completed_calls = [
            call for call in mock_event_service.publish.call_args_list
            if call.args[0].event_type == EventType.TRIBUNAL_SESSION_COMPLETED
        ]
        assert len(completed_calls) == 1
        payload = completed_calls[0].args[0].payload
        assert payload.refined is False


class TestGenerateCommandVerifierFailure:
    """TribunalVerifierFailedError propagates through generate_command end-to-end.

    These tests exercise the full pipeline (generation -> voting -> verification)
    where the verifier stage fails, verifying the error surfaces correctly to callers
    with the right attributes and that SSE events are emitted in the correct order
    before the exception propagates.
    """

    @staticmethod
    def _settings(
        provider=LLMProvider.OLLAMA,
        assistant_model="gemma3:1b",
        passes=3,
    ):
        llm = LLMSettings(
            provider=provider,
            assistant_model=assistant_model,
            llm_command_gen_passes=passes,
            llm_command_gen_verifier=True,
        )
        return G8eeUserSettings(llm=llm)

    @staticmethod
    def _provider_with_verifier_behavior(
        generation_text: str,
        verifier_side_effect=None,
        verifier_return=None,
        *,
        passes: int = 3,
    ):
        """Build a mock provider where generation succeeds and verifier behaves as specified.

        The first ``passes`` calls return ``generation_text`` (generation stage).
        Subsequent calls use ``verifier_side_effect`` or ``verifier_return``.
        """
        call_count = 0

        async def _side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count <= passes:
                resp = MagicMock()
                resp.text = generation_text
                return resp
            if verifier_side_effect is not None:
                raise verifier_side_effect
            return verifier_return

        return _make_mock_provider(generate_content_lite_side_effect=_side_effect)

    @pytest.mark.asyncio
    async def test_empty_verifier_response_raises_through_generate_command(self):
        """Empty verifier response propagates TribunalVerifierFailedError from generate_command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        empty_response = MagicMock()
        empty_response.text = None

        mock_provider = self._provider_with_verifier_behavior(
            "ls -la", verifier_return=empty_response,
        )
        settings = self._settings()

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalVerifierFailedError) as exc_info:
                await generate_command(
                    original_command="ls",
                    intent="list files with details",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-1",
                    user_id="user-vf-1",
                    case_id="case-vf-1",
                    investigation_id="inv-vf-1",
                    settings=settings,
                )

            assert exc_info.value.reason == "empty_response"
            assert exc_info.value.original_command == "ls -la"
            assert "Verifier returned empty response" in exc_info.value.error

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert emitted_types.count(EventType.TRIBUNAL_VOTING_PASS_COMPLETED) == 3
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED not in emitted_types

        started_idx = emitted_types.index(EventType.TRIBUNAL_SESSION_STARTED)
        review_started_idx = emitted_types.index(EventType.TRIBUNAL_VOTING_REVIEW_STARTED)
        review_completed_idx = emitted_types.index(EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED)
        assert started_idx < review_started_idx < review_completed_idx

    @pytest.mark.asyncio
    async def test_no_valid_revision_raises_through_generate_command(self):
        """Non-ok verifier answer without valid revision propagates TribunalVerifierFailedError."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        same_command_response = MagicMock()
        same_command_response.text = "ls -la"

        mock_provider = self._provider_with_verifier_behavior(
            "ls -la", verifier_return=same_command_response,
        )
        settings = self._settings()

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalVerifierFailedError) as exc_info:
                await generate_command(
                    original_command="ls",
                    intent="list files with details",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-2",
                    user_id="user-vf-2",
                    case_id="case-vf-2",
                    investigation_id="inv-vf-2",
                    settings=settings,
                )

            assert exc_info.value.reason == "no_valid_revision"
            assert exc_info.value.original_command == "ls -la"
            assert "non-ok answer without valid revision" in exc_info.value.error

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED not in emitted_types

    @pytest.mark.asyncio
    async def test_verifier_exception_raises_through_generate_command(self):
        """Verifier exception propagates TribunalVerifierFailedError from generate_command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_with_verifier_behavior(
            "ls -la", verifier_side_effect=RuntimeError("Verifier API timeout"),
        )
        settings = self._settings()

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalVerifierFailedError) as exc_info:
                await generate_command(
                    original_command="ls",
                    intent="list files with details",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-3",
                    user_id="user-vf-3",
                    case_id="case-vf-3",
                    investigation_id="inv-vf-3",
                    settings=settings,
                )

            assert exc_info.value.reason == "exception"
            assert "timeout" in exc_info.value.error.lower()
            assert exc_info.value.original_command == "ls -la"

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_REVIEW_COMPLETED in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED not in emitted_types

    @pytest.mark.asyncio
    async def test_verifier_failure_preserves_original_command_from_vote_winner(self):
        """TribunalVerifierFailedError.original_command is the vote winner, not the caller's original command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        empty_response = MagicMock()
        empty_response.text = ""

        mock_provider = self._provider_with_verifier_behavior(
            "cat /etc/hostname", verifier_return=empty_response,
        )
        settings = self._settings()

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalVerifierFailedError) as exc_info:
                await generate_command(
                    original_command="hostname",
                    intent="show system hostname",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-4",
                    user_id="user-vf-4",
                    case_id="case-vf-4",
                    investigation_id="inv-vf-4",
                    settings=settings,
                )

            assert exc_info.value.original_command == "cat /etc/hostname"
            assert exc_info.value.reason == "empty_response"

    @pytest.mark.asyncio
    async def test_single_pass_verifier_failure_raises(self):
        """Single-pass configuration (passes=1) still raises TribunalVerifierFailedError on verifier failure."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_with_verifier_behavior(
            "whoami",
            verifier_side_effect=RuntimeError("Connection refused"),
            passes=1,
        )
        settings = self._settings(passes=1)

        with patch(
            "app.services.ai.command_generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalVerifierFailedError) as exc_info:
                await generate_command(
                    original_command="who",
                    intent="show current user",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-5",
                    user_id="user-vf-5",
                    case_id="case-vf-5",
                    investigation_id="inv-vf-5",
                    settings=settings,
                )

            assert exc_info.value.reason == "exception"
            assert exc_info.value.original_command == "whoami"

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        assert emitted_types.count(EventType.TRIBUNAL_VOTING_PASS_COMPLETED) == 1


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
            provider=mock_provider,
            model="test-model",
            intent="list files",
            original_command="ls",
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user"),
            pass_index=0,
            emitter=emitter,
            pass_errors=[],
            command_constraints_message="No whitelist or blacklist constraints are active.",
        
        )

        call_kwargs = mock_provider.generate_content_lite.call_args
        settings = call_kwargs.kwargs.get("lite_llm_settings")
        assert settings.max_output_tokens == _MAX_TOKENS_GENERATION

    @pytest.mark.asyncio
    async def test_verifier_uses_max_tokens(self):
        from unittest.mock import patch
        mock_response = MagicMock()
        mock_response.text = "ok"
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, None)

        # Mock get_model_config to return a config for verifier
        with patch('app.models.model_configs.get_model_config') as mock_get_config:
            mock_config = LLMModelConfig(
                name="test-model",
                max_output_tokens=_MAX_TOKENS_VERIFIER,
                top_p=1.0,
                top_k=None,
                stop_sequences=None,
            )
            mock_get_config.return_value = mock_config

            await _run_verifier(
            provider=mock_provider,
            model="test-model",
            intent="list files",
            candidate_command="ls -la",
            operator_context=_make_mock_operator_context(os="linux"),
            emitter=emitter,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            
        )

            call_kwargs = mock_provider.generate_content_lite.call_args
            settings = call_kwargs.kwargs.get("lite_llm_settings")
            assert settings.max_output_tokens == _MAX_TOKENS_VERIFIER


class TestForbiddenPatternsMessage:
    """The forbidden-patterns message must unconditionally ban sudo regardless of uid.

    Regression: a prior refactor introduced a uid-conditional variant that told the
    Tribunal sudo was acceptable for non-root users. That contradicts
    tool_service.execute_tool_call which rejects any command containing
    FORBIDDEN_COMMAND_PATTERNS, so any sudo-bearing Tribunal output was guaranteed
    to be blocked downstream with SECURITY_VIOLATION.
    """

    def test_message_always_critical_and_never(self):
        message = _format_forbidden_patterns_message()
        assert "CRITICAL" in message
        assert "NEVER" in message
        assert "privilege escalation" in message

    def test_message_lists_all_forbidden_base_patterns(self):
        from app.constants import FORBIDDEN_COMMAND_PATTERNS

        message = _format_forbidden_patterns_message()
        for pattern in FORBIDDEN_COMMAND_PATTERNS:
            base = pattern.strip()
            if base:
                assert base in message, f"{base!r} missing from forbidden-patterns message"

    def test_message_does_not_expose_uid_conditional_wording(self):
        """The message must not suggest sudo is sometimes acceptable."""
        message = _format_forbidden_patterns_message()
        assert "should only be added" not in message
        assert "include sudo explicitly" not in message


class TestBuildOperatorContextString:
    """_build_operator_context_string displays uid=0 correctly (regression)."""

    def test_uid_zero_is_displayed(self):
        ctx = OperatorContext(
            operator_id="op",
            username="root",
            uid=0,
        )
        rendered = _build_operator_context_string(ctx)
        assert "root" in rendered
        assert "uid=0" in rendered

    def test_none_uid_is_not_rendered(self):
        ctx = OperatorContext(
            operator_id="op",
            username="alice",
            uid=None,
        )
        rendered = _build_operator_context_string(ctx)
        assert "alice" in rendered
        assert "uid=" not in rendered

    def test_none_context_returns_placeholder(self):
        assert _build_operator_context_string(None) == "No operator context available"


class TestPromptFields:
    """_prompt_fields returns all keys required by every Tribunal persona template."""

    def test_returns_all_required_keys(self):
        fields = _prompt_fields(
            OperatorContext(
                operator_id="op",
                os="linux",
                shell="bash",
                working_directory="/home/alice",
                username="alice",
                uid=1000,
            )
        )
        assert fields["os"] == "linux"
        assert fields["shell"] == "bash"
        assert fields["working_directory"] == "/home/alice"
        assert fields["user_context"] == "alice (uid=1000)"
        assert "alice" in fields["operator_context"]
        assert "CRITICAL" in fields["forbidden_patterns_message"]

    def test_defaults_applied_when_context_none(self):
        from app.constants import DEFAULT_OS_NAME, DEFAULT_SHELL, DEFAULT_WORKING_DIRECTORY

        fields = _prompt_fields(None)
        assert fields["os"] == DEFAULT_OS_NAME
        assert fields["shell"] == DEFAULT_SHELL
        assert fields["working_directory"] == DEFAULT_WORKING_DIRECTORY
        assert fields["user_context"] == "unknown"

    def test_root_uid_is_preserved_in_user_context(self):
        fields = _prompt_fields(
            OperatorContext(operator_id="op", username="root", uid=0)
        )
        assert fields["user_context"] == "root (uid=0)"

    def test_all_tribunal_personas_accept_prompt_fields(self):
        """Every Tribunal persona template (+ auditor) must render with _prompt_fields.

        Regression: previously 4 of 6 personas lacked the {operator_context} placeholder
        and/or the _prompt_fields keys. str.format silently ignores unused kwargs but
        raises KeyError on missing placeholders — this test catches the latter and
        also verifies all six personas now receive {operator_context}.
        """
        from app.utils.agent_persona_loader import get_agent_persona, get_tribunal_member

        fields = _prompt_fields(
            OperatorContext(
                operator_id="op",
                os="linux",
                shell="bash",
                working_directory="/home/alice",
                username="alice",
                uid=1000,
                hostname="host1",
                architecture="x86_64",
            )
        )
        common = dict(
            command_constraints_message="No whitelist or blacklist constraints are active.",
            intent="list files",
        )

        for member_id in ("axiom", "concord", "variance", "pragma", "nemesis"):
            persona = get_tribunal_member(member_id)
            rendered = persona.persona.format(
                original_command="ls",
                **common,
                **fields,
            )
            assert "{operator_context}" in persona.persona, (
                f"{member_id}: persona template is missing the {{operator_context}} placeholder"
            )
            assert "host1" in rendered, f"{member_id}: operator_context did not render"
            assert "CRITICAL" in rendered, f"{member_id}: forbidden_patterns missing"

        auditor = get_agent_persona("auditor")
        rendered = auditor.get_system_prompt().format(
            candidate_command="ls -la",
            **common,
            **fields,
        )
        assert "host1" in rendered
        assert "CRITICAL" in rendered


class TestTribunalMemberCycling:
    """Tribunal member assignment cycles correctly through AXIOM, CONCORD, VARIANCE, PRAGMA, NEMESIS."""

    def test_member_for_pass_cycles_correctly(self):
        """_member_for_pass returns members in order: AXIOM (0), CONCORD (1), VARIANCE (2), PRAGMA (3), NEMESIS (4), then repeats."""
        assert _member_for_pass(0) == TribunalMember.AXIOM
        assert _member_for_pass(1) == TribunalMember.CONCORD
        assert _member_for_pass(2) == TribunalMember.VARIANCE
        assert _member_for_pass(3) == TribunalMember.PRAGMA
        assert _member_for_pass(4) == TribunalMember.NEMESIS
        assert _member_for_pass(5) == TribunalMember.AXIOM
        assert _member_for_pass(6) == TribunalMember.CONCORD
        assert _member_for_pass(7) == TribunalMember.VARIANCE
        assert _member_for_pass(8) == TribunalMember.PRAGMA
        assert _member_for_pass(9) == TribunalMember.NEMESIS


