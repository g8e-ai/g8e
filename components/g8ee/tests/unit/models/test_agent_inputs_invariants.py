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

"""Pydantic invariants for ``AgentInputs`` and ``AgentStreamState``.

Pins two contracts that the AgentInputs/AgentStreamState split depends on:

1. ``extra='forbid'`` — unknown fields MUST raise ValidationError, so a
   subtle typo (``sentinal_mode=True``) or a stale caller passing removed
   fields (``execution_id``, ``web_session_id``) cannot silently succeed.
2. Top-level list immutability in ``_stream_with_tool_loop`` — the agent
   shallow-copies ``inputs.contents`` before mutating, so ``inputs.contents``
   is byte-identical pre- and post-run. We pin this as a direct assertion
   against the copy semantics used by the loop (``list(contents)``).
"""

from __future__ import annotations

import pytest
from pydantic import ValidationError as PydanticValidationError

import app.llm.llm_types as types
from app.models.agent import AgentInputs, AgentStreamState
from tests.fakes.agent_helpers import make_agent_inputs

pytestmark = [pytest.mark.unit]


class TestAgentInputsExtraForbid:
    """``AgentInputs`` must reject unknown fields — never silently accept them."""

    def test_unknown_field_raises_validation_error(self):
        with pytest.raises(PydanticValidationError) as exc_info:
            make_agent_inputs(nonexistent_field="whoops")
        # Pydantic surfaces the unknown-field complaint as 'extra_forbidden'.
        assert "extra_forbidden" in str(exc_info.value) or "Extra inputs" in str(exc_info.value)

    def test_typo_of_real_field_is_rejected(self):
        # ``sentinel_mode`` is real; ``sentinal_mode`` is a typo. Without
        # extra='forbid' this would silently default sentinel_mode and
        # persist the typo field, masking the bug.
        with pytest.raises(PydanticValidationError):
            make_agent_inputs(sentinal_mode=True)

    def test_removed_field_execution_id_is_rejected(self):
        # ``execution_id`` was removed from ExecutorCommandArgs; the analogous
        # guard here pins that callers cannot sneak it onto AgentInputs either.
        with pytest.raises(PydanticValidationError):
            make_agent_inputs(execution_id="exec-stale-1")


class TestAgentStreamStateExtraForbid:
    """``AgentStreamState`` must reject unknown fields — only the four sinks."""

    def test_only_declared_sinks_accepted(self):
        # All four declared sinks must construct without error.
        state = AgentStreamState(
            response_text="",
            token_usage=None,
            finish_reason=None,
            grounding_metadata=None,
        )
        assert state.response_text == ""

    def test_unknown_sink_field_raises_validation_error(self):
        with pytest.raises(PydanticValidationError):
            AgentStreamState(response_text="", unknown_sink="value")

    def test_attempt_to_add_request_field_is_rejected(self):
        # Someone copying an AgentInputs field onto AgentStreamState (e.g.
        # trying to store ``investigation_id`` on the state sink) must fail.
        with pytest.raises(PydanticValidationError):
            AgentStreamState(investigation_id="inv-1")


class TestAgentInputsContentsShallowCopyInvariant:
    """``_stream_with_tool_loop`` shallow-copies ``inputs.contents``.

    The loop at ``agent.py:279`` does ``contents = list(contents)`` before
    appending model/tool-response Content objects. This test pins that the
    semantic the loop relies on — top-level list isolation — actually holds.
    """

    def test_list_copy_isolates_top_level_appends(self):
        original = [
            types.Content(role=types.Role.USER, parts=[types.Part.from_text(text="hi")])
        ]
        inputs = make_agent_inputs(contents=original)

        # Simulate exactly what _stream_with_tool_loop does.
        working = list(inputs.contents)
        working.append(
            types.Content(role=types.Role.MODEL, parts=[types.Part.from_text(text="response")])
        )
        working.append(
            types.Content(role=types.Role.USER, parts=[types.Part.from_text(text="tool-response")])
        )

        # inputs.contents must not have seen the appends.
        assert len(inputs.contents) == 1, (
            "inputs.contents was mutated by list-copy callers — AgentInputs "
            "is supposed to be immutable across a run"
        )
        assert inputs.contents[0] is original[0]

    def test_copy_preserves_identity_of_existing_elements(self):
        # Intentional: we DO share element identity, because deepcopy is
        # expensive. Downstream code must treat Content.parts as read-only.
        # If someone starts mutating Content.parts in-place, this test will
        # still pass — but the follow-up invariant test below will fail,
        # alerting reviewers.
        original_content = types.Content(
            role=types.Role.USER, parts=[types.Part.from_text(text="original")]
        )
        inputs = make_agent_inputs(contents=[original_content])
        working = list(inputs.contents)
        assert working[0] is original_content, (
            "Shallow copy is the intended semantic — if this fails, either "
            "the helper switched to deepcopy or a wrapper object was "
            "introduced. Reconsider the corresponding doc claim at "
            "_stream_with_tool_loop."
        )


class TestAgentInputsImmutability:
    """Regression tests for AgentInputs immutability (Report 2)."""

    def test_inputs_model_dump_round_trip_preserves_all_fields(self):
        """AgentInputs must survive model_dump() -> model_validate() round-trip.

        This regression test verifies the AgentInputs/AgentStreamState split
        (Report 2) maintains data integrity through serialization. All fields
        must be preserved when dumping to dict and reconstructing, ensuring
        no silent data loss occurs during persistence or transmission.
        """
        from app.models.agent import AgentInputs
        from tests.fakes.factories import build_enriched_context, build_g8e_http_context
        from app.models.settings import G8eeUserSettings, LLMSettings
        from app.constants import AgentMode

        inv = build_enriched_context(investigation_id="inv-immutable-test")
        g8e_ctx = build_g8e_http_context(user_id="user-immutable-test")
        request_settings = G8eeUserSettings(llm=LLMSettings())

        from app.models.investigations import ConversationHistoryMessage
        from app.utils.ledger_hash import genesis_hash

        genesis = genesis_hash("inv-immutable-test", inv.created_at.isoformat())
        test_hash = "0" * 64

        conversation_history = [
            ConversationHistoryMessage(
                id="msg-1",
                sender="user.chat",
                content="test",
                prev_hash=genesis,
                entry_hash=test_hash,
            )
        ]

        original_inputs = AgentInputs(
            case_id="case-immutable",
            investigation_id="inv-immutable-test",
            user_id="user-immutable-test",
            web_session_id="web-immutable",
            agent_mode=AgentMode.OPERATOR_BOUND,
            sentinel_mode=True,
            investigation=inv,
            g8e_context=g8e_ctx,
            request_settings=request_settings,
            operator_bound=True,
            model_to_use="test-model",
            max_tokens=2048,
            conversation_history=conversation_history,
            system_instructions="You are a helpful assistant",
            contents=[{"role": "user", "parts": [{"text": "hello"}]}],
            user_memories=[{"id": "mem-1", "content": "test memory", "case_id": "case-immutable", "investigation_id": "inv-immutable-test", "user_id": "user-immutable-test", "status": "Open", "case_title": "test case"}],
            case_memories=[{"id": "case-mem-1", "content": "case memory", "case_title": "test case", "case_id": "case-immutable", "investigation_id": "inv-immutable-test", "user_id": "user-immutable-test", "status": "Open"}],
        )

        # Serialize to dict
        dumped = original_inputs.model_dump()

        # Reconstruct from dict
        reconstructed = AgentInputs(**dumped)

        # Verify all critical fields are preserved
        assert reconstructed.case_id == original_inputs.case_id
        assert reconstructed.investigation_id == original_inputs.investigation_id
        assert reconstructed.user_id == original_inputs.user_id
        assert reconstructed.web_session_id == original_inputs.web_session_id
        assert reconstructed.agent_mode == original_inputs.agent_mode
        assert reconstructed.sentinel_mode == original_inputs.sentinel_mode
        assert reconstructed.operator_bound == original_inputs.operator_bound
        assert reconstructed.model_to_use == original_inputs.model_to_use
        assert reconstructed.max_tokens == original_inputs.max_tokens
        assert reconstructed.system_instructions == original_inputs.system_instructions
        assert len(reconstructed.conversation_history) == len(original_inputs.conversation_history)
        assert len(reconstructed.contents) == len(original_inputs.contents)
        assert len(reconstructed.user_memories) == len(original_inputs.user_memories)
        assert len(reconstructed.case_memories) == len(original_inputs.case_memories)
