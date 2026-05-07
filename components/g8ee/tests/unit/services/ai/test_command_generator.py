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

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import (
    AuditorReason,
    CommandGenerationOutcome,
    ComponentName,
    ErrorAnalysisCategory,
    EventType,
    LLMProvider,
    RiskLevel,
    TieBreakReason,
    TribunalMember,
)
from app.llm.llm_types import Role
from app.llm.prompts import (
    build_forbidden_patterns_message as _format_forbidden_patterns_message,
)
from app.llm.prompts import (
    build_tribunal_operator_context_string as _build_operator_context_string,
)
from app.llm.prompts import (
    build_tribunal_prompt_fields as _prompt_fields,
)
from app.models.agent import OperatorContext
from app.models.agents.tribunal import (
    CandidateCommand,
    TribunalAuditorFailedError,
    TribunalAuditorFailedPayload,
    TribunalDisabledError,
    TribunalGenerationFailedError,
    TribunalModelNotConfiguredError,
    TribunalProviderUnavailableError,
    TribunalSessionCompletedPayload,
    TribunalSessionDisabledPayload,
    TribunalSessionGenerationFailedPayload,
    TribunalSessionModelNotConfiguredPayload,
    TribunalSessionProviderUnavailablePayload,
    TribunalSessionStartedPayload,
    TribunalSessionSystemErrorPayload,
    TribunalSystemError,
    TribunalWardenBlockedError,
    TribunalWardenBlockedPayload,
    VoteBreakdown,
)
from app.models.http_context import G8eHttpContext
from app.models.model_configs import LLMModelConfig
from app.models.reputation import ReputationCommitment
from app.models.settings import G8eeUserSettings, LLMSettings
from app.models.tool_results import (
    CommandRiskAnalysis,
    ErrorAnalysisResult,
)
from app.services.ai.auditor_service import AuditorClusterInfo
from app.services.ai.generator import (
    TribunalEmitter,
    _build_and_emit_result,
    _member_for_pass,
    _resolve_model,
    generate_command,
)
from app.services.ai.tribunal.stages.generation import _run_generation_pass
from app.services.ai.tribunal.stages.auditor import TribunalAuditor
from app.services.ai.tribunal.stages.warden import _run_warden_stage
from app.services.ai.tribunal.utils import _is_system_error
from app.models.model_configs import get_model_config
from app.utils.agent_persona_loader import get_agent_persona

_TEST_HMAC_KEY = "a" * 64


def _make_mock_reputation_service() -> MagicMock:
    """Stub ReputationDataService sufficient for ``commit_reputation``.

    These tests assert Tribunal verdict-path behavior; commitment
    semantics are covered in ``test_auditor_commitment.py``. The stub
    returns an empty scoreboard and no prior commitment, so the commit
    step produces a deterministic genesis commitment without hitting any
    real cache/DB client.
    """
    svc = MagicMock()
    svc.list_states = AsyncMock(return_value=[])
    svc.get_latest_commitment = AsyncMock(return_value=None)

    async def _create_commitment(commitment: ReputationCommitment) -> ReputationCommitment:
        return commitment

    svc.create_commitment = AsyncMock(side_effect=_create_commitment)
    return svc


_REPUTATION_KWARGS = {
    "reputation_data_service": _make_mock_reputation_service(),
    "auditor_hmac_key": _TEST_HMAC_KEY,
}


_AUDIT_STAGE_REPUTATION_KWARGS = {
    **_REPUTATION_KWARGS,
    "investigation_id": "inv-test",
}


_MOCK_USER_SETTINGS = G8eeUserSettings(
    llm=LLMSettings(
        assistant_model="test-assistant",
        primary_model="test-primary",
        lite_model="test-lite"
    )
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


def _make_mock_g8e_context() -> G8eHttpContext:
    """Create a mock G8eHttpContext for tests."""
    return G8eHttpContext(
        web_session_id="test-session-id",
        user_id="test-user-id",
        case_id="test-case-id",
        investigation_id="test-investigation-id",
        source_component=ComponentName.G8EE,
    )


class TestRoleImportRegression:
    """Regression: command_generator must use Role.USER, not types.Role.USER.

    Before the fix, both _run_generation_pass and run_auditor referenced
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
        emitter = TribunalEmitter(None, _make_mock_g8e_context())
        pass_errors: list[str] = []

        # Importing from generation module
        from app.services.ai.tribunal.stages.generation import _run_generation_pass
        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
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
    async def test_auditor_uses_role_user_in_content(self):
        mock_response = MagicMock()
        mock_response.text = '{"status": "ok"}'

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, _make_mock_g8e_context())

        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )

        from app.services.ai.tribunal.stages.auditor import TribunalAuditor
        auditor = TribunalAuditor(
            emitter=emitter,
            **_REPUTATION_KWARGS,
        )
        audit_result = await auditor.run(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            vote_winner="ls -la",
            vote_breakdown=vote_breakdown,
            operator_context=_make_mock_operator_context(os="linux"),
            auditor_enabled=True,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-test",
        )
        final_cmd, outcome, auditor_passed, auditor_revision, auditor_reason, commitment_id = (
            audit_result.final_command,
            audit_result.outcome,
            audit_result.passed,
            audit_result.revision,
            audit_result.reason,
            audit_result.reputation_commitment_id,
        )

        assert auditor_passed is True
        assert auditor_revision is None
        call_kwargs = mock_provider.generate_content_lite.call_args
        contents = call_kwargs.kwargs.get("contents") or call_kwargs[1].get("contents")
        assert len(contents) == 1
        assert contents[0].role == Role.USER


class TestTribunalSystemError:
    """TribunalSystemError carries pass_errors and request."""

    def test_attributes(self):
        errors = ["401 Unauthorized", "Connection refused"]
        exc = TribunalSystemError(pass_errors=errors, request="list files")
        assert exc.pass_errors == errors
        assert exc.request == "list files"
        assert "401 Unauthorized" in str(exc)

    def test_is_exception(self):
        exc = TribunalSystemError(pass_errors=["err"], request="list files")
        assert isinstance(exc, Exception)


class TestPassErrorsCollection:
    """_run_generation_pass appends errors to the pass_errors list."""

    @pytest.mark.asyncio
    async def test_exception_appends_to_pass_errors(self):
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(
            side_effect=RuntimeError("Connection refused")
        )
        emitter = TribunalEmitter(None, _make_mock_g8e_context())
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
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
        emitter = TribunalEmitter(None, _make_mock_g8e_context())
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
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
        emitter = TribunalEmitter(None, _make_mock_g8e_context())
        pass_errors: list[str] = []

        result = await _run_generation_pass(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user"),
            pass_index=0,
            emitter=emitter,
            pass_errors=pass_errors,
            command_constraints_message="No whitelist or blacklist constraints are active.",

        )

        assert result == "ls -la"
        assert pass_errors == []


class TestTribunalSessionTerminalPayloads:
    """Each Tribunal terminal-state scenario has its own typed payload."""

    def test_system_error_payload_requires_pass_errors(self):
        payload = TribunalSessionSystemErrorPayload(
            request="list files",
            pass_errors=["auth failed", "network timeout"],
        )
        assert payload.request == "list files"
        assert payload.pass_errors == ["auth failed", "network timeout"]

    def test_generation_failed_payload_requires_pass_errors(self):
        payload = TribunalSessionGenerationFailedPayload(
            request="list files",
            pass_errors=["model refused"],
        )
        assert payload.pass_errors == ["model refused"]

    def test_model_not_configured_payload_requires_provider(self):
        payload = TribunalSessionModelNotConfiguredPayload(
            request="list files",
            provider="ollama",
            error="no model set",
        )
        assert payload.provider == "ollama"
        assert payload.error == "no model set"


class TestGenerateCommandOutcomes:
    """End-to-end outcomes for generate_command."""

    @pytest.mark.asyncio
    async def test_returns_disabled_outcome_when_tribunal_is_not_enabled(self):
        llm = LLMSettings(llm_command_gen_enabled=False)
        settings = G8eeUserSettings(llm=llm)

        with pytest.raises(TribunalDisabledError) as exc_info:
            await generate_command(
                request="list files",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/tmp", username="root", uid=0),
                g8ed_event_service=AsyncMock(),
                web_session_id="ws-1",
                user_id="user-1",
                case_id="case-1",
                investigation_id="inv-1",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert exc_info.value.request == "list files"


class TestGenerateCommandSystemError:
    """generate_command raises TribunalSystemError on all-system-error passes."""

    @pytest.mark.asyncio
    async def test_raises_on_all_system_errors(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            lite_provider=LLMProvider.OLLAMA,
            lite_model="gemma3:1b",
        )
        settings = G8eeUserSettings(llm=llm)

        mock_provider = _make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("401 Unauthorized")
        )

        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalSystemError) as exc_info:
                await generate_command(
                    request="list files",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert len(exc_info.value.pass_errors) > 0
            assert all("401" in e for e in exc_info.value.pass_errors)

    @pytest.mark.asyncio
    async def test_raises_generation_failed_error_on_non_system_errors(self):
        """Non-system errors now raise TribunalGenerationFailedError instead of silent fallback."""
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            lite_provider=LLMProvider.OLLAMA,
            lite_model="gemma3:1b",
        )
        settings = G8eeUserSettings(llm=llm)

        mock_provider = _make_mock_provider(
            generate_content_lite_side_effect=RuntimeError("Model returned gibberish")
        )

        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalGenerationFailedError) as exc_info:
                await generate_command(
                    request="list files",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.request == "list files"
            assert len(exc_info.value.pass_errors) > 0

    @pytest.mark.asyncio
    async def test_provider_routing_uses_settings_provider(self):
        llm = LLMSettings(
            primary_provider=LLMProvider.GEMINI,
            lite_provider=LLMProvider.OLLAMA,
            lite_model="gemma3:1b",
        )
        settings = G8eeUserSettings(llm=llm)

        call_count = 0

        async def mock_generate_content_lite(**kwargs):
            nonlocal call_count
            call_count += 1
            mock_response = MagicMock()
            if call_count <= 3:  # 3 generation passes
                mock_response.text = "ls -la"
            else:  # auditor call
                mock_response.text = '{"status": "ok"}'
            return mock_response

        mock_provider = _make_mock_provider(generate_content_lite_side_effect=mock_generate_content_lite)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ) as mock_factory:
            mock_event_service = MagicMock()
            mock_event_service.publish = AsyncMock()
            result = await generate_command(
                request="list files",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-1",
                user_id="user-1",
                case_id="case-1",
                investigation_id="inv-1",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

            # Tribunal generation uses lite tier, auditor uses primary tier
            assert mock_factory.call_count == 2
            # First call is for tribunal generation (lite)
            assert mock_factory.call_args_list[0] == ((settings.llm,), {"is_lite": True})
            # Second call is for auditor (primary)
            assert mock_factory.call_args_list[1] == ((settings.llm,), {"is_assistant": False, "is_lite": False})
            assert result.final_command == "ls -la"


class TestMixedErrorFallback:
    """Mixed system + non-system errors raise TribunalGenerationFailedError, not TribunalSystemError."""

    @pytest.mark.asyncio
    async def test_mixed_errors_raise_generation_failed_error(self):
        """1 system error + 2 non-system errors must raise TribunalGenerationFailedError."""
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            lite_provider=LLMProvider.OLLAMA,
            lite_model="gemma3:1b",
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

        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalGenerationFailedError) as exc_info:
                await generate_command(
                    request="list files",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.request == "list files"
            assert len(exc_info.value.pass_errors) == 3


class TestNewEnumValues:
    """New enum values exist and are distinct from existing ones."""

    def test_tribunal_terminal_events_are_distinct(self):
        # Terminal states are distinguished by event type, not a reason enum.
        assert EventType.TRIBUNAL_SESSION_SYSTEM_ERROR != EventType.TRIBUNAL_SESSION_GENERATION_FAILED
        assert EventType.TRIBUNAL_SESSION_DISABLED != EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED
        assert EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE != EventType.TRIBUNAL_SESSION_AUDITOR_FAILED


class TestTribunalProviderUnavailableError:
    """TribunalProviderUnavailableError raised when provider cannot be initialized."""

    @pytest.mark.asyncio
    async def test_raises_on_provider_init_failure(self):
        """Provider init failure raises TribunalProviderUnavailableError instead of silent fallback."""
        llm = LLMSettings(
            lite_provider=LLMProvider.OLLAMA,
            lite_model="gemma3:1b",
        )
        settings = G8eeUserSettings(llm=llm)

        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        with patch(
            "app.services.ai.generator.get_llm_provider",
            side_effect=RuntimeError("Unsupported LLM provider: foo"),
        ):
            with pytest.raises(TribunalProviderUnavailableError) as exc_info:
                await generate_command(
                    request="list files",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.provider == "ollama"
            assert exc_info.value.request == "list files"
            assert "Unsupported LLM provider" in exc_info.value.error


class TestTribunalModelNotConfiguredError:
    """TribunalModelNotConfiguredError raised when no model is configured."""

    @pytest.mark.asyncio
    async def test_raises_on_no_model_configured(self):
        """No model configured raises TribunalModelNotConfiguredError with fallback event."""
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
        )
        settings = G8eeUserSettings(llm=llm)

        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        with patch(
            "app.services.ai.generator.get_llm_provider",
        ):
            with pytest.raises(TribunalModelNotConfiguredError) as exc_info:
                await generate_command(
                    request="list files",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-1",
                    user_id="user-1",
                    case_id="case-1",
                    investigation_id="inv-1",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.provider == "ollama"
            assert exc_info.value.request == "list files"
            assert "model not configured" in str(exc_info.value).lower()

            mock_event_service.publish.assert_called()
            call_args = mock_event_service.publish.call_args
            event = call_args[0][0]
            assert event.event_type == EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED
            assert event.payload.provider == "ollama"
            assert isinstance(event.payload, TribunalSessionModelNotConfiguredPayload)


class TestTribunalAuditorFailedError:
    """TribunalAuditorFailedError raised when auditor fails and cannot validate candidate."""

    @pytest.mark.asyncio
    async def test_raises_on_empty_auditor_response(self):
        """Empty auditor response raises TribunalAuditorFailedError instead of treating as passed."""
        mock_response = MagicMock()
        mock_response.text = None

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, _make_mock_g8e_context())

        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )

        from app.services.ai.tribunal.stages.auditor import TribunalAuditor
        auditor = TribunalAuditor(
            emitter=emitter,
            **_REPUTATION_KWARGS,
        )
        with pytest.raises(TribunalAuditorFailedError) as exc_info:
            await auditor.run(
                provider=mock_provider,
                model="test-model",
                request="list files",
                guidelines="",
                vote_winner="ls -la",
                vote_breakdown=vote_breakdown,
                operator_context=_make_mock_operator_context(os="linux"),
                auditor_enabled=True,
                command_constraints_message="No whitelist or blacklist constraints are active",
                investigation_id="inv-test",
            )

        assert exc_info.value.reason == AuditorReason.EMPTY_RESPONSE
        assert exc_info.value.request == "list files"

    @pytest.mark.asyncio
    async def test_raises_on_no_valid_revision(self):
        """Non-ok answer without valid revision raises TribunalAuditorFailedError instead of treating as passed."""
        mock_response = MagicMock()
        # Response that's not "ok" and normalizes to the same command (not a valid revision)
        mock_response.text = "ls -la"

        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(return_value=mock_response)
        emitter = TribunalEmitter(None, _make_mock_g8e_context())

        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )

        from app.services.ai.tribunal.stages.auditor import TribunalAuditor
        auditor = TribunalAuditor(
            emitter=emitter,
            **_REPUTATION_KWARGS,
        )
        with pytest.raises(TribunalAuditorFailedError) as exc_info:
            await auditor.run(
                provider=mock_provider,
                model="test-model",
                request="list files",
                guidelines="",
                vote_winner="ls -la",
                vote_breakdown=vote_breakdown,
                operator_context=_make_mock_operator_context(os="linux"),
                auditor_enabled=True,
                command_constraints_message="No whitelist or blacklist constraints are active",
                investigation_id="inv-test",
            )

        assert exc_info.value.reason == AuditorReason.NO_VALID_REVISION
        assert exc_info.value.request == "list files"

    @pytest.mark.asyncio
    async def test_raises_on_auditor_exception(self):
        """Auditor exception raises TribunalAuditorFailedError instead of treating as passed."""
        mock_provider = MagicMock()
        mock_provider.generate_content_lite = AsyncMock(
            side_effect=RuntimeError("Auditor API timeout")
        )
        emitter = TribunalEmitter(None, _make_mock_g8e_context())

        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )

        from app.services.ai.tribunal.stages.auditor import TribunalAuditor
        auditor = TribunalAuditor(
            emitter=emitter,
            **_REPUTATION_KWARGS,
        )
        with pytest.raises(TribunalAuditorFailedError) as exc_info:
            await auditor.run(
                provider=mock_provider,
                model="test-model",
                request="list files",
                guidelines="",
                vote_winner="ls -la",
                vote_breakdown=vote_breakdown,
                operator_context=_make_mock_operator_context(os="linux"),
                auditor_enabled=True,
                command_constraints_message="No whitelist or blacklist constraints are active",
                investigation_id="inv-test",
            )

        assert exc_info.value.reason == AuditorReason.AUDITOR_ERROR
        assert "timeout" in exc_info.value.error
        assert exc_info.value.request == "list files"

class TestRunAuditStageWardenRiskAnalysis:
    """TribunalAuditor exercises Warden risk analysis before the Auditor.

    Regression coverage for the Warden command-risk path: ``CommandRiskAnalysis``
    only carries ``risk_level``. Touching any other attribute on the analysis
    object inside ``_run_audit_stage`` is a bug. These tests cover both the
    LOW-risk pass-through and the HIGH-risk Two-Strike Circuit Breaker paths
    so any future drift between the model and the consumer fails fast.
    """

    @staticmethod
    def _make_breakdown():
        return VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )

    @staticmethod
    def _make_analyzer(risk_level, error_user_message: str | None = None, error_suggested_fix: str | None = None):
        from app.constants import ErrorAnalysisCategory
        from app.models.tool_results import CommandRiskAnalysis, ErrorAnalysisResult
        analyzer = MagicMock()
        analyzer.analyze_command_risk = AsyncMock(
            return_value=CommandRiskAnalysis(risk_level=risk_level)
        )
        analyzer.analyze_error_and_suggest_fix = AsyncMock(
            return_value=ErrorAnalysisResult(
                error_category=ErrorAnalysisCategory.UNKNOWN,
                root_cause="HIGH-risk command",
                can_auto_fix=False,
                should_escalate=True,
                reasoning="warden block",
                user_message=error_user_message or "Use a safer alternative.",
                suggested_fix=error_suggested_fix,
            )
        )
        return analyzer

    @pytest.mark.asyncio
    async def test_low_risk_passes_through_without_attribute_error(self):
        """LOW risk classification logs and proceeds to the auditor.

        Regression: the post-analysis log statement must only reference
        attributes that exist on ``CommandRiskAnalysis``. Before the fix it
        accessed ``risk_score`` and ``reason``, both removed when the
        Warden contract was simplified to ``risk_level`` only.
        """
        from app.constants import RiskLevel
        analyzer = self._make_analyzer(RiskLevel.LOW)
        emitter = TribunalEmitter(None, _make_mock_g8e_context())

        risk_analysis = await _run_warden_stage(
            request="list files", guidelines="",
            vote_winner="ls -la",
            operator_context=_make_mock_operator_context(os="linux", username="user", uid=1000),
            emitter=emitter,
            settings=_MOCK_USER_SETTINGS,
            ai_response_analyzer=analyzer,
            investigation_id="inv-test",
            investigation_state=MagicMock(),
        )

        analyzer.analyze_command_risk.assert_awaited_once()
        assert risk_analysis is not None
        assert risk_analysis.risk_level == RiskLevel.LOW

    @pytest.mark.asyncio
    async def test_high_risk_first_strike_emits_warden_blocked_and_increments_counter(self):
        """HIGH risk on a fresh investigation raises a first-strike block."""
        from app.constants import EventType, RiskLevel
        from app.models.agents.tribunal import (
            TribunalWardenBlockedError,
            TribunalWardenBlockedPayload,
        )

        analyzer = self._make_analyzer(
            RiskLevel.HIGH,
            error_user_message="Command blocked. Try a read-only listing.",
            error_suggested_fix="ls -la /etc",
        )
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()
        emitter = TribunalEmitter(mock_event_service, _make_mock_g8e_context(), correlation_id="corr-warden-1")

        investigation_state = MagicMock()
        investigation_state.warden_block_count = 0

        with pytest.raises(TribunalWardenBlockedError) as exc_info:
            await _run_warden_stage(
                request="purge logs", guidelines="",
                vote_winner="rm -rf /var/log",
                operator_context=_make_mock_operator_context(os="linux", username="root", uid=0),
                emitter=emitter,
                settings=_MOCK_USER_SETTINGS,
                ai_response_analyzer=analyzer,
                investigation_id="inv-test",
                investigation_state=investigation_state,
            )

        assert exc_info.value.risk_level == RiskLevel.HIGH
        assert investigation_state.warden_block_count == 1
        analyzer.analyze_error_and_suggest_fix.assert_awaited_once()

        emitted_types = [call.args[0].event_type for call in mock_event_service.publish.call_args_list]
        assert EventType.TRIBUNAL_SESSION_WARDEN_BLOCKED in emitted_types
        warden_payloads = [
            call.args[0].payload
            for call in mock_event_service.publish.call_args_list
            if call.args[0].event_type == EventType.TRIBUNAL_SESSION_WARDEN_BLOCKED
        ]
        assert len(warden_payloads) == 1
        payload = warden_payloads[0]
        assert isinstance(payload, TribunalWardenBlockedPayload)
        assert payload.risk_level == RiskLevel.HIGH
        assert payload.is_conflict is False

    @pytest.mark.asyncio
    async def test_high_risk_second_strike_emits_agent_conflict_and_resets_counter(self):
        """HIGH risk after a prior block raises an agent-conflict second strike."""
        from app.constants import EventType, RiskLevel
        from app.models.agents.tribunal import (
            TribunalWardenBlockedError,
            TribunalWardenBlockedPayload,
        )

        analyzer = self._make_analyzer(RiskLevel.HIGH)
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()
        emitter = TribunalEmitter(mock_event_service, _make_mock_g8e_context(), correlation_id="corr-warden-2")

        investigation_state = MagicMock()
        investigation_state.warden_block_count = 1

        with pytest.raises(TribunalWardenBlockedError) as exc_info:
            await _run_warden_stage(
                request="purge logs", guidelines="",
                vote_winner="rm -rf /var/log",
                operator_context=_make_mock_operator_context(os="linux", username="root", uid=0),
                emitter=emitter,
                settings=_MOCK_USER_SETTINGS,
                ai_response_analyzer=analyzer,
                investigation_id="inv-test",
                investigation_state=investigation_state,
            )

        assert exc_info.value.risk_level == RiskLevel.HIGH
        assert investigation_state.warden_block_count == 0
        analyzer.analyze_error_and_suggest_fix.assert_not_awaited()

        emitted_types = [call.args[0].event_type for call in mock_event_service.publish.call_args_list]
        assert EventType.AI_AGENT_CONFLICT_DETECTED in emitted_types
        conflict_payloads = [
            call.args[0].payload
            for call in mock_event_service.publish.call_args_list
            if call.args[0].event_type == EventType.AI_AGENT_CONFLICT_DETECTED
        ]
        assert len(conflict_payloads) == 1
        payload = conflict_payloads[0]
        assert isinstance(payload, TribunalWardenBlockedPayload)
        assert payload.risk_level == RiskLevel.HIGH
        assert payload.is_conflict is True


class TestBuildAndEmitResult:
    """_build_and_emit_result assembles the result model correctly."""

    @pytest.mark.asyncio
    async def test_builds_complete_result(self):
        candidates = [
            CandidateCommand(command="ls -la", pass_index=0, member=TribunalMember.AXIOM),
        ]
        vote_breakdown = VoteBreakdown(
            candidates_by_member={"axiom": "ls -la"},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        emitter = TribunalEmitter(None, _make_mock_g8e_context())

        result = await _build_and_emit_result(
            request="list files", guidelines="", final_command="ls -la",
            outcome=CommandGenerationOutcome.VERIFIED, candidates=candidates,
            vote_winner="ls -la", vote_score=1.0, vote_breakdown=vote_breakdown,
            auditor_passed=True, auditor_revision=None, auditor_reason=AuditorReason.OK,
            emitter=emitter,
            reputation_commitment_id="comm-1",
        )

        assert result.request == "list files"
        assert result.final_command == "ls -la"
        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.vote_winner == "ls -la"
        assert result.vote_score == 1.0
        assert result.vote_breakdown is not None
        assert result.auditor_passed is True
        assert result.auditor_revision is None
        assert result.auditor_reason == AuditorReason.OK


class TestGenerateCommandHappyPath:
    """Full happy-path integration tests for generate_command.

    Each test exercises the complete pipeline (generation -> voting ->
    verification -> result) with a mocked LLM provider, validating the
    returned CommandGenerationResult and SSE event emissions.
    """

    @staticmethod
    def _settings(
        primary_provider=LLMProvider.OLLAMA,
        lite_provider=LLMProvider.OLLAMA,
        lite_model="gemma3:1b",
        auditor=True,
        passes=3,
    ):
        llm = LLMSettings(
            primary_provider=primary_provider,
            lite_provider=lite_provider,
            lite_model=lite_model,
            llm_command_gen_passes=passes,
            llm_command_gen_auditor=auditor,
        )
        return G8eeUserSettings(llm=llm)

    @staticmethod
    def _provider_returning(generation_text, auditor_text=None, *, passes=3):
        """Build a mock provider for a full pipeline run.

        The first ``passes`` calls return ``generation_text`` (concurrent
        generation stage). Subsequent calls return ``auditor_text`` (or
        repeat ``generation_text`` when ``auditor_text`` is ``None``).
        
        Auditor responses are automatically converted to JSON format.
        """
        call_count = 0

        async def _side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            resp = MagicMock()
            if call_count <= passes:
                resp.text = generation_text
            elif auditor_text is not None:
                # Convert auditor text to JSON format
                if auditor_text == "ok":
                    resp.text = '{"status": "ok"}'
                else:
                    # Assume it's a revised command
                    import json
                    resp.text = json.dumps({"status": "revised", "revised_command": auditor_text})
            else:
                resp.text = generation_text
            return resp

        return _make_mock_provider(generate_content_lite_side_effect=_side_effect)

    @pytest.mark.asyncio
    async def test_consensus_path_auditor_disabled(self):
        """All passes agree, auditor disabled -> CONSENSUS outcome."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("ls -la")
        settings = self._settings(auditor=False)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                request="list files with details",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/tmp", username="root", uid=0),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-1",
                user_id="user-happy-1",
                case_id="case-happy-1",
                investigation_id="inv-happy-1",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert result.outcome == CommandGenerationOutcome.CONSENSUS
        assert result.final_command == "ls -la"
        assert result.request == "list files with details"
        assert len(result.candidates) == 3
        assert result.vote_winner == "ls -la"
        assert result.vote_score == 1.0
        assert result.auditor_passed is True
        assert result.auditor_revision is None

        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        from app.constants import EventType
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert emitted_types.count(EventType.TRIBUNAL_VOTING_PASS_COMPLETED) == 3
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_STARTED not in emitted_types

    @pytest.mark.asyncio
    async def test_verified_path_auditor_approves(self):
        """All passes agree, auditor says 'ok' -> VERIFIED outcome."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("find /var/log -name '*.log'", "ok")
        settings = self._settings(auditor=True)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                request="find all log files under /var/log",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-2",
                user_id="user-happy-2",
                case_id="case-happy-2",
                investigation_id="inv-happy-2",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.final_command == "find /var/log -name '*.log'"
        assert result.vote_winner == "find /var/log -name '*.log'"
        assert result.auditor_passed is True
        assert result.auditor_revision is None
        assert len(result.candidates) == 3

        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        from app.constants import EventType
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert emitted_types.count(EventType.TRIBUNAL_VOTING_PASS_COMPLETED) == 3
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED in emitted_types

    @pytest.mark.asyncio
    async def test_verification_failed_path_auditor_revises(self):
        """All passes agree, auditor revises -> VERIFICATION_FAILED outcome with revised command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("grep -r TODO .", "grep -rn TODO .")
        settings = self._settings(auditor=True)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                request="find all TODO comments recursively with line numbers",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user/project", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-3",
                user_id="user-happy-3",
                case_id="case-happy-3",
                investigation_id="inv-happy-3",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert result.outcome == CommandGenerationOutcome.VERIFICATION_FAILED
        assert result.final_command == "grep -rn TODO ."
        assert result.vote_winner == "grep -r TODO ."
        assert result.auditor_passed is False
        assert result.auditor_revision == "grep -rn TODO ."

        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        from app.constants import EventType
        assert EventType.TRIBUNAL_VOTING_AUDIT_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED in emitted_types
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
                resp.text = '{"status": "ok"}'
            return resp

        mock_provider = _make_mock_provider(generate_content_lite_side_effect=_side_effect)
        settings = self._settings(auditor=True, passes=3)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                request="show disk usage in human-readable format",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-4",
                user_id="user-happy-4",
                case_id="case-happy-4",
                investigation_id="inv-happy-4",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.final_command == "df -h"
        assert len(result.candidates) == 2
        assert result.vote_winner == "df -h"
        assert result.auditor_passed is True

    @pytest.mark.asyncio
    async def test_single_pass_auditor_approved(self):
        """Minimal configuration (passes=2) still exercises all four stages."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("whoami", "ok", passes=2)
        settings = self._settings(auditor=True, passes=2)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                request="show current user name",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-5",
                user_id="user-happy-5",
                case_id="case-happy-5",
                investigation_id="inv-happy-5",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.final_command == "whoami"
        assert len(result.candidates) == 2
        assert result.vote_score == 1.0
        assert result.auditor_passed is True

    @pytest.mark.asyncio
    async def test_event_emission_order(self):
        """Events are emitted in pipeline stage order."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("uptime", "ok", passes=2)
        settings = self._settings(auditor=True, passes=2)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            await generate_command(
                request="show system uptime",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-6",
                user_id="user-happy-6",
                case_id="case-happy-6",
                investigation_id="inv-happy-6",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]

        started_idx = emitted_types.index(EventType.TRIBUNAL_SESSION_STARTED)
        pass_indices = [i for i, t in enumerate(emitted_types) if t == EventType.TRIBUNAL_VOTING_PASS_COMPLETED]
        consensus_idx = emitted_types.index(EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED)
        review_started_idx = emitted_types.index(EventType.TRIBUNAL_VOTING_AUDIT_STARTED)
        review_completed_idx = emitted_types.index(EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED)
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
        settings = self._settings(auditor=True, passes=3)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                request="list files in /tmp with details",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-7",
                user_id="user-happy-7",
                case_id="case-happy-7",
                investigation_id="inv-happy-7",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert result.request == "list files in /tmp with details"
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
        assert result.auditor_passed is True
        assert result.auditor_revision is None

    @pytest.mark.asyncio
    async def test_refined_command_differs_from_original(self):
        """When the pipeline refines a command, final_command differs from request and payload reflects the refinement."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("cat /etc/hostname", "ok")
        settings = self._settings(auditor=True, passes=3)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                request="show the system hostname from /etc/hostname",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-8",
                user_id="user-happy-8",
                case_id="case-happy-8",
                investigation_id="inv-happy-8",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert result.final_command != result.request
        assert result.final_command == "cat /etc/hostname"

        from app.constants import EventType
        completed_calls = [
            call for call in mock_event_service.publish.call_args_list
            if call.args[0].event_type == EventType.TRIBUNAL_SESSION_COMPLETED
        ]
        assert len(completed_calls) == 1
        payload = completed_calls[0].args[0].payload
        assert payload.request == "show the system hostname from /etc/hostname"
        assert payload.final_command == "cat /etc/hostname"

    @pytest.mark.asyncio
    async def test_unchanged_command_marks_refined_false(self):
        """When final_command equals request, the completed event payload reflects the unchanged command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_returning("ls", "ok")
        settings = self._settings(auditor=True, passes=3)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            result = await generate_command(
                request="ls",
                guidelines="",
                operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-happy-9",
                user_id="user-happy-9",
                case_id="case-happy-9",
                investigation_id="inv-happy-9",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

        assert result.final_command == result.request

        from app.constants import EventType
        completed_calls = [
            call for call in mock_event_service.publish.call_args_list
            if call.args[0].event_type == EventType.TRIBUNAL_SESSION_COMPLETED
        ]
        assert len(completed_calls) == 1
        payload = completed_calls[0].args[0].payload
        assert payload.request == "ls"
        assert payload.final_command == "ls"


class TestGenerateCommandAuditorFailure:
    """TribunalAuditorFailedError propagates through generate_command end-to-end.

    These tests exercise the full pipeline (generation -> voting -> verification)
    where the auditor stage fails, verifying the error surfaces correctly to callers
    with the right attributes and that SSE events are emitted in the correct order
    before the exception propagates.
    """

    @staticmethod
    def _settings(
        primary_provider=LLMProvider.OLLAMA,
        lite_provider=LLMProvider.OLLAMA,
        lite_model="gemma3:1b",
        passes=3,
    ):
        llm = LLMSettings(
            primary_provider=primary_provider,
            lite_provider=lite_provider,
            lite_model=lite_model,
            llm_command_gen_passes=passes,
            llm_command_gen_auditor=True,
        )
        return G8eeUserSettings(llm=llm)

    @staticmethod
    def _provider_with_auditor_behavior(
        generation_text: str,
        auditor_side_effect=None,
        auditor_return=None,
        *,
        passes: int = 3,
    ):
        """Build a mock provider where generation succeeds and auditor behaves as specified.

        The first ``passes`` calls return ``generation_text`` (generation stage).
        Subsequent calls use ``auditor_side_effect`` or ``auditor_return``.
        
        Auditor responses are automatically converted to JSON format if they are plain text.
        """
        call_count = 0

        async def _side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count <= passes:
                resp = MagicMock()
                resp.text = generation_text
                return resp
            if auditor_side_effect is not None:
                raise auditor_side_effect
            if auditor_return is not None:
                # Convert auditor response to JSON format if it's plain text
                if hasattr(auditor_return, "text"):
                    text = auditor_return.text
                    if text == "ok":
                        auditor_return.text = '{"status": "ok"}'
                    elif text and text != generation_text:
                        # Assume it's a revised command
                        import json
                        auditor_return.text = json.dumps({"status": "revised", "revised_command": text})
            return auditor_return

        return _make_mock_provider(generate_content_lite_side_effect=_side_effect)

    @pytest.mark.asyncio
    async def test_empty_auditor_response_raises_through_generate_command(self):
        """Empty auditor response propagates TribunalAuditorFailedError from generate_command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        empty_response = MagicMock()
        empty_response.text = None

        mock_provider = self._provider_with_auditor_behavior(
            "ls -la", auditor_return=empty_response,
        )
        settings = self._settings()

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalAuditorFailedError) as exc_info:
                await generate_command(
                    request="list files with details",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-1",
                    user_id="user-vf-1",
                    case_id="case-vf-1",
                    investigation_id="inv-vf-1",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.reason == AuditorReason.EMPTY_RESPONSE
            assert exc_info.value.request == "list files with details"
            assert "Provider returned empty response" in exc_info.value.error

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert emitted_types.count(EventType.TRIBUNAL_VOTING_PASS_COMPLETED) == 3
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_STARTED in emitted_types
        assert EventType.TRIBUNAL_SESSION_AUDITOR_FAILED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED not in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED not in emitted_types

    @pytest.mark.asyncio
    async def test_no_valid_revision_raises_through_generate_command(self):
        """Non-ok auditor answer without valid revision propagates TribunalAuditorFailedError."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        same_command_response = MagicMock()
        same_command_response.text = "ls -la"

        mock_provider = self._provider_with_auditor_behavior(
            "ls -la", auditor_return=same_command_response,
        )
        settings = self._settings()

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalAuditorFailedError) as exc_info:
                await generate_command(
                    request="list files with details",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-2",
                    user_id="user-vf-2",
                    case_id="case-vf-2",
                    investigation_id="inv-vf-2",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.reason == AuditorReason.NO_VALID_REVISION
            assert exc_info.value.request == "list files with details"
            assert "invalid JSON" in exc_info.value.error

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_STARTED in emitted_types
        assert EventType.TRIBUNAL_SESSION_AUDITOR_FAILED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED not in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED not in emitted_types

    @pytest.mark.asyncio
    async def test_auditor_exception_raises_through_generate_command(self):
        """Auditor exception propagates TribunalAuditorFailedError from generate_command."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_with_auditor_behavior(
            "ls -la", auditor_side_effect=RuntimeError("Auditor API timeout"),
        )
        settings = self._settings()

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalAuditorFailedError) as exc_info:
                await generate_command(
                    request="list files with details",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-3",
                    user_id="user-vf-3",
                    case_id="case-vf-3",
                    investigation_id="inv-vf-3",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.reason == AuditorReason.AUDITOR_ERROR
            assert "timeout" in exc_info.value.error.lower()
            assert exc_info.value.request == "list files with details"

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        assert EventType.TRIBUNAL_SESSION_STARTED in emitted_types
        assert EventType.TRIBUNAL_VOTING_CONSENSUS_REACHED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_STARTED in emitted_types
        assert EventType.TRIBUNAL_SESSION_AUDITOR_FAILED in emitted_types
        assert EventType.TRIBUNAL_VOTING_AUDIT_COMPLETED not in emitted_types
        assert EventType.TRIBUNAL_SESSION_COMPLETED not in emitted_types

    @pytest.mark.asyncio
    async def test_auditor_failure_preserves_request_from_vote_winner(self):
        """TribunalAuditorFailedError.request is the vote winner, not the caller's request."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        empty_response = MagicMock()
        empty_response.text = ""

        mock_provider = self._provider_with_auditor_behavior(
            "cat /etc/hostname", auditor_return=empty_response,
        )
        settings = self._settings()

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalAuditorFailedError) as exc_info:
                await generate_command(
                    request="show system hostname",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-4",
                    user_id="user-vf-4",
                    case_id="case-vf-4",
                    investigation_id="inv-vf-4",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.request == "show system hostname"
            assert exc_info.value.reason == AuditorReason.EMPTY_RESPONSE

    @pytest.mark.asyncio
    async def test_single_pass_auditor_failure_raises(self):
        """Minimal configuration (passes=2) still raises TribunalAuditorFailedError on auditor failure."""
        mock_event_service = MagicMock()
        mock_event_service.publish = AsyncMock()

        mock_provider = self._provider_with_auditor_behavior(
            "whoami",
            auditor_side_effect=RuntimeError("Connection refused"),
            passes=2,
        )
        settings = self._settings(passes=2)

        with patch(
            "app.services.ai.generator.get_llm_provider",
            return_value=mock_provider,
        ):
            with pytest.raises(TribunalAuditorFailedError) as exc_info:
                await generate_command(
                    request="show current user",
                    guidelines="",
                    operator_context=_make_mock_operator_context(os="linux", shell="bash", working_directory="/home/user", username="user", uid=1000),
                    g8ed_event_service=mock_event_service,
                    web_session_id="ws-vf-5",
                    user_id="user-vf-5",
                    case_id="case-vf-5",
                    investigation_id="inv-vf-5",
                    settings=settings,
                    **_REPUTATION_KWARGS,
                )

            assert exc_info.value.reason == AuditorReason.AUDITOR_ERROR
            assert exc_info.value.request == "show current user"

        from app.constants import EventType
        emitted_types = [
            call.args[0].event_type
            for call in mock_event_service.publish.call_args_list
        ]
        assert emitted_types.count(EventType.TRIBUNAL_VOTING_PASS_COMPLETED) == 2


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
        assert "FORBIDDEN" in message
        assert "rejected" in message

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
        from app.constants import DEFAULT_OS_NAME, DEFAULT_SHELL, DEFAULT_WORKING_DIRECTORY
        fields = _prompt_fields(
            OperatorContext(
                operator_id="op",
                os="linux",
                shell="bash",
                working_directory="/home/alice",
                username="alice",
                uid=1000,
            ),
            request="test request",
            guidelines="test guidelines",
            default_os=DEFAULT_OS_NAME,
            default_shell=DEFAULT_SHELL,
            default_working_directory=DEFAULT_WORKING_DIRECTORY,
        )
        assert fields["os"] == "linux"
        assert fields["shell"] == "bash"
        assert fields["working_directory"] == "/home/alice"
        assert fields["user_context"] == "alice (uid=1000)"
        assert "alice" in fields["operator_context"]
        assert "FORBIDDEN" in fields["forbidden_patterns_message"]

    def test_defaults_applied_when_context_none(self):
        from app.constants import DEFAULT_OS_NAME, DEFAULT_SHELL, DEFAULT_WORKING_DIRECTORY

        fields = _prompt_fields(None, request="", guidelines="", default_os=DEFAULT_OS_NAME, default_shell=DEFAULT_SHELL, default_working_directory=DEFAULT_WORKING_DIRECTORY)
        assert fields["os"] == DEFAULT_OS_NAME
        assert fields["shell"] == DEFAULT_SHELL
        assert fields["working_directory"] == DEFAULT_WORKING_DIRECTORY
        assert fields["user_context"] == "unknown"

    def test_root_uid_is_preserved_in_user_context(self):
        from app.constants import DEFAULT_OS_NAME, DEFAULT_SHELL, DEFAULT_WORKING_DIRECTORY
        fields = _prompt_fields(
            OperatorContext(operator_id="op", username="root", uid=0),
            request="",
            guidelines="",
            default_os=DEFAULT_OS_NAME,
            default_shell=DEFAULT_SHELL,
            default_working_directory=DEFAULT_WORKING_DIRECTORY,
        )
        assert fields["user_context"] == "root (uid=0)"

    def test_all_tribunal_personas_accept_prompt_fields(self):
        """Every Tribunal template (+ auditor) must render cleanly with _prompt_fields.

        After the scaffolding refactor, placeholders live in TRIBUNAL_PROMPT_TEMPLATE
        and TRIBUNAL_AUDITOR_TEMPLATE — not in the persona text itself. This test
        guards against drift in either the templates or _prompt_fields.
        """
        from app.constants import DEFAULT_OS_NAME, DEFAULT_SHELL, DEFAULT_WORKING_DIRECTORY
        from app.llm.prompts import PromptFile
        from app.prompts_data.loader import load_prompt
        from app.utils.agent_persona_loader import get_agent_persona

        TRIBUNAL_PROMPT_TEMPLATE = load_prompt(PromptFile.TRIBUNAL_GENERATOR)
        TRIBUNAL_AUDITOR_TEMPLATE = load_prompt(PromptFile.TRIBUNAL_AUDITOR)

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
            ),
            request="test request",
            guidelines="test guidelines",
            default_os=DEFAULT_OS_NAME,
            default_shell=DEFAULT_SHELL,
            default_working_directory=DEFAULT_WORKING_DIRECTORY,
        )
        common = dict(
            command_constraints_message="No whitelist or blacklist constraints are active.",
        )

        for member_id in ("axiom", "concord", "variance", "pragma", "nemesis"):
            rendered = TRIBUNAL_PROMPT_TEMPLATE.format(
                **common,
                **fields,
            )
            assert "{operator_context}" in TRIBUNAL_PROMPT_TEMPLATE, (
                "TRIBUNAL_PROMPT_TEMPLATE missing {operator_context} placeholder"
            )
            assert "host1" in rendered, f"{member_id}: operator_context did not render"
            assert "FORBIDDEN" in rendered, f"{member_id}: forbidden_patterns missing"

        auditor = get_agent_persona("auditor")
        rendered = TRIBUNAL_AUDITOR_TEMPLATE.format(
            auditor_context="Auditor context placeholder",
            **common,
            **fields,
        )
        assert "host1" in rendered
        assert "FORBIDDEN" in rendered


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


class TestTribunalEmitter:
    """TribunalEmitter distinguishes terminal from progress events and handles publish failures appropriately."""

    @pytest.mark.asyncio
    async def test_terminal_event_publish_failure_raises(self):
        """Terminal event publish failures are re-raised to ensure caller is aware of the failure."""
        from app.models.http_context import G8eHttpContext
        from app.services.infra.g8ed_event_service import EventService

        mock_event_service = MagicMock(spec=EventService)
        mock_event_service.publish = AsyncMock(side_effect=RuntimeError("broker down"))

        ctx = G8eHttpContext(
            web_session_id="test-session",
            user_id="test-user",
            case_id="test-case",
            investigation_id="test-investigation",
            source_component=ComponentName.G8EE,
        )

        emitter = TribunalEmitter(event_service=mock_event_service, g8e_context=ctx)

        with pytest.raises(RuntimeError, match="broker down"):
            await emitter.emit(
                EventType.TRIBUNAL_SESSION_AUDITOR_FAILED,
                TribunalSessionGenerationFailedPayload(request="test", pass_errors=["error"]),
            )

    @pytest.mark.asyncio
    async def test_progress_event_publish_failure_swallowed(self):
        """Progress event publish failures are logged but not re-raised."""
        from app.models.agents.tribunal import TribunalPassCompletedPayload
        from app.models.http_context import G8eHttpContext
        from app.services.infra.g8ed_event_service import EventService

        mock_event_service = MagicMock(spec=EventService)
        mock_event_service.publish = AsyncMock(side_effect=RuntimeError("broker down"))

        ctx = G8eHttpContext(
            web_session_id="test-session",
            user_id="test-user",
            case_id="test-case",
            investigation_id="test-investigation",
            source_component=ComponentName.G8EE,
        )

        emitter = TribunalEmitter(event_service=mock_event_service, g8e_context=ctx)

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(
                pass_index=0, member=TribunalMember.AXIOM, candidate="ls", success=True
            ),
        )

        mock_event_service.publish.assert_called_once()

    @pytest.mark.asyncio
    async def test_round_2_peer_review_flow(self):
        """Test the full Round 2 peer review flow when consensus is low."""
        llm = LLMSettings(
            primary_provider=LLMProvider.OLLAMA,
            lite_provider=LLMProvider.OLLAMA,
            lite_model="gemma3:1b",
            llm_command_gen_passes=3,
            llm_command_gen_auditor=False,
        )
        settings = G8eeUserSettings(llm=llm)

        call_count = 0

        async def round_based_side_effect(**kwargs):
            nonlocal call_count
            call_count += 1
            mock_response = MagicMock()
            if call_count <= 3:
                # Round 1: No consensus
                if call_count == 1: mock_response.text = "cmd1"
                elif call_count == 2: mock_response.text = "cmd2"
                elif call_count == 3: mock_response.text = "cmd3"
            else:
                # Round 2: Consensus reached
                mock_response.text = "cmd_consensus"
            return mock_response

        mock_provider = _make_mock_provider(generate_content_lite_side_effect=round_based_side_effect)

        with patch("app.services.ai.generator.get_llm_provider", return_value=mock_provider):
            mock_event_service = MagicMock()
            mock_event_service.publish = AsyncMock()

            result = await generate_command(
                request="test round 2",
                guidelines="",
                operator_context=_make_mock_operator_context(),
                g8ed_event_service=mock_event_service,
                web_session_id="ws-1",
                user_id="user-1",
                case_id="case-1",
                investigation_id="inv-1",
                settings=settings,
                **_REPUTATION_KWARGS,
            )

            assert result.final_command == "cmd_consensus"
            assert result.round_2_candidates is not None
            assert len(result.round_2_candidates) == 3
            assert result.round_2_vote_breakdown is not None
            assert result.round_2_vote_breakdown.winner == "cmd_consensus"

            # Verify event emissions for Round 2
            emitted_types = [args[0][0].event_type for args in mock_event_service.publish.call_args_list]
            assert EventType.TRIBUNAL_VOTING_ROUND_2_STARTED in emitted_types
            assert EventType.TRIBUNAL_VOTING_ROUND_2_CONSENSUS_REACHED in emitted_types

    @pytest.mark.asyncio
    async def test_all_terminal_events_raise_on_publish_failure(self):
        """All TRIBUNAL_SESSION_* events are terminal and should re-raise on publish failure."""
        from app.models.agents.tribunal import (
            TribunalSessionModelNotConfiguredPayload,
            TribunalSessionStartedPayload,
            TribunalSessionSystemErrorPayload,
        )
        from app.models.http_context import G8eHttpContext
        from app.services.infra.g8ed_event_service import EventService

        mock_event_service = MagicMock(spec=EventService)
        mock_event_service.publish = AsyncMock(side_effect=RuntimeError("broker down"))

        ctx = G8eHttpContext(
            web_session_id="test-session",
            user_id="test-user",
            case_id="test-case",
            investigation_id="test-investigation",
            source_component=ComponentName.G8EE,
        )

        emitter = TribunalEmitter(event_service=mock_event_service, g8e_context=ctx)

        terminal_events = [
            (
                EventType.TRIBUNAL_SESSION_STARTED,
                TribunalSessionStartedPayload(
                    request="test",
                    model="gemma3:1b",
                    num_passes=3,
                    members=[TribunalMember.AXIOM],
                    correlation_id="test-corr-id",
                ),
            ),
            (
                EventType.TRIBUNAL_SESSION_COMPLETED,
                TribunalSessionCompletedPayload(
                    request="test",
                    final_command="test",
                    outcome=CommandGenerationOutcome.VERIFIED,
                    vote_score=1.0,
                ),
            ),
            (EventType.TRIBUNAL_SESSION_DISABLED, TribunalSessionDisabledPayload(request="test")),
            (
                EventType.TRIBUNAL_SESSION_MODEL_NOT_CONFIGURED,
                TribunalSessionModelNotConfiguredPayload(request="test", provider="ollama", error="model not configured"),
            ),
            (
                EventType.TRIBUNAL_SESSION_PROVIDER_UNAVAILABLE,
                TribunalSessionProviderUnavailablePayload(request="test", provider="ollama", error="provider unavailable"),
            ),
            (
                EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
                TribunalSessionSystemErrorPayload(request="test", pass_errors=["error"]),
            ),
            (
                EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
                TribunalSessionGenerationFailedPayload(request="test", pass_errors=["error"]),
            ),
            (
                EventType.TRIBUNAL_SESSION_AUDITOR_FAILED,
                TribunalAuditorFailedPayload(
                    request="test", reason=AuditorReason.AUDITOR_ERROR, error="error", candidate_command="ls"
                ),
            ),
        ]

        for event_type, payload in terminal_events:
            with pytest.raises(RuntimeError, match="broker down"):
                await emitter.emit(event_type, payload)


class TestDefaultConfigCoversAllMembers:
    """Regression test: default llm_command_gen_passes=5 must cover all five Tribunal members."""

    def test_default_pass_count_covers_all_members(self):
        """At default config (5 passes), all five Tribunal members must be assigned."""
        from app.models.settings import LLMSettings

        default_settings = LLMSettings()
        default_passes = default_settings.llm_command_gen_passes

        # Get the member assigned to each pass index at default config
        members_for_passes = [_member_for_pass(i) for i in range(default_passes)]

        # Verify we have exactly 5 passes (the new default)
        assert default_passes == 5, f"Default llm_command_gen_passes should be 5, got {default_passes}"

        # Verify all five distinct members are assigned
        expected_members = {
            TribunalMember.AXIOM,
            TribunalMember.CONCORD,
            TribunalMember.VARIANCE,
            TribunalMember.PRAGMA,
            TribunalMember.NEMESIS,
        }
        actual_members = set(members_for_passes)

        assert actual_members == expected_members, (
            f"Default config should assign all five Tribunal members. "
            f"Expected {expected_members}, got {actual_members}"
        )

        # Verify the cycling order matches expectations (Axiom, Concord, Variance, Pragma, Nemesis)
        expected_order = [
            TribunalMember.AXIOM,
            TribunalMember.CONCORD,
            TribunalMember.VARIANCE,
            TribunalMember.PRAGMA,
            TribunalMember.NEMESIS,
        ]
        assert members_for_passes == expected_order, (
            f"Member cycling order should be {expected_order}, got {members_for_passes}"
        )
