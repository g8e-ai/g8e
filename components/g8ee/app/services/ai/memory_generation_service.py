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

import logging
import re

import app.llm.llm_types as types
from app.constants.message_sender import MessageSender
from app.errors import OllamaEmptyResponseError
from app.llm import get_llm_provider, Role
from app.llm.structured import parse_structured_response
from app.utils.agent_persona_loader import get_agent_persona
from app.models.settings import G8eeUserSettings
from app.models.investigations import ConversationHistoryMessage, InvestigationModel
from app.models.memory import InvestigationMemory, MemoryAnalysis
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder
from app.services.protocols import MemoryDataServiceProtocol

logger = logging.getLogger(__name__)

CONVERSATION_HISTORY_LIMIT = 20
FALLBACK_TEXT_LIMIT = 2000


class MemoryGenerationService:
    """AI-backed memory analysis — updates an InvestigationMemory from conversation history.

    Depends on MemoryDataService for all persistence. Does not touch the DB or KV
    directly — all reads and writes are delegated to MemoryDataService.
    """

    def __init__(self, memory_crud: MemoryDataServiceProtocol) -> None:
        self._memory_crud = memory_crud

    async def update_memory_from_conversation(
        self,
        conversation_history: list[ConversationHistoryMessage],
        investigation: InvestigationModel,
        settings: G8eeUserSettings,
    ) -> InvestigationMemory:
        investigation_id = investigation.id

        logger.info(
            "Starting AI memory update for investigation %s",
            investigation_id,
            extra={
                "investigation_id": investigation_id,
                "conversation_length": len(conversation_history),
                "operation": "memory_update_start",
            },
        )

        existing = await self._memory_crud.get_memory(investigation_id)
        is_new = existing is None

        if not conversation_history:
            logger.info(
                "Skipping AI memory update for %s: no conversation history",
                investigation_id,
                extra={"investigation_id": investigation_id, "operation": "memory_update_skipped"},
            )
            if existing is not None:
                return existing
            return await self._memory_crud.create_memory(investigation)

        if is_new:
            memory = InvestigationMemory(
                case_id=investigation.case_id,
                investigation_id=investigation.id,
                user_id=investigation.user_id,
                status=investigation.status,
                case_title=investigation.case_title,
            )
        else:
            logger.info(
                "Retrieved existing memory for investigation %s",
                investigation_id,
                extra={
                    "investigation_id": investigation_id,
                    "status": existing.status,
                    "operation": "memory_retrieved",
                },
            )
            memory = existing

        await self._ai_update_memory(memory, conversation_history, settings)
        memory.status = investigation.status
        memory.update_timestamp()

        await self._memory_crud.save_memory(memory, is_new=is_new)

        logger.info(
            "Memory updated for investigation %s",
            investigation_id,
            extra={
                "investigation_id": investigation_id,
                "case_id": memory.case_id,
                "user_id": memory.user_id,
                "status": memory.status,
                "has_preferences": bool(memory.communication_preferences or memory.technical_background),
                "operation": "memory_update_complete",
            },
        )

        return memory

    async def _ai_update_memory(
        self,
        memory: InvestigationMemory,
        conversation_history: list[ConversationHistoryMessage],
        settings: G8eeUserSettings,
    ) -> None:
        contents = self._conversation_to_contents(conversation_history, memory)

        memory_persona = get_agent_persona("codex")
        system_instructions = f"You are analyzing a technical support conversation for case: {memory.case_title}. {memory_persona.get_system_prompt()}"

        assistant_model = settings.llm.resolved_assistant_model
        if not assistant_model:
            logger.warning("[MEMORY-GEN] No assistant_model configured, skipping AI memory update")
            return

        provider = get_llm_provider(settings.llm, is_assistant=True)

        config = AIGenerationConfigBuilder.build_assistant_settings(
            model=assistant_model,
            max_tokens=None,
            system_instructions=system_instructions,
            response_format=types.ResponseFormat.from_pydantic_schema(MemoryAnalysis.model_json_schema()),
        )
        try:
            response = await provider.generate_content_assistant(
                model=assistant_model,
                contents=contents,
                assistant_llm_settings=config,
            )
            if response.text is None:
                raise OllamaEmptyResponseError(
                    "LLM returned empty response",
                    model=assistant_model,
                    channel="assistant",
                    done_reason="stop",
                    prompt_eval_count=None,
                    eval_count=None,
                    num_ctx=0,
                    num_predict=0,
                    thinking_len=0,
                    tool_calls_count=0,
                    ctx_overflow_suspected=False,
                )
            ai_analysis = self._parse_memory_analysis(response.text)
        except OllamaEmptyResponseError as exc:
            logger.warning(
                "AI response was empty during memory update for %s, skipping preference update: %s",
                memory.investigation_id,
                exc,
            )
            return

        if not any([
            ai_analysis.investigation_summary,
            ai_analysis.communication_preferences,
            ai_analysis.technical_background,
            ai_analysis.response_style,
            ai_analysis.problem_solving_approach,
            ai_analysis.interaction_style,
        ]):
            logger.warning(
                "AI response for memory update contained no extractable data for %s",
                memory.investigation_id,
            )
            return

        memory.investigation_summary = ai_analysis.investigation_summary or memory.investigation_summary
        memory.communication_preferences = ai_analysis.communication_preferences or memory.communication_preferences
        memory.technical_background = ai_analysis.technical_background or memory.technical_background
        memory.response_style = ai_analysis.response_style or memory.response_style
        memory.problem_solving_approach = ai_analysis.problem_solving_approach or memory.problem_solving_approach
        memory.interaction_style = ai_analysis.interaction_style or memory.interaction_style

        logger.info("AI preference analysis successful for %s", memory.investigation_id)

    @staticmethod
    def _conversation_to_contents(
        conversation_history: list[ConversationHistoryMessage],
        memory: InvestigationMemory,
    ) -> list[types.Content]:
        contents: list[types.Content] = []
        
        # Add existing memory context first
        memory_context = (
            f"CURRENT MEMORY STATE:\n"
            f"Investigation Summary: {memory.investigation_summary or 'None'}\n"
            f"Technical Background: {memory.technical_background or 'None'}\n"
            f"Communication Preferences: {memory.communication_preferences or 'None'}\n"
            f"Response Style: {memory.response_style or 'None'}\n"
            f"Problem Solving Approach: {memory.problem_solving_approach or 'None'}\n"
            f"Interaction Style: {memory.interaction_style or 'None'}\n\n"
            f"CRITICAL TASK: Update ALL memory fields based on the conversation.\n"
            f"The conversation may introduce NEW TOPICS including equipment, systems, logs, and locations.\n"
            f"You MUST ADD these new concepts to the technical background and other relevant fields.\n"
            f"Do NOT ignore new equipment types, locations, or technical terms mentioned in the conversation.\n"
            f"MERGE existing knowledge with NEW conversation details - both should be present in the updated memory."
        )
        contents.append(types.Content(
            role=Role.USER,
            parts=[types.Part.from_text(text=memory_context)],
        ))
        
        # Add conversation history
        for msg in conversation_history[-CONVERSATION_HISTORY_LIMIT:]:
            if not msg.content or msg.metadata.is_thinking:
                continue
            if msg.sender == MessageSender.USER_CHAT:
                role = "user"
            elif msg.sender in (MessageSender.AI_PRIMARY, MessageSender.AI_ASSISTANT):
                role = "model"
            else:
                continue
            contents.append(types.Content(
                role=Role.USER if role == "user" else Role.MODEL,
                parts=[types.Part.from_text(text=msg.content)],
            ))
        
        # Add the analysis request
        contents.append(types.Content(
            role=Role.USER,
            parts=[types.Part.from_text(text="Analyze the conversation above and populate the memory fields. Return a JSON object with these fields:\n- \"investigation_summary\": high-level summary (no hostnames/IPs)\n- \"communication_preferences\": how the user prefers to communicate\n- \"technical_background\": user's technical experience and skills\n- \"response_style\": how they want information presented\n- \"problem_solving_approach\": how they debug and investigate\n- \"interaction_style\": meta-preferences about questions and context\nAll fields are optional but try to populate each one.")],
        ))
        return contents

    @staticmethod
    def _extract_json_from_markdown(text: str) -> str | None:
        """Extract JSON from markdown code blocks if present."""
        json_pattern = r'```(?:json)?\s*\n?(.*?)\n?```'
        matches = re.findall(json_pattern, text, re.DOTALL | re.IGNORECASE)
        for match in matches:
            stripped = match.strip()
            if stripped.startswith('{') or stripped.startswith('['):
                return stripped
        return None

    @staticmethod
    def _extract_key_value_pairs(text: str) -> dict[str, str]:
        """Extract key-value pairs from plain text/markdown as last resort."""
        result: dict[str, str] = {}
        lines = text.split('\n')
        current_key: str | None = None
        current_value: list[str] = []

        for line in lines:
            line = line.strip()
            if not line:
                continue
            if line.startswith('#') or line.startswith('//') or line.startswith('/*'):
                continue
            # Look for key: value pattern, handling quoted keys like "key": "value"
            if ':' in line and not line.startswith('-'):
                if current_key and current_value:
                    result[current_key] = ' '.join(current_value).strip()
                key_part = line.split(':', 1)[0].strip()
                # Remove quotes from key if present
                key_lower = key_part.lower().replace(' ', '_').strip('"\'')
                value_part = line.split(':', 1)[1].strip()
                # Remove trailing commas
                if value_part.endswith(','):
                    value_part = value_part[:-1].strip()
                if value_part.startswith('"') and value_part.endswith('"'):
                    value_part = value_part[1:-1]
                if value_part.startswith("'") and value_part.endswith("'"):
                    value_part = value_part[1:-1]
                current_key = key_lower
                current_value = [value_part] if value_part else []
            elif current_key:
                # Remove trailing commas from continuation lines
                if line.endswith(','):
                    line = line[:-1].strip()
                current_value.append(line)

        if current_key and current_value:
            result[current_key] = ' '.join(current_value).strip()

        return result

    def _parse_memory_analysis(self, text: str) -> MemoryAnalysis:
        """Parse AI response into MemoryAnalysis with multiple fallback strategies.

        Strategy:
        1. Delegate JSON recovery (direct, fenced, substring, truncated) to
           the shared structured-response parser. Bare-value coercion is
           disabled because MemoryAnalysis has zero required fields.
        2. Fallback: extract key-value pairs from plain text/markdown.
        3. Final fallback: store raw text in investigation_summary.
        """
        text = text.strip()
        if not text:
            return MemoryAnalysis()

        try:
            return parse_structured_response(text, MemoryAnalysis, allow_bare_value=False)
        except Exception:
            pass

        # Key-value text fallback
        kv_pairs = self._extract_key_value_pairs(text)
        if kv_pairs:
            mapped = MemoryAnalysis()
            field_map = {
                'investigation_summary': ['investigation_summary', 'summary', 'investigation', 'context'],
                'communication_preferences': ['communication_preferences', 'communication', 'preferences', 'how_the_user_prefers_to_communicate'],
                'technical_background': ['technical_background', 'technical', 'background', 'experience', 'skills'],
                'response_style': ['response_style', 'response', 'style', 'format', 'how_the_user_wants_information_presented'],
                'problem_solving_approach': ['problem_solving_approach', 'problem_solving', 'approach', 'debugging', 'how_the_user_approaches_debugging'],
                'interaction_style': ['interaction_style', 'interaction', 'meta_preferences', 'questions', 'context'],
            }
            for field, aliases in field_map.items():
                for alias in aliases:
                    if alias in kv_pairs:
                        setattr(mapped, field, kv_pairs[alias])
                        break
            if any([
                mapped.investigation_summary,
                mapped.communication_preferences,
                mapped.technical_background,
                mapped.response_style,
                mapped.problem_solving_approach,
                mapped.interaction_style,
            ]):
                return mapped

        # Strategy 4: Fallback - raw text becomes investigation_summary
        cleaned = re.sub(r'^\s*[{\[]\s*', '', text)
        cleaned = re.sub(r'\s*[}\]]\s*$', '', cleaned)
        cleaned = cleaned.strip()
        if cleaned:
            return MemoryAnalysis(investigation_summary=cleaned[:FALLBACK_TEXT_LIMIT])

        return MemoryAnalysis()
