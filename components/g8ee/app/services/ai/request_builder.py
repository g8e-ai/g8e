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
AI Request Builder - Stateful assembly of LLM request contents and configs.

This service is responsible for:
- Building contents arrays from conversation history for stateless generate_content
- Building GenerateContentConfig for main-model calls (requires tool executor)
- Formatting attachments for LLM provider consumption

Separation of Concerns:
- AIRequestBuilder: stateful request assembly — tool executor, contents, attachments
- AIGenerationConfigBuilder: stateless config factory — lite/schema config builders
- AIEventPublisher: HOW to publish events to frontend
- AIResponseAnalyzer: HOW to analyze AI responses
- AIToolExecutor: HOW to execute AI tool calls
- g8eEngine: Core streaming loop (uses all above)
"""

import logging
from typing import Protocol, runtime_checkable

from app.models.settings import G8eeUserSettings
import app.llm.llm_types as types
from app.llm.llm_types import PrimaryLLMSettings
from app.constants import ATTACHMENT_FILENAMES_PREFIX_TEMPLATE, AgentMode
from app.constants.message_sender import MessageSender
from app.models.attachments import ProcessedAttachment
from app.models.investigations import ConversationHistoryMessage, UserChatMetadata
from app.security import scrub_user_message
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder
from app.services.ai.grounding.attachment_provider import AttachmentGroundingProvider

logger = logging.getLogger(__name__)


@runtime_checkable
class ToolExecutorProtocol(Protocol):
    def get_tools(self, agent_mode: AgentMode, model_to_use: str | None) -> list[types.ToolGroup]: ...
    @property
    def g8e_web_search_available(self) -> bool: ...


class AIRequestBuilder:
    """Stateful assembler for AI request contents and main-model configs.

    Holds tool executor context to build function-calling configs. For stateless
    lightweight configs (triage, analysis, memory), use AIGenerationConfigBuilder directly.

    Responsibilities:
    - Building contents arrays from database conversation history
    - Configuring GenerateContentConfig for main-model calls (with tool declarations)
    - Formatting file attachments for LLM provider consumption
    """

    def __init__(self, tool_executor: ToolExecutorProtocol | None):
        self.tool_executor = tool_executor
        self._attachment_provider = AttachmentGroundingProvider()
        logger.info("AIRequestBuilder initialized")

    def build_contents_from_history(
        self,
        conversation_history: list[ConversationHistoryMessage],
        attachments: list[types.Part] | None = None,
        sentinel_mode: bool = True,
    ) -> list[types.Content]:
        """
        Build a contents array from database conversation history for stateless generate_content.

        Database conversation history is the single source of truth. The current user message
        must already be stored in conversation_history before calling this method.

        Attachment Part objects are appended to the last user message's parts,
        since binary attachment data is not stored in database.

        Args:
            conversation_history: List of ConversationHistoryMessage from database.
                Must include the current user message (already stored before this call).
            attachments: Optional list of Part objects to append to the last user message.
            sentinel_mode: Controls AI data access - True (scrubbed/redacted) or False (full data)

        Returns:
            List of types.Content objects ready for generate_content_stream()
        """
        contents: list[types.Content] = []
        should_scrub = sentinel_mode is True

        if conversation_history:
            logger.info(f" [BUILD_CONTENTS] Converting {len(conversation_history)} messages")

            for msg in conversation_history:
                content_text = msg.content
                sender = msg.sender

                if not content_text or not content_text.strip():
                    continue

                if sender == MessageSender.USER_CHAT:
                    text_for_ai = scrub_user_message(content_text) if should_scrub else content_text
                    filenames = (
                        msg.metadata.attachment_filenames
                        if isinstance(msg.metadata, UserChatMetadata) and msg.metadata.attachment_filenames
                        else []
                    )
                    if filenames:
                        text_for_ai = ATTACHMENT_FILENAMES_PREFIX_TEMPLATE.format(filenames=", ".join(filenames)) + text_for_ai
                    contents.append(types.Content(
                        role=types.Role.USER,
                        parts=[types.Part.from_text(text=text_for_ai)],
                    ))
                elif sender == MessageSender.USER_TERMINAL:
                    terminal_text_for_ai = scrub_user_message(content_text) if should_scrub else content_text
                    contents.append(types.Content(
                        role=types.Role.USER,
                        parts=[types.Part.from_text(text=f"[SYSTEM OUTPUT]\n{terminal_text_for_ai}")],
                    ))
                elif sender in [MessageSender.AI_PRIMARY, MessageSender.AI_ASSISTANT, types.Role.MODEL, types.Role.ASSISTANT]:
                    contents.append(types.Content(
                        role=types.Role.MODEL,
                        parts=[types.Part.from_text(text=content_text)],
                    ))

            logger.info(f" [BUILD_CONTENTS] Built {len(contents)} Content objects from history")

        if attachments and contents:
            for i in range(len(contents) - 1, -1, -1):
                if contents[i].role == types.Role.USER:
                    contents[i].parts.extend(attachments)
                    logger.info(f" [BUILD_CONTENTS] Appended {len(attachments)} attachment(s) to last user message")
                    break

        return contents

    def get_generation_config(
        self,
        system_instructions: str,
        settings: G8eeUserSettings,
        agent_mode: AgentMode,
        max_tokens: int | None = None,
        model_override: str | None = None,
    ) -> PrimaryLLMSettings:
        """
        Build PrimaryLLMSettings for main-model generate_content calls.

        Args:
            system_instructions: System instructions with senior engineer methodologies
            settings: Request-scoped Settings object
            agent_mode: OPERATOR_BOUND (execute) or OPERATOR_NOT_BOUND (advise)
            max_tokens: Maximum output tokens override
            model_override: Model name override (Pro, Flash, etc.)

        Returns:
            PrimaryLLMSettings ready for generate_content_stream_primary()
        """
        model = model_override or settings.llm.primary_model or "gpt-4o-mini"
        tools = self.tool_executor.get_tools(agent_mode, model) if self.tool_executor else []

        return AIGenerationConfigBuilder.build_primary_settings(
            model=model,
            temperature=settings.llm.llm_temperature,
            max_tokens=max_tokens or settings.llm.llm_max_tokens,
            system_instruction=system_instructions,
            tools=tools,
         )

    def format_attachment_parts(self, attachments: list[ProcessedAttachment] | None) -> list[types.Part]:
        """Format attachments as canonical Part objects for the LLM provider.

        Delegates to AttachmentGroundingProvider which owns all attachment-as-grounding
        formatting logic.

        Args:
            attachments: List of ProcessedAttachment with base64_data, filename, content_type.

        Returns:
            List of canonical Part objects.
        """
        return self._attachment_provider.format_parts(attachments)
