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

import pytest
from app.models.base import ValidationError

from app.constants import BatchWriteOpType, DB_COLLECTION_INVESTIGATIONS, InvestigationStatus
from app.models.cache import BatchWriteOperation, FieldFilter, QueryOrderBy

pytestmark = pytest.mark.unit


class TestFieldFilter:
    def test_construction_with_required_fields(self):
        f = FieldFilter(field="status", op="==", value=InvestigationStatus.OPEN)
        assert f.field == "status"
        assert f.op == "=="
        assert f.value == InvestigationStatus.OPEN

    def test_all_supported_operators(self):
        ops = ["==", "!=", "<", "<=", ">", ">=", "in", "not-in", "array-contains"]
        for op in ops:
            f = FieldFilter(field="x", op=op, value=1)  # type: ignore[arg-type]
            assert f.op == op

    def test_rejects_invalid_operator(self):
        with pytest.raises(ValidationError):
            FieldFilter(field="x", op="LIKE", value="foo")

    def test_value_accepts_string(self):
        f = FieldFilter(field="user_id", op="==", value="user-123")
        assert f.value == "user-123"

    def test_value_accepts_enum(self):
        f = FieldFilter(field="status", op="==", value=InvestigationStatus.OPEN)
        assert f.value == InvestigationStatus.OPEN

    def test_value_accepts_list_for_in_operator(self):
        f = FieldFilter(field="status", op="in", value=[InvestigationStatus.OPEN, InvestigationStatus.CLOSED])
        assert f.value == [InvestigationStatus.OPEN, InvestigationStatus.CLOSED]

    def test_db_dump_produces_plain_dict(self):
        f = FieldFilter(field="case_id", op="==", value="case-abc")
        d = f.model_dump(mode="json")
        assert d == {"field": "case_id", "op": "==", "value": "case-abc"}

    def test_wire_dump_produces_wire_safe_dict(self):
        f = FieldFilter(field="status", op="==", value=InvestigationStatus.OPEN)
        d = f.model_dump(mode="json")
        assert isinstance(d, dict)
        assert "field" in d and "op" in d and "value" in d


class TestQueryOrderBy:
    def test_default_direction_is_asc(self):
        o = QueryOrderBy(field="created_at")
        assert o.direction == "asc"

    def test_desc_direction(self):
        o = QueryOrderBy(field="updated_at", direction="desc")
        assert o.field == "updated_at"
        assert o.direction == "desc"

    def test_rejects_invalid_direction(self):
        with pytest.raises(ValidationError):
            QueryOrderBy(field="created_at", direction="ascending")  # type: ignore[arg-type]

    def test_produces_correct_client_dict(self):
        o = QueryOrderBy(field="created_at", direction="desc")
        assert {o.field: o.direction} == {"created_at": "desc"}

    def test_asc_direction_accepted(self):
        o = QueryOrderBy(field="created_at", direction="asc")
        assert o.direction == "asc"


class TestBatchWriteOpType:
    def test_is_str_subclass(self):
        assert issubclass(BatchWriteOpType, str)

    def test_all_members_present(self):
        members = {m for m in BatchWriteOpType}
        assert members == {BatchWriteOpType.SET, BatchWriteOpType.UPDATE, BatchWriteOpType.DELETE}


class TestBatchWriteOperation:
    def test_default_op_type_is_set(self):
        op = BatchWriteOperation(collection="docs", doc_id="d1")
        assert op.op_type == BatchWriteOpType.SET

    def test_default_merge_is_false(self):
        op = BatchWriteOperation(collection="col", doc_id="d1")
        assert op.merge is False

    def test_default_data_is_empty_dict(self):
        op = BatchWriteOperation(collection="col", doc_id="d1")
        assert op.data == {}

    def test_all_op_types_constructable(self):
        for op_type in BatchWriteOpType:
            op = BatchWriteOperation(op_type=op_type, collection="col", doc_id="d1")
            assert op.op_type == op_type

    def test_data_field_accepts_arbitrary_payload(self):
        payload = {"name": "alice", "count": 42, "tags": ["a", "b"]}
        op = BatchWriteOperation(collection="col", doc_id="d1", data=payload)
        assert op.data == payload

    def test_merge_true_for_update_op(self):
        op = BatchWriteOperation(
            op_type=BatchWriteOpType.UPDATE,
            collection="col",
            doc_id="d1",
            data={"x": 1},
            merge=True,
        )
        assert op.merge is True

    def test_delete_op_no_data_required(self):
        op = BatchWriteOperation(op_type=BatchWriteOpType.DELETE, collection="col", doc_id="d1")
        assert op.op_type == BatchWriteOpType.DELETE
        assert op.data == {}

    def test_collection_and_doc_id_stored(self):
        op = BatchWriteOperation(
            op_type=BatchWriteOpType.SET,
            collection=DB_COLLECTION_INVESTIGATIONS,
            doc_id="inv-abc-123",
            data={"status": InvestigationStatus.OPEN},
        )
        assert op.collection == DB_COLLECTION_INVESTIGATIONS
        assert op.doc_id == "inv-abc-123"
