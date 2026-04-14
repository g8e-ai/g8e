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
Unit tests for app/models/investigations.py

Covers: Attachment
"""

import pytest

from app.models.investigations import Attachment

pytestmark = [pytest.mark.unit]


class TestAttachment:

    def test_has_id(self):
        att = Attachment(filename="report.pdf")
        assert att.id is not None
        assert isinstance(att.id, str)
        assert len(att.id) > 0

    def test_id_is_unique_per_instance(self):
        a = Attachment(filename="a.pdf")
        b = Attachment(filename="b.pdf")
        assert a.id != b.id

    def test_explicit_id_is_preserved(self):
        att = Attachment(id="att-abc-123", filename="report.pdf")
        assert att.id == "att-abc-123"

    def test_has_created_at(self):
        att = Attachment(filename="report.pdf")
        assert att.created_at is not None

    def test_updated_at_defaults_to_none(self):
        att = Attachment(filename="report.pdf")
        assert att.updated_at is None

    def test_filename_required(self):
        with pytest.raises(Exception):
            Attachment()

    def test_content_type_defaults_to_none(self):
        att = Attachment(filename="report.pdf")
        assert att.content_type is None

    def test_size_defaults_to_none(self):
        att = Attachment(filename="report.pdf")
        assert att.size is None

    def test_uploaded_by_defaults_to_none(self):
        att = Attachment(filename="report.pdf")
        assert att.uploaded_by is None

    def test_no_url_field(self):
        att = Attachment(filename="report.pdf")
        assert not hasattr(att, "url")

    def test_no_uploaded_at_field(self):
        att = Attachment(filename="report.pdf")
        assert not hasattr(att, "uploaded_at")

    def test_flatten_for_db_includes_id_and_created_at(self):
        att = Attachment(id="att-1", filename="report.pdf", content_type="application/pdf", size=1024)
        flat = att.flatten_for_db()
        assert flat["id"] == "att-1"
        assert "created_at" in flat
        assert flat["filename"] == "report.pdf"
        assert flat["content_type"] == "application/pdf"
        assert flat["size"] == 1024

    def test_flatten_for_db_omits_none_fields(self):
        att = Attachment(filename="report.pdf")
        flat = att.flatten_for_db()
        assert "content_type" not in flat
        assert "size" not in flat
        assert "uploaded_by" not in flat
        assert "updated_at" not in flat

    def test_url_field_silently_ignored_on_construction(self):
        att = Attachment(filename="report.pdf", url="https://example.com/file")
        assert not hasattr(att, "url")
