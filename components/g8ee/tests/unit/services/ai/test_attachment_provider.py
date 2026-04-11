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
Unit tests for AttachmentGroundingProvider.

Pure logic — no external I/O.
- format_parts: PDF, image, text (pre-decoded), text (base64), OTHER, missing base64_data
"""

import base64

import pytest

from app.constants.settings import AttachmentType
from app.models.attachments import ProcessedAttachment
from app.services.ai.grounding.attachment_provider import AttachmentGroundingProvider

pytestmark = [pytest.mark.unit]


@pytest.fixture
def provider() -> AttachmentGroundingProvider:
    return AttachmentGroundingProvider()


def _b64(content: bytes) -> str:
    return base64.b64encode(content).decode("utf-8")


def _make_att(
    filename: str,
    content_type: str,
    attachment_type: AttachmentType,
    content: str = "",
    base64_data: str = "",
) -> ProcessedAttachment:
    return ProcessedAttachment(
        filename=filename,
        content_type=content_type,
        attachment_type=attachment_type,
        base64_data=base64_data,
        content=content,
    )


class TestFormatParts:
    """AttachmentGroundingProvider.format_parts converts attachments to LLM Parts."""

    def test_returns_empty_list_for_none(self, provider):
        assert provider.format_parts(None) == []

    def test_returns_empty_list_for_empty_list(self, provider):
        assert provider.format_parts([]) == []

    def test_skips_attachment_with_no_base64_data(self, provider):
        att = _make_att("empty.pdf", "application/pdf", AttachmentType.PDF, base64_data="")
        parts = provider.format_parts([att])
        assert parts == []

    def test_pdf_produces_inline_bytes_part(self, provider):
        pdf_bytes = b"%PDF-1.4 fake pdf content"
        att = _make_att(
            "doc.pdf", "application/pdf", AttachmentType.PDF,
            base64_data=_b64(pdf_bytes),
        )
        parts = provider.format_parts([att])
        assert len(parts) == 1
        part = parts[0]
        assert part.inline_data is not None
        assert part.inline_data.data == pdf_bytes
        assert part.inline_data.mime_type == "application/pdf"

    def test_image_produces_inline_bytes_part(self, provider):
        img_bytes = b"\x89PNG\r\n fake image"
        att = _make_att(
            "photo.png", "image/png", AttachmentType.IMAGE,
            base64_data=_b64(img_bytes),
        )
        parts = provider.format_parts([att])
        assert len(parts) == 1
        part = parts[0]
        assert part.inline_data is not None
        assert part.inline_data.mime_type == "image/png"

    def test_text_with_pre_decoded_content_produces_text_part(self, provider):
        att = _make_att(
            "notes.txt", "text/plain", AttachmentType.TEXT,
            base64_data=_b64(b"some text"),
            content="some text",
        )
        parts = provider.format_parts([att])
        assert len(parts) == 1
        part = parts[0]
        assert part.text is not None
        assert "notes.txt" in part.text
        assert "some text" in part.text

    def test_text_with_base64_only_decodes_and_produces_text_part(self, provider):
        raw = "hello from base64"
        att = _make_att(
            "readme.txt", "text/plain", AttachmentType.TEXT,
            base64_data=_b64(raw.encode("utf-8")),
        )
        parts = provider.format_parts([att])
        assert len(parts) == 1
        part = parts[0]
        assert part.text is not None
        assert "readme.txt" in part.text
        assert "hello from base64" in part.text

    def test_text_part_includes_filename_header_and_footer(self, provider):
        att = _make_att(
            "config.yaml", "text/plain", AttachmentType.TEXT,
            base64_data=_b64(b"key: value"),
            content="key: value",
        )
        parts = provider.format_parts([att])
        assert len(parts) == 1
        text = parts[0].text
        assert text is not None
        assert "--- Attached Document: config.yaml ---" in text
        assert "--- End of config.yaml ---" in text

    def test_other_type_skipped(self, provider):
        att = _make_att(
            "archive.zip", "application/zip", AttachmentType.OTHER,
            base64_data=_b64(b"zip content"),
        )
        parts = provider.format_parts([att])
        assert parts == []

    def test_multiple_attachments_all_processed(self, provider):
        pdf_bytes = b"%PDF fake"
        txt_content = "hello world"
        atts = [
            _make_att("a.pdf", "application/pdf", AttachmentType.PDF, base64_data=_b64(pdf_bytes)),
            _make_att("b.txt", "text/plain", AttachmentType.TEXT, base64_data=_b64(txt_content.encode()), content=txt_content),
        ]
        parts = provider.format_parts(atts)
        assert len(parts) == 2

    def test_mixed_list_skips_other_keeps_rest(self, provider):
        atts = [
            _make_att("doc.pdf", "application/pdf", AttachmentType.PDF, base64_data=_b64(b"pdf")),
            _make_att("data.bin", "application/octet-stream", AttachmentType.OTHER, base64_data=_b64(b"bin")),
            _make_att("note.txt", "text/plain", AttachmentType.TEXT, base64_data=_b64(b"note"), content="note"),
        ]
        parts = provider.format_parts(atts)
        assert len(parts) == 2

    def test_invalid_base64_raises_operation_error_for_pdf(self, provider):
        from app.errors import ValidationError
        att = _make_att(
            "bad.pdf", "application/pdf", AttachmentType.PDF,
            base64_data="!!!not-valid-base64!!!",
        )
        with pytest.raises(ValidationError):
            provider.format_parts([att])

    def test_invalid_base64_raises_operation_error_for_text(self, provider):
        from app.errors import ValidationError
        att = _make_att(
            "bad.txt", "text/plain", AttachmentType.TEXT,
            base64_data="!!!not-valid-base64!!!",
        )
        with pytest.raises(ValidationError):
            provider.format_parts([att])
