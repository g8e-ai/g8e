# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.


from app.constants import AttachmentType

from .base import G8eBaseModel, Field


class AttachmentMetadata(G8eBaseModel):
    """
    Attachment reference passed from client to G8EE.

    Contains the operator KV store key and file metadata needed to retrieve
    the full attachment data. client stores the binary content; g8ee retrieves
    it via this key before processing.
    """

    store_key: str | None = Field(
        default=None, description="Primary operator KV key (attachment:{inv_id}:{att_id})"
    )
    filename: str = Field(..., description="Original filename")
    file_size: int | None = Field(default=None, description="File size in bytes")
    content_type: str = Field(default="application/octet-stream", description="MIME content type")


class AttachmentData(G8eBaseModel):
    """
    Full attachment payload retrieved from operator KV store.

    Produced by AttachmentService.get_attachments_by_metadata() and consumed by
    AttachmentService.process_attachments() for classification and LLM formatting.
    """

    filename: str = Field(..., description="Original filename")
    content_type: str = Field(default="application/octet-stream", description="MIME content type")
    file_size: int | None = Field(default=None, description="File size in bytes")
    base64_data: str = Field(default="", description="Base64-encoded file content")


class ProcessedAttachment(G8eBaseModel):
    """
    Attachment after classification by AttachmentService.

    Produced by AttachmentService.process_attachments() and consumed by
    AIRequestBuilder.format_attachment_parts() to build LLM Part objects.
    """

    filename: str = Field(..., description="Original filename")
    content_type: str = Field(default="application/octet-stream", description="MIME content type")
    file_size: int | None = Field(default=None, description="File size in bytes")
    base64_data: str = Field(default="", description="Base64-encoded file content")
    attachment_type: AttachmentType = Field(
        default=AttachmentType.OTHER,
        description="Classified attachment type derived from content_type and filename",
    )
    content: str | None = Field(
        default=None, description="Pre-decoded UTF-8 text content (text files only)"
    )
