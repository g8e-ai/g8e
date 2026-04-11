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
AttachmentService tests.

Covers:
- g8es Blob Store retrieval (get_attachment, get_attachments_by_metadata)
- Attachment classification and content extraction (process_attachments)
"""

import base64

import pytest

from app.constants import AttachmentType
from app.errors import NetworkError
from app.models.attachments import AttachmentData, AttachmentMetadata
from app.services.data.attachment_store_service import AttachmentService

pytestmark = pytest.mark.unit


@pytest.mark.asyncio(loop_scope="session")
class TestAttachmentServiceRetrieval:
    """Blob Store retrieval methods."""

    @pytest.fixture
    def service(self, mock_blob_service, mock_settings):
        return AttachmentService(mock_blob_service, mock_settings)

    # ========================================================================
    # GET SINGLE ATTACHMENT
    # ========================================================================

    async def test_get_attachment_success(self, service, mock_blob_service):
        store_key = "att:inv123/abc123"
        attachment = AttachmentData(
            filename="test.txt",
            content_type="text/plain",
            file_size=100,
            base64_data="SGVsbG8gV29ybGQ=",
        )
        mock_blob_service.get_blob.return_value = attachment.model_dump_json().encode()

        result = await service.get_attachment(store_key)

        assert result is not None
        assert result.filename == "test.txt"
        assert result.base64_data == "SGVsbG8gV29ybGQ="
        mock_blob_service.get_blob.assert_called_once_with("att:inv123", "abc123")

    async def test_get_attachment_not_found(self, service, mock_blob_service):
        store_key = "att:inv123/expired"
        mock_blob_service.get_blob.return_value = None

        result = await service.get_attachment(store_key)

        assert result is None

    async def test_get_attachment_store_error(self, service, mock_blob_service):
        store_key = "att:inv123/abc123"
        mock_blob_service.get_blob.side_effect = Exception("g8es connection lost")

        with pytest.raises(NetworkError):
            await service.get_attachment(store_key)

    async def test_get_attachment_invalid_json(self, service, mock_blob_service):
        store_key = "att:inv123/abc123"
        mock_blob_service.get_blob.return_value = b"not valid json{{"

        with pytest.raises(NetworkError):
            await service.get_attachment(store_key)

    # ========================================================================
    # GET ATTACHMENTS BY METADATA
    # ========================================================================

    async def test_get_attachments_by_metadata_success(self, service, mock_blob_service):
        key1 = "att:inv1/a1"
        key2 = "att:inv1/a2"
        
        att1 = AttachmentData(filename="file1.txt", base64_data="data1")
        att2 = AttachmentData(filename="file2.pdf", base64_data="data2")
        
        mock_blob_service.get_blob.side_effect = [
            att1.model_dump_json().encode(),
            att2.model_dump_json().encode()
        ]

        metadata = [
            AttachmentMetadata(store_key=key1, filename="file1.txt"),
            AttachmentMetadata(store_key=key2, filename="file2.pdf"),
        ]

        result = await service.get_attachments_by_metadata(metadata)

        assert len(result) == 2
        assert result[0].filename == "file1.txt"
        assert result[1].filename == "file2.pdf"

    async def test_get_attachments_by_metadata_empty_list(self, service):
        result = await service.get_attachments_by_metadata([])
        assert result == []

    async def test_get_attachments_by_metadata_none(self, service):
        result = await service.get_attachments_by_metadata(None)
        assert result == []

    async def test_get_attachments_by_metadata_skips_missing_store_key(self, service, mock_blob_service):
        key1 = "att:inv1/a1"
        att1 = AttachmentData(filename="file1.txt", base64_data="data1")
        mock_blob_service.get_blob.return_value = att1.model_dump_json().encode()

        metadata = [
            AttachmentMetadata(filename="no_key.txt"),
            AttachmentMetadata(store_key=key1, filename="file1.txt"),
        ]

        result = await service.get_attachments_by_metadata(metadata)

        assert len(result) == 1
        assert result[0].filename == "file1.txt"

    async def test_get_attachments_by_metadata_handles_expired(self, service, mock_blob_service):
        key1 = "att:inv1/a1"
        key2 = "att:inv1/a2"
        att1 = AttachmentData(filename="file1.txt", base64_data="data1")
        
        # First call returns data, second returns None (expired)
        mock_blob_service.get_blob.side_effect = [att1.model_dump_json().encode(), None]

        metadata = [
            AttachmentMetadata(store_key=key1, filename="file1.txt"),
            AttachmentMetadata(store_key=key2, filename="expired.txt"),
        ]

        result = await service.get_attachments_by_metadata(metadata)

        assert len(result) == 1
        assert result[0].filename == "file1.txt"



class TestAttachmentServiceFileTypeDetection:
    """File type detection helpers."""

    @pytest.fixture
    def service(self, mock_blob_service, mock_settings):
        return AttachmentService(mock_blob_service, mock_settings)

    def test_is_text_file_by_mime_type(self, service):
        assert service._is_text_file("text/plain", "file.txt") is True
        assert service._is_text_file("text/html", "file.html") is True
        assert service._is_text_file("application/json", "data.json") is True
        assert service._is_text_file("application/xml", "config.xml") is True
        assert service._is_text_file("application/yaml", "deploy.yaml") is True
        assert service._is_text_file("application/javascript", "script.js") is True
        assert service._is_text_file("application/sql", "query.sql") is True

    def test_is_text_file_by_extension(self, service):
        assert service._is_text_file("application/octet-stream", "file.txt") is True
        assert service._is_text_file("application/octet-stream", "app.log") is True
        assert service._is_text_file("application/octet-stream", "script.py") is True
        assert service._is_text_file("application/octet-stream", "config.yaml") is True
        assert service._is_text_file("application/octet-stream", "data.json") is True
        assert service._is_text_file("application/octet-stream", "style.css") is True
        assert service._is_text_file("application/octet-stream", "README.md") is True
        assert service._is_text_file("application/octet-stream", "script.sh") is True

    def test_is_text_file_binary_files(self, service):
        assert service._is_text_file("image/png", "photo.png") is False
        assert service._is_text_file("application/pdf", "doc.pdf") is False
        assert service._is_text_file("video/mp4", "video.mp4") is False
        assert service._is_text_file("application/zip", "archive.zip") is False
        assert service._is_text_file("application/octet-stream", "binary.exe") is False

    def test_is_supported_image(self, service):
        assert service._is_supported_image("image/png") is True
        assert service._is_supported_image("image/jpeg") is True
        assert service._is_supported_image("image/jpg") is True
        assert service._is_supported_image("image/webp") is True
        assert service._is_supported_image("image/heic") is True
        assert service._is_supported_image("image/heif") is True
        assert service._is_supported_image("image/gif") is True
        assert service._is_supported_image("image/bmp") is True
        assert service._is_supported_image("image/svg+xml") is True
        assert service._is_supported_image("IMAGE/PNG") is True
        assert service._is_supported_image("Image/JPEG") is True
        assert service._is_supported_image("text/plain") is False
        assert service._is_supported_image("application/pdf") is False
        assert service._is_supported_image(None) is False
        assert service._is_supported_image("") is False


@pytest.mark.asyncio(loop_scope="session")
class TestAttachmentServiceProcessing:
    """process_attachments — classification and content extraction."""

    @pytest.fixture
    def service(self, mock_blob_service, mock_settings):
        return AttachmentService(mock_blob_service, mock_settings)

    # ========================================================================
    # TYPE CLASSIFICATION
    # ========================================================================

    async def test_process_attachments_empty_list(self, service):
        result = await service.process_attachments([])
        assert result == []

    async def test_process_attachments_text_file(self, service):
        text_content = "Hello, this is a log file"
        b64 = base64.b64encode(text_content.encode()).decode()

        attachments = [AttachmentData(
            filename="app.log",
            content_type="text/plain",
            file_size=len(text_content),
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        att = result[0]
        assert att.filename == "app.log"
        assert att.attachment_type == AttachmentType.TEXT
        assert att.base64_data == b64
        assert att.content == text_content

    async def test_process_attachments_pdf(self, service):
        pdf_bytes = b"%PDF-1.4 fake pdf content"
        b64 = base64.b64encode(pdf_bytes).decode()

        attachments = [AttachmentData(
            filename="document.pdf",
            content_type="application/pdf",
            file_size=len(pdf_bytes),
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        att = result[0]
        assert att.filename == "document.pdf"
        assert att.attachment_type == AttachmentType.PDF
        assert att.base64_data == b64
        assert att.content is None

    async def test_process_attachments_image(self, service):
        img_bytes = b"\x89PNG\r\n\x1a\n fake png"
        b64 = base64.b64encode(img_bytes).decode()

        attachments = [AttachmentData(
            filename="screenshot.png",
            content_type="image/png",
            file_size=len(img_bytes),
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        att = result[0]
        assert att.filename == "screenshot.png"
        assert att.attachment_type == AttachmentType.IMAGE
        assert att.base64_data == b64
        assert att.content is None

    async def test_process_attachments_unknown_type_classified_as_other(self, service):
        b64 = base64.b64encode(b"zip data").decode()

        attachments = [AttachmentData(
            filename="archive.zip",
            content_type="application/zip",
            file_size=8,
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        assert result[0].attachment_type == AttachmentType.OTHER
        assert result[0].content is None

    async def test_process_attachments_jpeg_image(self, service):
        b64 = base64.b64encode(b"\xff\xd8\xff\xe0 fake jpeg").decode()

        attachments = [AttachmentData(
            filename="photo.jpg",
            content_type="image/jpeg",
            file_size=16,
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        assert result[0].attachment_type == AttachmentType.IMAGE

    async def test_process_attachments_yaml_by_extension(self, service):
        yaml_content = "server:\n  port: 8080"
        b64 = base64.b64encode(yaml_content.encode()).decode()

        attachments = [AttachmentData(
            filename="config.yaml",
            content_type="application/octet-stream",
            file_size=len(yaml_content),
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        assert result[0].attachment_type == AttachmentType.TEXT
        assert result[0].content == yaml_content

    async def test_process_attachments_json_by_extension(self, service):
        json_content = '{"key": "value"}'
        b64 = base64.b64encode(json_content.encode()).decode()

        attachments = [AttachmentData(
            filename="data.json",
            content_type="application/octet-stream",
            file_size=len(json_content),
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        assert result[0].attachment_type == AttachmentType.TEXT
        assert result[0].content == json_content

    # ========================================================================
    # CONTENT EXTRACTION
    # ========================================================================

    async def test_process_attachments_text_content_decoded(self, service):
        text = "Server error at line 42: connection refused"
        b64 = base64.b64encode(text.encode()).decode()

        attachments = [AttachmentData(
            filename="error.log",
            content_type="text/plain",
            file_size=len(text),
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        assert result[0].content == text

    async def test_process_attachments_large_text_file_skips_content_decode(self, service):
        b64 = base64.b64encode(b"x" * 100).decode()

        attachments = [AttachmentData(
            filename="huge.log",
            content_type="text/plain",
            file_size=6 * 1024 * 1024,
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        assert result[0].attachment_type == AttachmentType.TEXT
        assert result[0].content is None

    async def test_process_attachments_pdf_has_no_decoded_content(self, service):
        b64 = base64.b64encode(b"%PDF-1.4 data").decode()

        attachments = [AttachmentData(
            filename="report.pdf",
            content_type="application/pdf",
            file_size=13,
            base64_data=b64,
        )]

        result = await service.process_attachments(attachments)

        assert len(result) == 1
        assert result[0].content is None

    # ========================================================================
    # SKIP / ERROR HANDLING
    # ========================================================================

    async def test_process_attachments_skips_missing_base64_data(self, service):
        attachments = [AttachmentData(
            filename="test.txt",
            content_type="text/plain",
            file_size=4,
        )]

        result = await service.process_attachments(attachments)
        assert result == []

    async def test_process_attachments_skips_empty_base64_data(self, service):
        attachments = [AttachmentData(
            filename="test.txt",
            content_type="text/plain",
            file_size=4,
            base64_data="",
        )]

        result = await service.process_attachments(attachments)
        assert result == []

    async def test_process_attachments_continues_on_error(self, service):
        good_b64 = base64.b64encode(b"good data").decode()

        attachments = [
            AttachmentData(filename="file1.txt", content_type="text/plain", file_size=None, base64_data=good_b64),
            AttachmentData(filename="file2.pdf", content_type="application/pdf", file_size=9, base64_data=good_b64),
        ]

        result = await service.process_attachments(attachments)

        assert len(result) == 2

    # ========================================================================
    # MULTIPLE FILES
    # ========================================================================

    async def test_process_attachments_multiple_types(self, service):
        attachments = [
            AttachmentData(filename="notes.txt", content_type="text/plain", file_size=5, base64_data=base64.b64encode(b"hello").decode()),
            AttachmentData(filename="report.pdf", content_type="application/pdf", file_size=5, base64_data=base64.b64encode(b"pdf01").decode()),
            AttachmentData(filename="screen.png", content_type="image/png", file_size=5, base64_data=base64.b64encode(b"png01").decode()),
            AttachmentData(filename="data.zip", content_type="application/zip", file_size=5, base64_data=base64.b64encode(b"zip01").decode()),
        ]

        result = await service.process_attachments(attachments)

        assert len(result) == 4
        assert result[0].attachment_type == AttachmentType.TEXT
        assert result[1].attachment_type == AttachmentType.PDF
        assert result[2].attachment_type == AttachmentType.IMAGE
        assert result[3].attachment_type == AttachmentType.OTHER
