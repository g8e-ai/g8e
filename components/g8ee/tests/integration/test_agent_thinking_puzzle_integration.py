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

"""
Integration test to confirm AI agent thinking capabilities with a puzzle.
"""

import pytest
from collections.abc import AsyncGenerator
from unittest.mock import AsyncMock

from app.constants import (
    EventType,
    AgentMode,
    PromptFile,
    InvestigationStatus,
)
import app.llm.llm_types as types
from app.services.ai.agent import g8eEngine
from app.services.ai.tool_service import AIToolService
from app.services.operator.command_service import OperatorCommandService
from app.services.service_factory import ServiceFactory
from app.services.ai.grounding.web_search_provider import WebSearchProvider
from app.models.agent import AgentStreamContext
from app.models.http_context import G8eHttpContext
from app.models.investigations import EnrichedInvestigationContext
from app.models.model_configs import get_model_config
from tests.fakes.factories import (
    build_enriched_context,
    build_g8e_http_context,
)

pytestmark = [pytest.mark.integration, pytest.mark.ai_integration, pytest.mark.slow]


@pytest.mark.asyncio(loop_scope="session")
async def test_agent_thinking_puzzle(llm_provider, cache_aside_service, all_services):
    """
    Verify that the AI agent uses the thinking model correctly when solving a puzzle.
    
    This test presents a logic puzzle to the agent and checks that:
    1. The model emits THINKING SSE events with proper action types (START/UPDATE/END).
    2. Thinking content is delivered via LLM_CHAT_ITERATION_THINKING_STARTED events.
    3. The model eventually emits TEXT chunk events.
    4. Thinking events properly precede text events in the stream.
    5. The final response contains a reasonable answer to the puzzle.
    """
    from app.llm.factory import get_llm_settings
    llm = get_llm_settings()
    if not llm or not llm.primary_model:
        pytest.skip("LLM provider is not configured")

    model_name = llm.primary_model  # Use primary model with thinking support
    config = get_model_config(model_name)
    # Note: Let the test fail naturally if model doesn't support thinking chunks
    # This will provide better error information than skipping

    # Get real services from all_services fixture
    event_service = all_services['g8ed_event_service']
    operator_command_service = all_services['operator_command_service']
    tool_executor = all_services['tool_executor']
    agent = all_services['g8e_agent']
    
    puzzle_text = (
        "Solve this logic puzzle: You have two ropes. Each rope takes exactly 1 hour to burn, "
        "but they burn at inconsistent rates. How can you measure exactly 45 minutes using only these two ropes and a lighter? "
        "Please think step by step before answering."
    )
    
    contents = [types.Content(role="user", parts=[types.Part.from_text(text=puzzle_text)])]
    
    # Create investigation context for reference
    investigation_ctx = EnrichedInvestigationContext(
        id="inv-puzzle-1",
        case_id="case-puzzle-1",
        user_id="user-puzzle-1",
        case_title="Logic Puzzle Test",
        status=InvestigationStatus.OPEN,
        sentinel_mode=True,
        memory=None,
        operator_documents=[],
    )
    
    # Create g8e HTTP context
    g8e_context = build_g8e_http_context(
        case_id="case-puzzle-1",
        investigation_id="inv-puzzle-1",
        web_session_id="ws-puzzle-1",
        bound_operators=[],
    )
    
    # Create agent context (should be AgentStreamContext)
    from app.models.settings import G8eeUserSettings, LLMSettings
    agent_ctx = AgentStreamContext(
        case_id="case-puzzle-1",
        investigation_id="inv-puzzle-1",
        web_session_id="ws-puzzle-1",
        agent_mode=AgentMode.OPERATOR_NOT_BOUND,
        g8e_context=g8e_context,
        user_id="user-puzzle-1",
        investigation=investigation_ctx,
        request_settings=G8eeUserSettings(llm=LLMSettings()),
    )
    
    # Load system prompt
    from app.llm.prompts import load_prompt
    sys_prompt = load_prompt(PromptFile.CORE_IDENTITY)
    
    # Create generation config
    from app.services.ai.generation_config_builder import AIGenerationConfigBuilder
    gen_config = AIGenerationConfigBuilder.build_primary_settings(
        model=model_name,
        temperature=None,
        max_tokens=None,
        system_instruction=sys_prompt,
        tools=[],
    )
    
    # Mock the event service publish method to capture events
    published_events = []
    
    async def mock_publish(event):
        published_events.append(event)
        # Return success without calling real publish
        return "success"
    
    event_service.publish = mock_publish
    
    await agent.run_with_sse(
        contents=contents,
        generation_config=gen_config,
        model_name=model_name,
        agent_streaming_context=agent_ctx,
        context=agent_ctx,
        g8ed_event_service=event_service,
        llm_provider=llm_provider,
    )
    
    assert len(published_events) > 0, "Expected SSE events to be published"
    
    events = published_events
    
    # Check for thinking events
    thinking_events = [e for e in events if e.event_type == EventType.LLM_CHAT_ITERATION_THINKING_STARTED]
    assert len(thinking_events) > 0, "Expected at least one thinking event"
    
    # Verify thinking event structure and action types
    thinking_start_events = [e for e in thinking_events if e.payload.action_type == "start"]
    thinking_update_events = [e for e in thinking_events if e.payload.action_type == "update"]
    thinking_end_events = [e for e in thinking_events if e.payload.action_type == "end"]
    
    # Should have at least one START and one END event
    assert len(thinking_start_events) >= 1, "Expected at least one thinking START event"
    assert len(thinking_end_events) >= 1, "Expected at least one thinking END event"
    
    # Verify thinking content is present in UPDATE events
    if thinking_update_events:
        for event in thinking_update_events:
            assert event.payload.thinking is not None, "Thinking UPDATE events should contain thinking content"
            assert len(event.payload.thinking.strip()) > 0, "Thinking content should not be empty"
    
    # Check for text events
    text_events = [e for e in events if e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED]
    assert len(text_events) > 0, "Expected at least one text chunk event"
    
    full_text = "".join(e.payload.content for e in text_events if e.payload.content)
    assert len(full_text) > 0, "Expected non-empty text response"
    
    # We expect the answer to involve lighting both ends of one rope and one end of the other
    answer_keywords = ["both ends", "one end", "30 minutes", "15 minutes"]
    found_keywords = [kw for kw in answer_keywords if kw.lower() in full_text.lower()]
    assert len(found_keywords) >= 2, f"Expected answer to contain puzzle solution concepts, got: {full_text}"
    
    # Verify event ordering: thinking should come before text
    thinking_indices = [i for i, e in enumerate(events) if e.event_type == EventType.LLM_CHAT_ITERATION_THINKING_STARTED]
    text_indices = [i for i, e in enumerate(events) if e.event_type == EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED]
    
    if thinking_indices and text_indices:
        first_text_index = min(text_indices)
        last_thinking_end_index = max([i for i, e in enumerate(events) 
                                      if e.event_type == EventType.LLM_CHAT_ITERATION_THINKING_STARTED 
                                      and e.payload.action_type == "end"])
        
        # Thinking END should come before first TEXT chunk
        assert last_thinking_end_index < first_text_index, "Thinking END should precede text chunks"
