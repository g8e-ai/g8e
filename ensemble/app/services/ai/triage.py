# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
AI Triage Agent

Classifies incoming user messages as 'simple' or 'complex' using the lightweight
assistant model before committing to the full main LLM.
"""

import logging
import time

import app.llm.llm_types as types
from app.llm import Role, get_llm_provider
from app.errors import OllamaEmptyResponseError
from app.llm.model_evidence import (
    model_boundary_hash,
)
from app.llm.model_call_attribution import build_model_call_telemetry, prepare_provider_call
from app.llm.structured import parse_structured_response
from app.constants import (
    LLMProvider,
    TRIAGE_CONVERSATION_TAIL_LIMIT,
    TRIAGE_EMPTY_CONVERSATION,
    TRIAGE_LOG_TRUNCATION_LENGTH,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
)
from app.constants.message_sender import MessageSender
from app.decision import get_decision_provider
from app.decision.types import ChoiceAnswer, ChoiceQuestion, EvaluateResponse
from app.models.agents.triage import TriageRequest, TriageResult
from app.models.investigations import ConversationHistoryMessage
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder
from app.utils.agent_persona_loader import get_agent_persona, AgentPersona

logger = logging.getLogger(__name__)

# Map Jev choice confidence to TriageConfidence cutoffs.
# HIGH when model confidence >= threshold; otherwise LOW.
JEV_TRIAGE_HIGH_CONFIDENCE_THRESHOLD = 0.85


class TriageAgent:
    """Agent responsible for classifying user intent and message complexity.

    Per GDD §14.1: Triage is the Gatekeeper/Classifier only. It reads every user
    message and emits structured classification metadata (complexity, intent, posture).
    It does NOT generate questions or interrogations - that responsibility belongs
    to the reasoning agents (Dash/Sage) per the Interrogation Protocol.
    """

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
                request_posture=TriageRequestPosture.NORMAL,
                posture_confidence=TriageConfidence.LOW,
            )

        try:
            model = request.model_override or request.settings.llm.resolved_lite_model

            if not model:
                logger.warning("[TRIAGE] No model available, defaulting to complex")
                return self._escalation_result(
                    "Triage unavailable: no lite model or assistant model configured. Configure a lite model or assistant model in settings, or provide model_override to enable triage.",
                    error_code="MODEL_UNAVAILABLE",
                )

            if request.settings.llm.lite_provider == LLMProvider.JEV:
                return await self._classify_with_jev(request, model)

            provider = get_llm_provider(request.settings.llm, is_lite=True)

            conversation_tail = self._build_conversation_tail(request.conversation_history)

            persona = get_agent_persona("triage")
            prompt_template = persona.get_system_prompt()
            conversation_tail_xml = AgentPersona.format_xml_tag(
                "conversation_tail", conversation_tail
            )
            message_xml = AgentPersona.format_xml_tag("message", request.message)
            prompt = f"{prompt_template}\n\n{conversation_tail_xml}\n\n{message_xml}"
            response_schema = TriageResult.model_json_schema()
            for field_name in ("error_code", "error_class", "error_message", "model_call"):
                response_schema.get("properties", {}).pop(field_name, None)

            config = AIGenerationConfigBuilder.build_lite_settings(
                model=model,
                max_tokens=None,
                system_instructions="",
                response_format=types.ResponseFormat.from_pydantic_schema(
                    response_schema, name="TriageResult"
                ),
            )

            try:
                prepare_provider_call(provider, g8e_context=request.g8e_context)
                contents = [types.Content(role=Role.USER, parts=[types.Part(text=prompt)])]
                input_artifact_hash = model_boundary_hash({
                    "model": model,
                    "contents": contents,
                    "settings": config,
                })
                monotonic_start = time.monotonic()
                response = await provider.generate_content_lite(
                    model=model,
                    contents=contents,
                    lite_llm_settings=config,
                )
                monotonic_end = time.monotonic()
                usage = response.usage_metadata
                finish_reason = response.candidates[0].finish_reason if response.candidates else None
                model_call = build_model_call_telemetry(
                    provider=provider,
                    agent_role="triage",
                    model_role="lite",
                    model=model,
                    monotonic_start=monotonic_start,
                    monotonic_end=monotonic_end,
                    input_artifact_hash=input_artifact_hash,
                    input_tokens=usage.prompt_token_count,
                    output_tokens=usage.candidates_token_count,
                    thinking_tokens=usage.thinking_token_count,
                    cache_tokens=usage.cache_token_count,
                    total_tokens=usage.total_token_count,
                    usage_reported=usage.usage_reported,
                    finish_reason=finish_reason,
                    generation_duration_seconds=usage.eval_duration_seconds,
                    prompt_eval_duration_seconds=usage.prompt_eval_duration_seconds,
                    total_duration_seconds=usage.total_duration_seconds,
                    load_duration_seconds=usage.load_duration_seconds,
                    output_artifact_hash=model_boundary_hash(response.text or ""),
                )
                if not response.text:
                    logger.warning(
                        "[TRIAGE] Empty response text from lite model, defaulting to complex"
                    )
                    failed_call = model_call.model_copy(update={
                        "succeeded": False,
                        "error_type": "EmptyResponseError",
                    })
                    return self._escalation_result(
                        "Triage unavailable: lite model returned empty text. Check model availability and connectivity, then retry.",
                        error_code="MODEL_EMPTY_RESPONSE",
                    ).model_copy(update={"model_call": failed_call})
                result = self._parse_response(response.text).model_copy(update={"model_call": model_call})
            except OllamaEmptyResponseError as exc:
                logger.warning(
                    "[TRIAGE] No response from lite model, defaulting to complex: %s", exc
                )
                failed_call = build_model_call_telemetry(
                    provider=provider,
                    agent_role="triage",
                    model_role="lite",
                    model=model,
                    monotonic_start=monotonic_start,
                    input_artifact_hash=input_artifact_hash,
                    succeeded=False,
                    error_type=type(exc).__name__,
                )
                return self._escalation_result(
                    f"Triage unavailable: lite model returned empty response ({exc}). Check model availability and connectivity, then retry.",
                    error_code="MODEL_EMPTY_RESPONSE",
                    error_class=exc.__class__.__name__,
                    error_message=str(exc),
                ).model_copy(update={"model_call": failed_call})
            except Exception as exc:
                logger.exception("[TRIAGE] Provider call failed, defaulting to complex")
                failed_call = build_model_call_telemetry(
                    provider=provider,
                    agent_role="triage",
                    model_role="lite",
                    model=model,
                    monotonic_start=monotonic_start,
                    input_artifact_hash=input_artifact_hash,
                    succeeded=False,
                    error_type=type(exc).__name__,
                )
                return self._escalation_result(
                    f"Triage unavailable: classification failed ({exc}). Escalating to full LLM for complexity classification. Check provider configuration and retry.",
                    error_code="CLASSIFICATION_ERROR",
                    error_class=exc.__class__.__name__,
                    error_message=str(exc),
                ).model_copy(update={"model_call": failed_call})

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
                logger.warning(
                    "[TRIAGE] Failed to parse model response: %s. Response: %r", e, response.text
                )
                failed_call = model_call.model_copy(update={
                    "succeeded": False,
                    "error_type": type(e).__name__,
                })
                return self._escalation_result(
                    f"Triage unavailable: failed to parse model response ({e}). Escalating to full LLM for complexity classification.",
                    error_code="PARSE_FAILURE",
                    error_class=e.__class__.__name__,
                    error_message=str(e),
                ).model_copy(update={"model_call": failed_call})

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

    async def _classify_with_jev(self, request: TriageRequest, model: str) -> TriageResult:
        """Classify triage fields via a batched System One evaluate call."""
        conversation_tail = self._build_conversation_tail(request.conversation_history)
        state = self._build_jev_state(conversation_tail, request.message)
        questions = self._build_jev_questions()

        try:
            provider = get_decision_provider(request.settings.llm)
        except Exception as exc:
            logger.exception("[TRIAGE] Jev provider unavailable, defaulting to complex")
            return self._escalation_result(
                f"Triage unavailable: Jev decision provider failed to initialize ({exc}). "
                "Check Jev API key and lite provider configuration, then retry.",
                error_code="CLASSIFICATION_ERROR",
                error_class=exc.__class__.__name__,
                error_message=str(exc),
            )

        provider.clear_input_artifact_hash()
        input_artifact_hash = model_boundary_hash({
            "model": model,
            "state": state,
            "questions": {
                name: question.model_dump(mode="json", exclude_none=True)
                for name, question in questions.items()
            },
        })
        monotonic_start = time.monotonic()

        try:
            response = await provider.evaluate(
                model=model,
                state=state,
                questions=questions,
            )
            monotonic_end = time.monotonic()
            result = self._map_jev_response_to_triage(response)
            model_call = build_model_call_telemetry(
                provider=provider,
                agent_role="triage",
                model_role="lite",
                model=response.model or model,
                monotonic_start=monotonic_start,
                monotonic_end=monotonic_end,
                input_artifact_hash=input_artifact_hash,
                input_tokens=response.usage.input_tokens,
                output_tokens=response.usage.output_tokens,
                total_tokens=response.usage.input_tokens + response.usage.output_tokens,
                usage_reported=True,
                output_artifact_hash=model_boundary_hash(
                    response.model_dump(mode="json", exclude_none=True)
                ),
            )
            logger.info(
                "[TRIAGE] Jev classification: complexity=%s confidence=%s model=%s intent=%s",
                result.complexity,
                result.intent_confidence,
                model,
                result.intent_summary[:TRIAGE_LOG_TRUNCATION_LENGTH],
            )
            return result.model_copy(update={"model_call": model_call})
        except Exception as exc:
            logger.exception("[TRIAGE] Jev classification failed, defaulting to complex")
            failed_call = build_model_call_telemetry(
                provider=provider,
                agent_role="triage",
                model_role="lite",
                model=model,
                monotonic_start=monotonic_start,
                input_artifact_hash=input_artifact_hash,
                succeeded=False,
                error_type=type(exc).__name__,
            )
            return self._escalation_result(
                f"Triage unavailable: Jev classification failed ({exc}). "
                "Escalating to full LLM for complexity classification. "
                "Check provider configuration and retry.",
                error_code="CLASSIFICATION_ERROR",
                error_class=exc.__class__.__name__,
                error_message=str(exc),
            ).model_copy(update={"model_call": failed_call})

    def _build_jev_state(self, conversation_tail: str, message: str) -> str:
        persona = get_agent_persona("triage")
        prompt_template = persona.get_system_prompt()
        conversation_tail_xml = AgentPersona.format_xml_tag(
            "conversation_tail", conversation_tail
        )
        message_xml = AgentPersona.format_xml_tag("message", message)
        return f"{prompt_template}\n\n{conversation_tail_xml}\n\n{message_xml}"

    def _build_jev_questions(self) -> dict[str, ChoiceQuestion]:
        return {
            "complexity": ChoiceQuestion(
                instructions=(
                    "Classify task complexity using the complexity_rules in state. "
                    "Security-sensitive operations are always complex."
                ),
                criteria={
                    "simple": "Single-step tasks, routine inquiries, or status checks.",
                    "complex": (
                        "Multi-step operations, ambiguous requests, security-sensitive "
                        "tasks, or deep reasoning."
                    ),
                },
            ),
            "intent": ChoiceQuestion(
                instructions="Classify user intent using the intent_rules in state.",
                criteria={
                    "information": "User wants to know something.",
                    "action": "User wants to change something or execute a tool.",
                    "unknown": "Intent is ambiguous or requires more context.",
                },
            ),
            "request_posture": ChoiceQuestion(
                instructions=(
                    "Classify user posture using the posture_rules in state. "
                    "First-turn messages cannot be adversarial."
                ),
                criteria={
                    "normal": "Productive and professional interaction.",
                    "escalated": "User is frustrated, hurried, or reporting a critical outage.",
                    "adversarial": (
                        "User attempting to bypass a prior refusal; requires history evidence."
                    ),
                    "confused": "Request contradicts stated goal or system reality.",
                },
            ),
        }

    @staticmethod
    def _jev_confidence_to_triage(confidence: float) -> TriageConfidence:
        if confidence >= JEV_TRIAGE_HIGH_CONFIDENCE_THRESHOLD:
            return TriageConfidence.HIGH
        return TriageConfidence.LOW

    def _map_jev_response_to_triage(self, response: EvaluateResponse) -> TriageResult:
        complexity_answer = response.answers.get("complexity")
        intent_answer = response.answers.get("intent")
        posture_answer = response.answers.get("request_posture")

        if not all(
            isinstance(answer, ChoiceAnswer)
            for answer in (complexity_answer, intent_answer, posture_answer)
        ):
            raise ValueError("Jev triage response missing required choice answers")

        complexity = TriageComplexityClassification(complexity_answer.choice)
        intent = TriageIntentClassification(intent_answer.choice)
        posture = TriageRequestPosture(posture_answer.choice)
        intent_summary = (
            f"{intent.value} intent with {complexity.value} complexity "
            f"({posture.value} posture)"
        )

        return TriageResult(
            complexity=complexity,
            complexity_confidence=self._jev_confidence_to_triage(
                complexity_answer.confidence
            ),
            intent=intent,
            intent_confidence=self._jev_confidence_to_triage(intent_answer.confidence),
            intent_summary=intent_summary,
            request_posture=posture,
            posture_confidence=self._jev_confidence_to_triage(posture_answer.confidence),
        )
