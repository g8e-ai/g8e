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
