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
AI Triage Agent

Classifies incoming user messages as 'simple' or 'complex' using the lightweight
assistant model before committing to the full main LLM.
"""

import logging
import app.llm.llm_types as types
from app.llm import Role, get_llm_provider
from app.errors import OllamaEmptyResponseError
from app.llm.structured import parse_structured_response
from app.constants import (
    TRIAGE_CONVERSATION_TAIL_LIMIT,
    TRIAGE_EMPTY_CONVERSATION,
    TRIAGE_LOG_TRUNCATION_LENGTH,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
)
from app.constants.message_sender import MessageSender
from app.models.agents.triage import TriageRequest, TriageResult
from app.models.investigations import ConversationHistoryMessage
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder
from app.utils.agent_persona_loader import get_agent_persona, AgentPersona

logger = logging.getLogger(__name__)


class TriageAgent:
    """Agent responsible for classifying user intent and message complexity."""

    def __init__(self):
        """Initialize the triage agent."""

    async def triage(self, request: TriageRequest) -> TriageResult:
        """Perform the triage operation using the configured LLM provider.
        
        Args:
            request: The TriageRequest containing message, history, and settings.
            
        Returns:
            TriageResult containing complexity and intent classification.
        """
        # Short-circuit: Attachments always escalate (multimodal analysis)
        if request.attachments:
            logger.info("[TRIAGE] Escalating: attachments present (%d)", len(request.attachments))
            return TriageResult(
                complexity=TriageComplexityClassification.COMPLEX,
                complexity_confidence=TriageConfidence.HIGH,
                intent=TriageIntentClassification.ACTION,
                intent_confidence=TriageConfidence.HIGH,
                intent_summary="User provided attachments for analysis.",
                request_posture=TriageRequestPosture.NORMAL,
                posture_confidence=TriageConfidence.LOW,
            )

        # Short-circuit: Empty message
        if not request.message or not request.message.strip():
            logger.info("[TRIAGE] Escalating: empty message")
            return TriageResult(
                complexity=TriageComplexityClassification.COMPLEX,
                complexity_confidence=TriageConfidence.HIGH,
                intent=TriageIntentClassification.UNKNOWN,
                intent_confidence=TriageConfidence.LOW,
                intent_summary="Empty message provided.",
                follow_up_question="How can I help you today?",
                request_posture=TriageRequestPosture.NORMAL,
                posture_confidence=TriageConfidence.LOW,
            )

        try:
            provider = get_llm_provider(request.settings.llm, is_assistant=True)
            model = request.model_override or request.settings.llm.assistant_model

            if not model:
                logger.warning("[TRIAGE] No model available, defaulting to complex")
                return self._escalation_result(
                    "Triage unavailable: no assistant model configured. Configure an assistant model in settings or provide model_override to enable triage.",
                    error_code="MODEL_UNAVAILABLE",
                )

            conversation_tail = self._build_conversation_tail(request.conversation_history)

            persona = get_agent_persona("triage")
            prompt_template = persona.get_system_prompt()
            conversation_tail_xml = AgentPersona.format_xml_tag("conversation_tail", conversation_tail)
            message_xml = AgentPersona.format_xml_tag("message", request.message)
            prompt = f"{prompt_template}\n\n{conversation_tail_xml}\n\n{message_xml}"

            config = AIGenerationConfigBuilder.build_lite_settings(
                model=model,
                max_tokens=None,
                system_instructions="",
            )

            try:
                response = await provider.generate_content_lite(
                    model=model,
                    contents=[types.Content(role=Role.USER, parts=[types.Part(text=prompt)])],
                    lite_llm_settings=config,
                )
                if not response.text:
                    logger.warning("[TRIAGE] Empty response text from assistant model, defaulting to complex")
                    return self._escalation_result(
                        "Triage unavailable: assistant model returned empty text. Check model availability and connectivity, then retry.",
                        error_code="MODEL_EMPTY_RESPONSE",
                    )
                result = self._parse_response(response.text)
            except OllamaEmptyResponseError as exc:
                logger.warning("[TRIAGE] No response from assistant model, defaulting to complex: %s", exc)
                return self._escalation_result(
                    f"Triage unavailable: assistant model returned empty response ({exc}). Check model availability and connectivity, then retry.",
                    error_code="MODEL_EMPTY_RESPONSE",
                    error_class=exc.__class__.__name__,
                    error_message=str(exc),
                )

            try:

                logger.info(
                    "[TRIAGE] Classification: complexity=%s confidence=%s model=%s intent=%s",
                    result.complexity,
                    result.intent_confidence,
                    model,
                    result.intent_summary[:TRIAGE_LOG_TRUNCATION_LENGTH],
                )
                return result
            except (ValueError, Exception) as e:
                logger.warning("[TRIAGE] Failed to parse model response: %s. Response: %r", e, response.text)
                return self._escalation_result(
                    f"Triage unavailable: failed to parse model response ({e}). Escalating to full LLM for complexity classification.",
                    error_code="PARSE_FAILURE",
                    error_class=e.__class__.__name__,
                    error_message=str(e),
                )

        except Exception as exc:
            logger.exception("[TRIAGE] Classification failed, defaulting to complex")
            return self._escalation_result(
                f"Triage unavailable: classification failed ({exc}). Escalating to full LLM for complexity classification. Check provider configuration and retry.",
                error_code="CLASSIFICATION_ERROR",
                error_class=exc.__class__.__name__,
                error_message=str(exc),
                )

    def _build_conversation_tail(self, history: list[ConversationHistoryMessage]) -> str:
        """Return the last few turns of conversation as a compact string."""
        if not history:
            return TRIAGE_EMPTY_CONVERSATION

        lines: list[str] = []
        for msg in history[-TRIAGE_CONVERSATION_TAIL_LIMIT:]:
            content = (msg.content or "").strip()
            if not content:
                continue
            
            # Use MessageSender for proper sender identification
            role = Role.USER if msg.sender == MessageSender.USER_CHAT else Role.MODEL
            lines.append(f"{role.value}: {content}")

        return "\n".join(lines) or TRIAGE_EMPTY_CONVERSATION

    def _escalation_result(
        self,
        summary: str,
        error_code: str | None = None,
        error_class: str | None = None,
        error_message: str | None = None,
    ) -> TriageResult:
        """Create an escalation result when triage fails.
        
        When triage cannot determine complexity, we escalate to COMPLEX (full LLM)
        as a safe default. This is more conservative than assuming SIMPLE.
        """
        return TriageResult(
            complexity=TriageComplexityClassification.COMPLEX,
            complexity_confidence=TriageConfidence.LOW,
            intent=TriageIntentClassification.UNKNOWN,
            intent_confidence=TriageConfidence.LOW,
            intent_summary=summary,
            request_posture=TriageRequestPosture.NORMAL,
            posture_confidence=TriageConfidence.LOW,
            error_code=error_code,
            error_class=error_class,
            error_message=error_message,
        )

    def _parse_response(self, text: str) -> TriageResult:
        """Parse the LLM response text into a TriageResult, with robust JSON extraction."""
        if not text:
            raise ValueError("Empty response text")
        return parse_structured_response(text, TriageResult, allow_bare_value=False)
