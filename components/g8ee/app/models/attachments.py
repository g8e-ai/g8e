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


from pydantic import Field

from app.constants import AttachmentType

from .base import VSOBaseModel


class AttachmentMetadata(VSOBaseModel):
    """
    Attachment reference passed from VSOD to G8EE.

    Contains the VSODB KV store key and file metadata needed to retrieve
    the full attachment data. VSOD stores the binary content; g8ee retrieves
    it via this key before processing.
    """
    store_key: str | None = Field(default=None, description="Primary VSODB KV key (attachment:{inv_id}:{att_id})")
    filename: str = Field(..., description="Original filename")
    file_size: int | None = Field(default=None, description="File size in bytes")
    content_type: str = Field(default="application/octet-stream", description="MIME content type")


class AttachmentData(VSOBaseModel):
    """
    Full attachment payload retrieved from VSODB KV store.

    Produced by AttachmentService.get_attachments_by_metadata() and consumed by
    AttachmentService.process_attachments() for classification and LLM formatting.
    """
    filename: str = Field(..., description="Original filename")
    content_type: str = Field(default="application/octet-stream", description="MIME content type")
    file_size: int | None = Field(default=None, description="File size in bytes")
    base64_data: str = Field(default="", description="Base64-encoded file content")


class ProcessedAttachment(VSOBaseModel):
    """
    Attachment after classification by AttachmentService.

    Produced by AttachmentService.process_attachments() and consumed by
    AIRequestBuilder.format_attachment_parts() to build LLM Part objects.
    """
    filename: str = Field(..., description="Original filename")
    content_type: str = Field(default="application/octet-stream", description="MIME content type")
    file_size: int | None = Field(default=None, description="File size in bytes")
    base64_data: str = Field(default="", description="Base64-encoded file content")
    attachment_type: AttachmentType = Field(default=AttachmentType.OTHER, description="Classified attachment type derived from content_type and filename")
    content: str | None = Field(default=None, description="Pre-decoded UTF-8 text content (text files only)")
