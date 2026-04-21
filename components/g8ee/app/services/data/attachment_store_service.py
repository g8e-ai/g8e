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
Attachment Service for g8ee

Retrieves file attachment data from g8es Blob Store that was stored by g8ed,
and processes the retrieved base64 data for LLM provider consumption.

Attachments are stored per-investigation in Blob Store namespaces.
Namespace format: att:{investigation_id}
Blob ID format: att:{investigation_id}/{attachment_id}

This service is read-only from g8ee's perspective - g8ed handles writes.
"""

import base64
import json
import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from app.db.blob_service import BlobService

from app.models.settings import G8eePlatformSettings
from app.constants import AttachmentType
from app.errors import NetworkError
from app.models.attachments import AttachmentData, AttachmentMetadata, ProcessedAttachment
from app.models.operators import AttachmentRecord

logger = logging.getLogger(__name__)


class AttachmentService:
    """
    Retrieves attachment data from g8es Blob Store and processes it for AI consumption.

    g8ed stores full AttachmentData JSON (including base64 data) in Blob Store.
    g8ee retrieves and classifies it here for LLM provider consumption.
    """

    def __init__(
        self,
        blob_service: "BlobService",
        settings: G8eePlatformSettings,
    ):
        self.blob_service = blob_service
        self.settings = settings

    def _is_text_file(self, content_type: str, filename: str) -> bool:
        text_types = [
            "text/", "application/json", "application/xml", "application/yaml",
            "application/x-yaml", "application/javascript", "application/sql"
        ]
        text_extensions = [
            ".txt", ".log", ".conf", ".config", ".ini", ".yaml", ".yml",
            ".json", ".xml", ".html", ".htm", ".css", ".js", ".py", ".java",
            ".cpp", ".c", ".h", ".sql", ".sh", ".bash", ".ps1", ".md", ".rst"
        ]
        for text_type in text_types:
            if content_type.startswith(text_type):
                return True
        
        # Check extension without using os.path
        filename_lower = filename.lower()
        for ext in text_extensions:
            if filename_lower.endswith(ext):
                return True
        return False

    def _is_supported_image(self, content_type: str) -> bool:
        if not content_type:
            return False
        return content_type.lower().strip().startswith("image/")

    async def get_attachment(self, store_key: str) -> AttachmentData | None:
        """
        Retrieve a single attachment from the Blob Store.

        Args:
            store_key: Attachment identifier in Blob Store format: att:{inv_id}/{att_id}

        Returns:
            Full attachment data including base64_data, or None if missing
        """
        if "/" not in store_key or not store_key.startswith("att:"):
            logger.warning(
                "[ATTACHMENTS] Invalid store_key format (expected att:inv_id/att_id)",
                extra={"store_key": store_key}
            )
            return None

        try:
            parts = store_key.split("/", 1)
            namespace = parts[0]
            blob_id = parts[1]
            blob_data = await self.blob_service.get_blob(namespace, blob_id)
            if blob_data is None:
                logger.warning(
                    "[ATTACHMENTS] Attachment not found in Blob Store",
                    extra={"store_key": store_key}
                )
                return None

            return AttachmentData(**json.loads(blob_data.decode("utf-8")))
        except Exception as e:
            raise NetworkError(
                "[ATTACHMENTS] Failed to retrieve attachment from Blob Store",
                component="g8ee",
                cause=e,
                details={"store_key": store_key}
            ) from e

    async def get_attachments_by_metadata(
        self,
        attachment_metadata: list[AttachmentMetadata]
    ) -> list[AttachmentData]:
        """
        Retrieve full attachment data for a list of attachment metadata from g8ed.

        Args:
            attachment_metadata: List of AttachmentMetadata from g8ed

        Returns:
            List of AttachmentData with base64_data included
        """
        if not attachment_metadata:
            return []

        attachments: list[AttachmentData] = []
        for meta in attachment_metadata:
            store_key = meta.store_key
            if not store_key:
                logger.warning(
                    "[ATTACHMENTS] Skipping attachment without store_key",
                    extra={"attachment_filename": meta.filename}
                )
                continue

            attachment_data = await self.get_attachment(store_key)
            if attachment_data:
                attachments.append(attachment_data)
            else:
                logger.warning(
                    "[ATTACHMENTS] Attachment expired or missing",
                    extra={
                        "store_key": store_key,
                        "attachment_filename": meta.filename
                    }
                )

        logger.info(
            "[ATTACHMENTS] Retrieved attachments from g8es",
            extra={"retrieved": len(attachments), "total": len(attachment_metadata)}
        )
        return attachments

    def classify_for_db(
        self,
        attachments: list[AttachmentMetadata],
    ) -> list[AttachmentRecord]:
        records: list[AttachmentRecord] = []
        for att in attachments:
            is_pdf = att.content_type == "application/pdf"
            is_image = self._is_supported_image(att.content_type)
            is_text = self._is_text_file(att.content_type, att.filename)

            if is_pdf:
                att_type = AttachmentType.PDF
            elif is_image:
                att_type = AttachmentType.IMAGE
            elif is_text:
                att_type = AttachmentType.TEXT
            else:
                att_type = AttachmentType.OTHER

            records.append(AttachmentRecord(
                filename=att.filename,
                content_type=att.content_type,
                size=att.file_size,
                type=att_type,
            ))
        return records

    async def process_attachments(
        self,
        attachments: list[AttachmentData],
    ) -> list[ProcessedAttachment]:
        """
        Classify and prepare attachments for LLM provider consumption.

        Args:
            attachments: List of AttachmentData with base64_data, filename, content_type, file_size

        Returns:
            List of ProcessedAttachment ready for the LLM provider
        """
        if not attachments:
            return []

        logger.info(
            "[ATTACHMENTS] Processing attachments",
            extra={"count": len(attachments)}
        )

        processed_files: list[ProcessedAttachment] = []

        for attachment in attachments:
            try:
                if not attachment.base64_data:
                    logger.warning(
                        "[ATTACHMENTS] Skipping attachment with no base64_data",
                        extra={"attachment_filename": attachment.filename}
                    )
                    continue

                is_pdf = attachment.content_type == "application/pdf"
                is_image = self._is_supported_image(attachment.content_type)
                is_text = self._is_text_file(attachment.content_type, attachment.filename)

                if is_pdf:
                    att_type = AttachmentType.PDF
                elif is_image:
                    att_type = AttachmentType.IMAGE
                elif is_text:
                    att_type = AttachmentType.TEXT
                else:
                    att_type = AttachmentType.OTHER

                decoded_content: str | None = None
                if is_text and (attachment.file_size or 0) < 5 * 1024 * 1024:
                    try:
                        decoded_bytes = base64.b64decode(attachment.base64_data)
                        decoded_content = decoded_bytes.decode("utf-8", errors="replace")
                    except Exception as decode_err:
                        logger.warning(
                            "[ATTACHMENTS] Failed to decode text content",
                            extra={"attachment_filename": attachment.filename, "error": str(decode_err)}
                        )

                file_size_mb = (attachment.file_size or 0) / (1024 * 1024)
                logger.info(
                    "[ATTACHMENTS] Classified attachment",
                    extra={
                        "attachment_filename": attachment.filename,
                        "type": att_type,
                        "size_mb": round(file_size_mb, 2),
                        "content_type": attachment.content_type,
                    }
                )

                processed_files.append(ProcessedAttachment(
                    filename=attachment.filename,
                    content_type=attachment.content_type,
                    file_size=attachment.file_size,
                    base64_data=attachment.base64_data,
                    attachment_type=att_type,
                    content=decoded_content,
                ))

            except Exception as e:
                logger.error(
                    "[ATTACHMENTS] Failed to process attachment",
                    extra={"attachment_filename": attachment.filename, "error": str(e)}
                )
                continue

        logger.info(
            "[ATTACHMENTS] Processing complete",
            extra={"processed": len(processed_files), "total": len(attachments)}
        )
        return processed_files
