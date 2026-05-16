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
AttachmentGroundingProvider — attachment-based grounding context for the AI.

User-uploaded files (PDF, image, text) are a form of grounding: they anchor AI
responses to concrete, user-provided data. This provider formats processed
attachment data into canonical LLM Part objects ready for injection into the
contents array.

Consumed by AIRequestBuilder.format_attachment_parts(), which delegates here.
"""

import base64
import logging

import app.llm.llm_types as types
from app.constants import (
    ATTACHED_DOCUMENT_FOOTER_TEMPLATE,
    ATTACHED_DOCUMENT_HEADER_TEMPLATE,
    AttachmentType,
)
from app.errors import ValidationError
from app.models.attachments import ProcessedAttachment

logger = logging.getLogger(__name__)


class AttachmentGroundingProvider:
    """Formats processed attachments into canonical LLM Part objects.

    Stateless — all methods are pure functions operating on their arguments.
    Instantiated once at startup and injected where needed.

    Supported attachment types and their LLM encoding:
      PDF   — inline binary data via Part.from_bytes()
      IMAGE — inline binary data via Part.from_bytes()
      TEXT  — plain text context injected via Part.from_text()
      OTHER — skipped with a warning log
    """

    def format_parts(self, attachments: list[ProcessedAttachment] | None) -> list[types.Part]:
        """Format attachments as canonical Part objects for the LLM provider.

        PDFs and images are sent as inline binary data via Part.from_bytes().
        Text files are included as text context in the conversation.

        Args:
            attachments: List of ProcessedAttachment with base64_data, filename, content_type.

        Returns:
            List of canonical Part objects ready for injection into contents.
        """
        parts: list[types.Part] = []

        if not attachments:
            return parts

        logger.info("Formatting %d attachment(s) as grounding parts", len(attachments))

        for att in attachments:
            if not att.base64_data:
                logger.warning("%s: skipped - no base64_data", att.filename)
                continue

            is_pdf = att.attachment_type == AttachmentType.PDF
            is_image = att.attachment_type == AttachmentType.IMAGE
            is_text = att.attachment_type == AttachmentType.TEXT

            if is_pdf or is_image:
                try:
                    file_bytes = base64.b64decode(att.base64_data, validate=True)
                except Exception as e:
                    raise ValidationError(
                        f"Failed to decode base64 data for attachment '{att.filename}'",
                        details={"cause": str(e)},
                    ) from e
                parts.append(types.Part.from_bytes(
                    data=file_bytes,
                    mime_type=att.content_type,
                ))
                file_type = "PDF" if is_pdf else "image"
                logger.info("%s: added as %s inline (%d bytes)", att.filename, file_type, len(file_bytes))

            elif is_text and att.content:
                text_content = (
                    ATTACHED_DOCUMENT_HEADER_TEMPLATE.format(filename=att.filename)
                    + att.content
                    + "\n"
                    + ATTACHED_DOCUMENT_FOOTER_TEMPLATE.format(filename=att.filename)
                )
                parts.append(types.Part.from_text(text=text_content))
                logger.info("%s: added as text context (%d chars)", att.filename, len(att.content))

            elif is_text:
                try:
                    file_bytes = base64.b64decode(att.base64_data, validate=True)
                    text_content = file_bytes.decode("utf-8", errors="replace")
                except Exception as e:
                    raise ValidationError(
                        f"Failed to decode text content for attachment '{att.filename}'",
                        details={"cause": str(e)},
                    ) from e
                formatted = (
                    ATTACHED_DOCUMENT_HEADER_TEMPLATE.format(filename=att.filename)
                    + text_content
                    + "\n"
                    + ATTACHED_DOCUMENT_FOOTER_TEMPLATE.format(filename=att.filename)
                )
                parts.append(types.Part.from_text(text=formatted))
                logger.info("%s: added as decoded text (%d chars)", att.filename, len(text_content))

            else:
                logger.warning(
                    "%s: skipped - unsupported type (content_type=%s)",
                    att.filename, att.content_type,
                )

        logger.info("Formatted %d attachment grounding part(s)", len(parts))
        return parts
