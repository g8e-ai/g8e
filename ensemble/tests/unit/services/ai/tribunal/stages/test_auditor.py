# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import AsyncMock, MagicMock

import pytest

from app.constants import AuditorReason, CommandGenerationOutcome, EventType
from app.llm.llm_types import Candidate, Content, GenerateContentResponse, Part, Role, UsageMetadata
from app.models.agents.tribunal import TribunalAuditorFailedError, VoteBreakdown
from app.models.http_context import RequestContext
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.stages.auditor import TribunalAuditor


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
            auditor_hmac_key="a" * 64,
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
            auditor_hmac_key="a" * 64,
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
            auditor_hmac_key="a" * 64,
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
            auditor_hmac_key="a" * 64,
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
