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
import json
import re
import app.llm.llm_types as types
from app.llm import Role, get_llm_provider
from app.constants import (
    TRIAGE_CONVERSATION_TAIL_LIMIT,
    TRIAGE_EMPTY_CONVERSATION,
    TRIAGE_LOG_TRUNCATION_LENGTH,
    EventType,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    LLMProvider,
    AgentMode,
)
from app.constants.message_sender import MessageSender
from app.models.agents.triage import TriageRequest, TriageResult
from app.models.investigations import ConversationHistoryMessage
from app.models.attachments import AttachmentMetadata
from app.models.settings import G8eeUserSettings
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder

logger = logging.getLogger(__name__)

_TRIAGE_PROMPT_TEMPLATE = """\
You are a routing and intent-analysis assistant. Your goal is to identify the user's TRUE intent and classify the complexity of their request.

<definitions>
# Intent Categories
- information: The user is asking for an explanation, fact, or concept. No action required.
- action: The user wants something DONE (execution, modification, deployment, debugging).
- unknown: The intent is ambiguous or missing.

# Complexity Levels
- simple: A single, self-contained conversational or factual question.
  - No code execution, no file operations, no system commands needed.
  - No multi-step reasoning required.
  - Can be answered confidently in one pass without tools.
  - Examples: "What is a reverse proxy?", "How many MB in a GB?", "What does chmod 755 mean?"

- complex: Anything that requires tools, commands, multi-step reasoning, or technical depth.
  - Requires running commands or accessing a system.
  - Requires reading, writing, or analysing files or logs.
  - Involves debugging, root-cause analysis, or a sequence of steps.
  - Involves configuring, deploying, or modifying infrastructure.
  - The user wants something *done*, not just explained.
</definitions>

<instructions>
1. Summarize the user's true intent: what is their end goal? What do they ultimately need?
2. Classify the intent category as 'information', 'action', or 'unknown'.
3. Rate your confidence in the intent classification as 'high' or 'low'.
4. Classify the complexity as 'simple' or 'complex'.
5. Rate your confidence in the complexity classification as 'high' or 'low'.
6. If intent confidence is 'low', provide a concise follow-up question to clarify their ultimate goal.
</instructions>

<conversation_tail>
{conversation_tail}
</conversation_tail>

<message>
{message}
</message>

Respond ONLY with a JSON object following this structure:
{{
  "intent_summary": "string",
  "intent": "information" | "action" | "unknown",
  "intent_confidence": "high" | "low",
  "complexity": "simple" | "complex",
  "complexity_confidence": "high" | "low",
  "follow_up_question": "string" | null
}}
"""


class TriageAgent:
    """Agent responsible for classifying user intent and message complexity."""

    def __init__(self):
        """Initialize the triage agent."""
        self._json_fence_re = re.compile(r"```(?:json)?\s*(.*?)\s*```", re.DOTALL)

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
            )

        try:
            async with get_llm_provider(request.settings.llm) as provider:
                model = request.model_override or request.settings.llm.primary_model

                conversation_tail = self._build_conversation_tail(request.conversation_history)

                prompt = _TRIAGE_PROMPT_TEMPLATE.format(
                    conversation_tail=conversation_tail,
                    message=request.message,
                )

                config = AIGenerationConfigBuilder.build_lite_settings(
                    model=model,
                    temperature=None,
                    max_tokens=None,
                    system_instruction="",
                )

                response = await provider.generate_content_lite(
                    model=model,
                    contents=[types.Content(role=Role.USER, parts=[types.Part(text=prompt)])],
                    lite_llm_settings=config,
                )

                if not response or not response.text:
                    logger.warning("[TRIAGE] No response from assistant model, defaulting to complex")
                    return self._fallback_result("Could not determine intent (no model response).")

                try:
                    result = self._parse_response(response.text)

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
                    return self._fallback_result("Could not parse intent summary from model response.")

        except Exception as exc:
            logger.exception("[TRIAGE] Classification failed, defaulting to complex")
            return self._fallback_result(f"Error during triage: {exc}")

    def _build_conversation_tail(self, history: list[ConversationHistoryMessage]) -> str:
        """Return the last few turns of conversation as a compact string."""
        if not history:
            return TRIAGE_EMPTY_CONVERSATION

        lines = []
        for msg in history[-TRIAGE_CONVERSATION_TAIL_LIMIT:]:
            content = (msg.content or "").strip()
            if not content:
                continue
            
            # Use MessageSender for proper sender identification
            role = Role.USER if msg.sender == MessageSender.USER_CHAT else Role.MODEL
            lines.append(f"{role.value}: {content}")

        return "\n".join(lines) or TRIAGE_EMPTY_CONVERSATION

    def _fallback_result(self, summary: str) -> TriageResult:
        """Create a safe fallback result when triage fails."""
        return TriageResult(
            complexity=TriageComplexityClassification.COMPLEX,
            complexity_confidence=TriageConfidence.LOW,
            intent=TriageIntentClassification.UNKNOWN,
            intent_confidence=TriageConfidence.LOW,
            intent_summary=summary,
        )

    def _parse_response(self, text: str) -> TriageResult:
        """Parse the LLM response text into a TriageResult, with robust JSON extraction."""
        if not text:
            raise ValueError("Empty response text")

        # 1. Try markdown fence extraction
        fence_match = self._json_fence_re.search(text)
        if fence_match:
            try:
                return TriageResult.model_validate_json(fence_match.group(1).strip())
            except Exception:
                pass

        # 2. Try raw parse of stripped text
        stripped = text.strip()
        try:
            return TriageResult.model_validate_json(stripped)
        except Exception:
            pass

        # 3. Try finding first { and last }
        start = stripped.find("{")
        end = stripped.rfind("}")
        if start != -1 and end != -1 and end > start:
            try:
                return TriageResult.model_validate_json(stripped[start : end + 1])
            except Exception:
                pass

        # 4. Final attempt: partial JSON recovery (common with Gemini/thinking)
        # If it starts with { but doesn't end with }, try appending braces
        if start != -1 and end == -1:
            potential_json = stripped[start:]
            for suffix in ["}", "]}", "}}", "}}]", "]}"]:
                try:
                    return TriageResult.model_validate_json(potential_json + suffix)
                except Exception:
                    continue

        # If all else fails, re-raise the last error from a direct parse attempt
        return TriageResult.model_validate_json(stripped)
