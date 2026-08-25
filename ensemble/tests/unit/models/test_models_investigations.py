# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

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

    def test_db_dump_includes_id_and_created_at(self):
        att = Attachment(
            id="att-1", filename="report.pdf", content_type="application/pdf", size=1024
        )
        flat = att.model_dump(mode="json")
        assert flat["id"] == "att-1"
        assert "created_at" in flat
        assert flat["filename"] == "report.pdf"
        assert flat["content_type"] == "application/pdf"
        assert flat["size"] == 1024

    def test_db_dump_omits_none_fields(self):
        att = Attachment(filename="report.pdf")
        flat = att.model_dump(mode="json")
        assert "content_type" not in flat
        assert "size" not in flat
        assert "uploaded_by" not in flat
        assert "updated_at" not in flat

    def test_url_field_silently_ignored_on_construction(self):
        att = Attachment(filename="report.pdf", url="https://example.com/file")
        assert not hasattr(att, "url")
