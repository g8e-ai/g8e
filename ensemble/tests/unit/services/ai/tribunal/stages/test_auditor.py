# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import AuditorReason, CommandGenerationOutcome, EventType
from app.constants.generated_status import ConsensusAuditMode
from app.errors import ContextWindowExceededError
from app.llm.llm_types import Candidate, Content, GenerateContentResponse, Part, Role, UsageMetadata
from app.models.agents.tribunal import TribunalAuditorFailedError, VoteBreakdown
from app.models.http_context import RequestContext
from app.services.ai.auditor_service import run_auditor
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.auditor import TribunalAuditor
from app.utils.agent_persona_loader import get_agent_persona


@pytest.mark.asyncio
class TestRunAuditStage:
    async def test_auditor_disabled_returns_consensus(
        self, mock_g8e_context, mock_operator_context, mock_reputation_service
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        emitter = TribunalEmitter(None, mock_g8e_context)
        auditor = TribunalAuditor(
            emitter=emitter,
            reputation_data_service=mock_reputation_service,
        )

        result = await auditor.run(
            provider=MagicMock(),
            model="test-model",
            request="list files",
            guidelines="",
            vote_winner="ls -la",
            vote_breakdown=vote_breakdown,
            tied_candidates=None,
            operator_context=mock_operator_context,
            auditor_enabled=False,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-1",
            context=RequestContext(
                web_session_id="test-web-session",
                user_id="test-user",
                investigation_id="inv-1",
            ),
        )

        assert result.final_command == "ls -la"
        assert result.outcome == CommandGenerationOutcome.CONSENSUS
        assert result.passed is True
        assert result.revision is None
        assert result.reason == AuditorReason.OK

    async def test_auditor_approves_returns_verified(
        self, make_mock_provider, mock_g8e_context, mock_operator_context, mock_reputation_service
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        mock_response = MagicMock()
        mock_response.text = '{"status": "ok"}'
        mock_provider = make_mock_provider(generate_content_lite_return=mock_response)
        emitter = TribunalEmitter(None, mock_g8e_context)
        emitter.correlation_id = "tribunal_test_command"
        auditor = TribunalAuditor(
            emitter=emitter,
            reputation_data_service=mock_reputation_service,
        )

        result = await auditor.run(
            provider=mock_provider,
            model="test-model",
            request="list files",
            guidelines="",
            vote_winner="ls -la",
            vote_breakdown=vote_breakdown,
            tied_candidates=None,
            operator_context=mock_operator_context,
            auditor_enabled=True,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-1",
            context=RequestContext(
                web_session_id="test-web-session",
                user_id="test-user",
                investigation_id="inv-1",
            ),
        )

        assert result.final_command == "ls -la"
        assert result.outcome == CommandGenerationOutcome.VERIFIED
        assert result.passed is True
        assert result.revision is None
        assert result.reason == AuditorReason.OK
        assert result.reputation_commitment_id is not None

    async def test_auditor_retries_emit_one_model_call_observation_per_attempt(
        self, make_mock_provider, mock_g8e_context, mock_operator_context, mock_reputation_service
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        responses = [
            GenerateContentResponse(
                candidates=[Candidate(content=Content(role=Role.MODEL, parts=[Part(text="invalid")]), finish_reason="stop")],
                usage_metadata=UsageMetadata(prompt_token_count=10, candidates_token_count=2, total_token_count=12),
            ),
            GenerateContentResponse(
                candidates=[Candidate(content=Content(role=Role.MODEL, parts=[Part(text='{"status": "ok"}')]), finish_reason="stop")],
                usage_metadata=UsageMetadata(prompt_token_count=12, candidates_token_count=4, total_token_count=16),
            ),
        ]
        provider = make_mock_provider(generate_content_lite_side_effect=responses)
        event_service = MagicMock()
        event_service.publish = AsyncMock()
        emitter = TribunalEmitter(event_service, mock_g8e_context, correlation_id="tribunal-test")
        auditor = TribunalAuditor(
            emitter=emitter,
            reputation_data_service=mock_reputation_service,
        )

        await auditor.run(
            provider=provider,
            model="test-model",
            request="list files",
            guidelines="",
            vote_winner="ls -la",
            vote_breakdown=vote_breakdown,
            tied_candidates=None,
            operator_context=mock_operator_context,
            auditor_enabled=True,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-1",
            context=RequestContext(
                web_session_id="test-web-session",
                user_id="test-user",
                investigation_id="inv-1",
            ),
        )

        completed_event = next(
            call.args[0]
            for call in event_service.publish.await_args_list
            if call.args[0].event_type == EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED
        )
        model_calls = completed_event.payload.model_calls
        assert len(model_calls) == 2
        assert [call.retry_count for call in model_calls] == [0, 1]
        assert [call.input_tokens for call in model_calls] == [10, 12]
        assert [call.output_tokens for call in model_calls] == [2, 4]
        assert all(call.agent_role == "auditor" for call in model_calls)
        assert all(call.model == "test-model" for call in model_calls)
        assert all(call.monotonic_end >= call.monotonic_start for call in model_calls)
        assert all(call.input_artifact_hash for call in model_calls)
        assert all(call.output_artifact_hash for call in model_calls)

    @pytest.mark.parametrize(
        ("kwargs", "expected_role"),
        [
            pytest.param({}, "primary", id="auditor-runs-on-the-primary-tier-by-default"),
            pytest.param({"model_role": "lite"}, "lite", id="fallback-to-the-lite-provider-is-reported"),
        ],
    )
    async def test_auditor_attributes_its_calls_to_the_tier_it_was_resolved_from(
        self,
        make_mock_provider,
        mock_g8e_context,
        mock_operator_context,
        mock_reputation_service,
        kwargs,
        expected_role,
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        response = GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(role=Role.MODEL, parts=[Part(text='{"status": "ok"}')]),
                    finish_reason="stop",
                )
            ],
            usage_metadata=UsageMetadata(prompt_token_count=10, candidates_token_count=2, total_token_count=12),
        )
        provider = make_mock_provider(generate_content_lite_side_effect=[response])
        event_service = MagicMock()
        event_service.publish = AsyncMock()
        emitter = TribunalEmitter(event_service, mock_g8e_context, correlation_id="tribunal-test")
        auditor = TribunalAuditor(
            emitter=emitter,
            reputation_data_service=mock_reputation_service,
        )

        await auditor.run(
            provider=provider,
            model="test-model",
            request="list files",
            guidelines="",
            vote_winner="ls -la",
            vote_breakdown=vote_breakdown,
            tied_candidates=None,
            operator_context=mock_operator_context,
            auditor_enabled=True,
            command_constraints_message="No whitelist or blacklist constraints are active.",
            investigation_id="inv-1",
            context=RequestContext(
                web_session_id="test-web-session",
                user_id="test-user",
                investigation_id="inv-1",
            ),
            **kwargs,
        )

        completed_event = next(
            call.args[0]
            for call in event_service.publish.await_args_list
            if call.args[0].event_type == EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED
        )
        assert [call.model_role for call in completed_event.payload.model_calls] == [expected_role]

    async def test_auditor_empty_responses_emit_failed_model_call_observations(
        self, make_mock_provider, mock_g8e_context, mock_operator_context, mock_reputation_service
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        empty_response = GenerateContentResponse(
            candidates=[Candidate(content=Content(role=Role.MODEL), finish_reason="stop")],
            usage_metadata=UsageMetadata(prompt_token_count=10, total_token_count=10),
        )
        provider = make_mock_provider(
            generate_content_lite_side_effect=[empty_response, empty_response]
        )
        event_service = MagicMock()
        event_service.publish = AsyncMock()
        emitter = TribunalEmitter(event_service, mock_g8e_context, correlation_id="tribunal-test")
        auditor = TribunalAuditor(
            emitter=emitter,
            reputation_data_service=mock_reputation_service,
        )

        with pytest.raises(TribunalAuditorFailedError):
            await auditor.run(
                provider=provider,
                model="test-model",
                request="list files",
                guidelines="",
                vote_winner="ls -la",
                vote_breakdown=vote_breakdown,
                tied_candidates=None,
                operator_context=mock_operator_context,
                auditor_enabled=True,
                command_constraints_message="No whitelist or blacklist constraints are active.",
                investigation_id="inv-1",
                context=RequestContext(
                    web_session_id="test-web-session",
                    user_id="test-user",
                    investigation_id="inv-1",
                ),
            )

        failed_event = next(
            call.args[0]
            for call in event_service.publish.await_args_list
            if call.args[0].event_type == EventType.AI_CONSENSUS_SESSION_AUDITOR_FAILED
        )
        model_calls = failed_event.payload.model_calls
        assert len(model_calls) == 2
        assert [call.retry_count for call in model_calls] == [0, 1]
        assert all(call.succeeded is False for call in model_calls)
        assert all(call.error_type == "OllamaEmptyResponseError" for call in model_calls)
        assert all(call.monotonic_end >= call.monotonic_start for call in model_calls)
        assert all(call.input_artifact_hash for call in model_calls)


def _context_overflow_error() -> ContextWindowExceededError:
    return ContextWindowExceededError(
        "prompt filled the context window",
        model="test-model",
        service_name="ollama",
        num_ctx=32768,
        prompt_tokens=32768,
        channel="lite",
    )


@pytest.mark.asyncio
class TestAuditorContextOverflow:
    async def test_overflow_fails_auditor_once_with_overflow_reason(
        self, make_mock_provider, mock_g8e_context, mock_operator_context, mock_reputation_service
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        provider = make_mock_provider(generate_content_lite_side_effect=_context_overflow_error())
        event_service = MagicMock()
        event_service.publish = AsyncMock()
        emitter = TribunalEmitter(event_service, mock_g8e_context, correlation_id="tribunal-test")
        auditor = TribunalAuditor(
            emitter=emitter,
            reputation_data_service=mock_reputation_service,
        )

        with pytest.raises(TribunalAuditorFailedError) as exc_info:
            await auditor.run(
                provider=provider,
                model="test-model",
                request="list files",
                guidelines="",
                vote_winner="ls -la",
                vote_breakdown=vote_breakdown,
                tied_candidates=None,
                operator_context=mock_operator_context,
                auditor_enabled=True,
                command_constraints_message="No whitelist or blacklist constraints are active.",
                investigation_id="inv-1",
                context=RequestContext(
                    web_session_id="test-web-session",
                    user_id="test-user",
                    investigation_id="inv-1",
                ),
            )

        assert exc_info.value.reason == AuditorReason.CONTEXT_OVERFLOW
        assert "context window" in exc_info.value.error
        assert "empty" not in exc_info.value.error.lower()
        # Retrying the same prompt cannot succeed.
        assert provider.generate_content_lite.await_count == 1

        failed_event = next(
            call.args[0]
            for call in event_service.publish.await_args_list
            if call.args[0].event_type == EventType.AI_CONSENSUS_SESSION_AUDITOR_FAILED
        )
        assert failed_event.payload.reason == AuditorReason.CONTEXT_OVERFLOW
        assert [call.error_type for call in failed_event.payload.model_calls] == [
            "ContextWindowExceededError"
        ]

    async def test_deprecated_run_auditor_fails_once_with_overflow_reason(
        self, make_mock_provider, mock_g8e_context
    ):
        vote_breakdown = VoteBreakdown(
            candidates_by_member={},
            candidates_by_command={"ls -la": ["axiom"]},
            winner="ls -la",
            winner_supporters=["axiom"],
            dissenters_by_command={},
            consensus_strength=1.0,
        )
        provider = make_mock_provider(generate_content_lite_side_effect=_context_overflow_error())
        emitter = TribunalEmitter(None, mock_g8e_context)

        with pytest.raises(TribunalAuditorFailedError) as exc_info:
            await run_auditor(
                provider=provider,
                model="test-model",
                request="list files",
                guidelines="",
                mode=ConsensusAuditMode.UNANIMOUS,
                vote_winner="ls -la",
                vote_breakdown=vote_breakdown,
                tied_candidates=None,
                operator_context=None,
                emitter=emitter,
                command_constraints_message="No whitelist or blacklist constraints are active.",
                auditor_persona=get_agent_persona("auditor"),
            )

        assert exc_info.value.reason == AuditorReason.CONTEXT_OVERFLOW
        assert "context window" in exc_info.value.error
        assert provider.generate_content_lite.await_count == 1
