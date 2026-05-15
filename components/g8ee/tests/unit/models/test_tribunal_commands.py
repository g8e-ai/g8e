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

from datetime import datetime, UTC
import pytest
from pydantic import ValidationError

from app.constants import (
    CommandGenerationOutcome,
    AuditorReason,
    TribunalMember
)
from app.models.agents.tribunal import CandidateCommand, VoteBreakdown
from app.models.tribunal_commands import (
    TribunalCommand,
    TribunalCommandRequestContext,
    TribunalCommandGenerationResult,
    TribunalCommandAuditor,
    TribunalCommandPipelineMetadata,
    TribunalCommandErrorContext
)

pytestmark = [pytest.mark.unit]

class TestTribunalCommandModels:
    def test_tribunal_command_request_context(self):
        ctx = TribunalCommandRequestContext(
            request="list files",
            guidelines="use ls -la",
            model="gpt-4o",
            num_passes=5,
            rounds_executed=1
        )
        assert ctx.request == "list files"
        assert ctx.guidelines == "use ls -la"
        assert ctx.model == "gpt-4o"
        assert ctx.num_passes == 5
        assert ctx.rounds_executed == 1

        # Test defaults
        ctx_default = TribunalCommandRequestContext(request="minimal")
        assert ctx_default.guidelines == ""
        assert ctx_default.model is None
        assert ctx_default.num_passes is None
        assert ctx_default.rounds_executed == 1

    def test_tribunal_command_generation_result(self):
        res = TribunalCommandGenerationResult(
            final_command="ls -la",
            outcome=CommandGenerationOutcome.CONSENSUS,
            vote_winner="ls -la",
            vote_score=1.0
        )
        assert res.final_command == "ls -la"
        assert res.outcome == CommandGenerationOutcome.CONSENSUS
        assert res.vote_winner == "ls -la"
        assert res.vote_score == 1.0

    def test_tribunal_command_auditor(self):
        auditor = TribunalCommandAuditor(
            auditor_passed=True,
            auditor_reason=AuditorReason.OK,
            reputation_commitment_id="rep-123"
        )
        assert auditor.auditor_passed is True
        assert auditor.auditor_reason == AuditorReason.OK
        assert auditor.reputation_commitment_id == "rep-123"

        # Test swap fields
        auditor_swap = TribunalCommandAuditor(
            auditor_passed=False,
            auditor_reason=AuditorReason.SWAPPED_TO_DISSENTER,
            swap_to_cluster="cluster-1",
            swap_to_member="member-1"
        )
        assert auditor_swap.swap_to_cluster == "cluster-1"
        assert auditor_swap.swap_to_member == "member-1"

    def test_tribunal_command_pipeline_metadata(self):
        meta = TribunalCommandPipelineMetadata(
            consensus_confidence="unanimous_verified",
            execution_duration_ms=500,
            stage_1_duration_ms=200,
            stage_2_duration_ms=100,
            stage_3_duration_ms=200
        )
        assert meta.consensus_confidence == "unanimous_verified"
        assert meta.execution_duration_ms == 500
        assert meta.stage_1_duration_ms == 200
        assert meta.stage_2_duration_ms == 100
        assert meta.stage_3_duration_ms == 200

    def test_tribunal_command_error_context(self):
        err = TribunalCommandErrorContext(
            error_type="ProviderUnavailable",
            error_message="Connection lost",
            pass_errors=["fail 1", "fail 2"]
        )
        assert err.error_type == "ProviderUnavailable"
        assert err.error_message == "Connection lost"
        assert err.pass_errors == ["fail 1", "fail 2"]

    def test_tribunal_command_full_model(self):
        now_dt = datetime.now(UTC)
        cmd = TribunalCommand(
            investigation_id="inv-123",
            case_id="case-123",
            created_at=now_dt,
            request_context=TribunalCommandRequestContext(request="test"),
            generation_result=TribunalCommandGenerationResult(
                outcome=CommandGenerationOutcome.CONSENSUS,
                final_command="echo hello"
            ),
            candidates=[
                CandidateCommand(
                    command="echo hello",
                    pass_index=0,
                    member=TribunalMember.AXIOM,
                    reasoning="seems correct"
                )
            ],
            vote_breakdown=VoteBreakdown(
                candidates_by_member={TribunalMember.AXIOM.value: "echo hello"},
                candidates_by_command={"echo hello": [TribunalMember.AXIOM.value]},
                winner="echo hello",
                winner_supporters=[TribunalMember.AXIOM.value],
                consensus_strength=1.0
            )
        )

        assert cmd.investigation_id == "inv-123"
        assert cmd.case_id == "case-123"
        assert cmd.request_context.request == "test"
        assert len(cmd.candidates) == 1
        assert cmd.vote_breakdown.winner == "echo hello"

        # Test serialization
        dumped = cmd.model_dump(mode="json")
        assert dumped["investigation_id"] == "inv-123"
        assert dumped["case_id"] == "case-123"
        assert "created_at" in dumped
        assert isinstance(dumped["created_at"], str) # ISO format from UTCDatetime

        # Test deserialization
        cmd2 = TribunalCommand.model_validate(dumped)
        assert cmd2.id == cmd.id
        assert cmd2.investigation_id == cmd.investigation_id

    def test_tribunal_command_validation_error(self):
        with pytest.raises(ValidationError):
            # Missing required investigation_id
            TribunalCommand(
                case_id="case-123",
                created_at=datetime.now(UTC),
                request_context=TribunalCommandRequestContext(request="test"),
                generation_result=TribunalCommandGenerationResult(
                    outcome=CommandGenerationOutcome.CONSENSUS
                )
            )

    def test_tribunal_command_error_state(self):
        cmd = TribunalCommand(
            investigation_id="inv-123",
            case_id="case-123",
            created_at=datetime.now(UTC),
            request_context=TribunalCommandRequestContext(request="test"),
            generation_result=TribunalCommandGenerationResult(
                outcome=CommandGenerationOutcome.CONSENSUS_FAILED
            ),
            error_context=TribunalCommandErrorContext(
                error_type="ConsensusFailed",
                error_message="No agreement",
                pass_errors=["pass 0: echo a", "pass 1: echo b"]
            )
        )
        assert cmd.generation_result.outcome == CommandGenerationOutcome.CONSENSUS_FAILED
        assert cmd.error_context.error_type == "ConsensusFailed"
        assert len(cmd.error_context.pass_errors) == 2
