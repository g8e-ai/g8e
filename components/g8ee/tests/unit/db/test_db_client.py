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

"""Unit tests for DBClient.

DBClient is an HTTP shim over VSODB. Tests mock the HTTP layer via
InMemoryVSODB to validate all client-side logic: CRUD, timestamp resolution,
ArrayUnion/ArrayRemove, batch_write, and typed error propagation.
"""

import copy

import pytest

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

from app.constants import BatchWriteOpType, InvestigationStatus
from app.db import ArrayRemove, ArrayUnion, DBClient
from app.errors import NetworkError, ResourceNotFoundError
from app.models.cache import BatchWriteOperation


class InMemoryVSODB:
    """In-memory backend wired to DBClient typed _request variants for unit tests."""

    def __init__(self):
        self._store: dict[str, dict[str, dict]] = {}

    def _col(self, collection: str) -> dict:
        return self._store.setdefault(collection, {})

    async def handle_json(self, method: str, path: str, **kwargs) -> dict | None:
        parts = path.strip("/").split("/")
        if len(parts) == 3 and parts[0] == "db":
            collection, doc_id = parts[1], parts[2]
            if method == "GET":
                doc = self._col(collection).get(doc_id)
                return copy.deepcopy(doc) if doc else None
            if method == "PUT":
                data = kwargs.get("json", {})
                data["id"] = doc_id
                self._col(collection)[doc_id] = data
                return {"success": True}
            if method == "PATCH":
                col = self._col(collection)
                if doc_id not in col:
                    return None
                col[doc_id].update(kwargs.get("json", {}))
                return {"success": True}
        return None

    async def handle_list(self, method: str, path: str, **kwargs) -> list:
        parts = path.strip("/").split("/")
        if len(parts) == 3 and parts[0] == "db" and parts[2] == "_query":
            collection = parts[1]
            body = kwargs.get("json", {})
            docs = list(self._col(collection).values())
            for f in body.get("filters", []):
                field = f.get("field")
                op = f.get("op", "==")
                value = f.get("value")
                if op == "==":
                    docs = [d for d in docs if d.get(field) == value]
            order_by_str = body.get("order_by", "")
            if order_by_str:
                ob_parts = order_by_str.split()
                if len(ob_parts) == 2:
                    ob_field, direction = ob_parts
                    docs = sorted(docs, key=lambda d: d.get(ob_field, ""), reverse=(direction.upper() == "DESC"))
            limit = body.get("limit")
            if limit is not None:
                docs = docs[:limit]
            return docs
        return []

    async def handle_void(self, method: str, path: str, **kwargs) -> None:
        parts = path.strip("/").split("/")
        if len(parts) == 3 and parts[0] == "db":
            collection, doc_id = parts[1], parts[2]
            if method == "PUT":
                data = kwargs.get("json", {})
                data["id"] = doc_id
                self._col(collection)[doc_id] = data
            elif method == "PATCH":
                col = self._col(collection)
                if doc_id in col:
                    col[doc_id].update(kwargs.get("json", {}))
            elif method == "DELETE":
                self._col(collection).pop(doc_id, None)


@pytest.fixture
def db_client():
    store = InMemoryVSODB()
    client = DBClient(ca_cert_path="/mock/ca.crt", internal_auth_token="mock-token")
    client._request_json = store.handle_json
    client._request_list = store.handle_list
    client._request_void = store.handle_void
    return client


@pytest.fixture
def db_client_http_error():
    client = DBClient(ca_cert_path="/mock/ca.crt", internal_auth_token="mock-token")

    async def raise_network_error(method, path, **kwargs):
        raise NetworkError("VSODB HTTP 500: internal error", component="g8ee")

    client._request_json = raise_network_error
    client._request_list = raise_network_error
    client._request_void = raise_network_error
    return client


class TestCRUD:
    async def test_create_and_get(self, db_client):
        result = await db_client.create_document("docs", "d1", {"name": "alice"})
        assert result.success is True
        assert result.document_id == "d1"

        got = await db_client.get_document("docs", "d1")
        assert got.success is True
        assert got.data["name"] == "alice"

    async def test_get_missing_document_returns_none_data(self, db_client):
        got = await db_client.get_document("docs", "nonexistent")
        assert got.success is True
        assert got.data is None

    async def test_update_document_merge_patches_fields(self, db_client):
        await db_client.create_document("docs", "d1", {"a": 1, "b": 2})
        result = await db_client.update_document("docs", "d1", {"b": 99})
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["a"] == 1
        assert got.data["b"] == 99

    async def test_update_document_replace_overwrites(self, db_client):
        await db_client.create_document("docs", "d1", {"a": 1, "b": 2})
        result = await db_client.update_document("docs", "d1", {"b": 99}, merge=False)
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["b"] == 99

    async def test_delete_document(self, db_client):
        await db_client.create_document("docs", "d1", {"x": 1})
        result = await db_client.delete_document("docs", "d1")
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data is None

    async def test_query_collection_no_filters(self, db_client):
        await db_client.create_document("items", "i1", {"status": InvestigationStatus.OPEN})
        await db_client.create_document("items", "i2", {"status": InvestigationStatus.CLOSED})

        result = await db_client.query_collection("items", field_filters=[], order_by={}, limit=0, select_fields=[])
        assert result.success is True
        assert len(result.data) == 2

    async def test_query_collection_with_filter(self, db_client):
        await db_client.create_document("items", "i1", {"status": InvestigationStatus.OPEN})
        await db_client.create_document("items", "i2", {"status": InvestigationStatus.CLOSED})

        result = await db_client.query_collection(
            "items",
            field_filters=[{"field": "status", "op": "==", "value": InvestigationStatus.OPEN}],
            order_by={},
            limit=0,
            select_fields=[],
        )
        assert result.success is True
        assert len(result.data) == 1
        assert result.data[0]["status"] == InvestigationStatus.OPEN

    async def test_query_collection_empty_returns_empty_list(self, db_client):
        result = await db_client.query_collection("empty-col", field_filters=[], order_by={}, limit=0, select_fields=[])
        assert result.success is True
        assert result.data == []

    async def test_query_collection_with_select_fields(self, db_client):
        await db_client.create_document("items", "i1", {"name": "foo", "secret": "x"})

        result = await db_client.query_collection("items", field_filters=[], order_by={}, limit=0, select_fields=["name"])
        assert result.success is True
        doc = result.data[0]
        assert "name" in doc
        assert "secret" not in doc
        assert "id" in doc

    async def test_count_documents_unfiltered(self, db_client):
        await db_client.create_document("items", "i1", {"t": "a"})
        await db_client.create_document("items", "i2", {"t": "b"})
        count = await db_client.count_documents("items", field_filters=[])
        assert count == 2

    async def test_count_documents_with_filter(self, db_client):
        await db_client.create_document("items", "i1", {"t": "a"})
        await db_client.create_document("items", "i2", {"t": "b"})
        count = await db_client.count_documents(
            "items", field_filters=[{"field": "t", "op": "==", "value": "a"}]
        )
        assert count == 1

    async def test_count_documents_empty_collection(self, db_client):
        count = await db_client.count_documents("nonexistent", field_filters=[])
        assert count == 0


class TestHyphenatedCollectionNames:
    async def test_create_and_get_hyphenated(self, db_client):
        result = await db_client.create_document("api_keys", "key-1", {"hash": "abc"})
        assert result.success is True

        got = await db_client.get_document("api_keys", "key-1")
        assert got.data["hash"] == "abc"

    async def test_update_hyphenated(self, db_client):
        await db_client.create_document("api_keys", "key-1", {"hash": "abc"})
        result = await db_client.update_document("api_keys", "key-1", {"hash": "xyz"})
        assert result.success is True

        got = await db_client.get_document("api_keys", "key-1")
        assert got.data["hash"] == "xyz"

    async def test_delete_hyphenated(self, db_client):
        await db_client.create_document("api_keys", "key-1", {"hash": "abc"})
        result = await db_client.delete_document("api_keys", "key-1")
        assert result.success is True

        got = await db_client.get_document("api_keys", "key-1")
        assert got.data is None

    async def test_query_hyphenated(self, db_client):
        await db_client.create_document("api_keys", "key-1", {"user_id": "u1"})
        await db_client.create_document("api_keys", "key-2", {"user_id": "u1"})
        result = await db_client.query_collection(
            "api_keys",
            field_filters=[{"field": "user_id", "op": "==", "value": "u1"}],
            order_by={},
            limit=0,
            select_fields=[],
        )
        assert result.success is True
        assert len(result.data) == 2


class TestTimestampSentinel:
    async def test_client_passes_timestamp_string_through(self, db_client):
        result = await db_client.create_document(
            "docs", "d1", {"created_at": "2026-01-01T00:00:00Z", "name": "test"}
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["created_at"] == "2026-01-01T00:00:00Z"

    async def test_client_passes_update_timestamp_through(self, db_client):
        await db_client.create_document("docs", "d1", {"name": "test"})
        await db_client.update_document("docs", "d1", {"updated_at": "2026-06-01T12:00:00Z"})

        got = await db_client.get_document("docs", "d1")
        assert got.data["updated_at"] == "2026-06-01T12:00:00Z"


class TestArrayUnionRemove:
    async def test_array_union_appends_to_existing(self, db_client):
        await db_client.create_document("docs", "d1", {"tags": ["a", "b"]})
        result = await db_client.update_document(
            "docs", "d1", {"tags": ArrayUnion(["c", "d"], max_length=None)}
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["tags"] == ["a", "b", "c", "d"]

    async def test_array_union_initializes_missing_field(self, db_client):
        await db_client.create_document("docs", "d1", {"name": "test"})
        result = await db_client.update_document(
            "docs", "d1", {"tags": ArrayUnion(["x"], max_length=None)}
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["tags"] == ["x"]

    async def test_array_remove_removes_items(self, db_client):
        await db_client.create_document("docs", "d1", {"tags": ["a", "b", "c"]})
        result = await db_client.update_document(
            "docs", "d1", {"tags": ArrayRemove(["b"])}
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["tags"] == ["a", "c"]

    async def test_array_remove_missing_item_is_noop(self, db_client):
        await db_client.create_document("docs", "d1", {"tags": ["a", "b"]})
        result = await db_client.update_document(
            "docs", "d1", {"tags": ArrayRemove(["z"])}
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["tags"] == ["a", "b"]

    async def test_array_union_and_scalar_field_together(self, db_client):
        await db_client.create_document("docs", "d1", {"tags": ["a"], "count": 1})
        result = await db_client.update_document(
            "docs", "d1", {"tags": ArrayUnion(["b"], max_length=None), "count": 2}
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["tags"] == ["a", "b"]
        assert got.data["count"] == 2


class TestUpdateWithArrayUnion:
    async def test_append_to_empty_list(self, db_client):
        await db_client.create_document(
            "docs", "d1", {"history": []}
        )
        result = await db_client.update_with_array_union(
            collection="docs",
            document_id="d1",
            array_field="history",
            items_to_add=[{"role": "user", "content": "hi"}],
            additional_updates={},
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert len(got.data["history"]) == 1
        assert got.data["history"][0]["content"] == "hi"

    async def test_append_to_existing_list(self, db_client):
        await db_client.create_document(
            "docs", "d1",
            {"history": [{"role": "user", "content": "first"}]},
        )
        await db_client.update_with_array_union(
            collection="docs",
            document_id="d1",
            array_field="history",
            items_to_add=[{"role": "assistant", "content": "second"}],
            additional_updates={},
        )

        got = await db_client.get_document("docs", "d1")
        assert len(got.data["history"]) == 2
        assert got.data["history"][1]["content"] == "second"

    async def test_null_field_initializes_as_list(self, db_client):
        await db_client.create_document("docs", "d1", {"attachments": None})
        result = await db_client.update_with_array_union(
            collection="docs",
            document_id="d1",
            array_field="attachments",
            items_to_add=[{"name": "file.pdf"}],
            additional_updates={},
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["attachments"] == [{"name": "file.pdf"}]

    async def test_with_additional_updates(self, db_client):
        await db_client.create_document(
            "docs", "d1", {"history": [], "updated_at": None}
        )
        await db_client.update_with_array_union(
            collection="docs",
            document_id="d1",
            array_field="history",
            items_to_add=[{"role": "user", "content": "msg"}],
            additional_updates={"updated_at": "2026-02-28T00:00:00Z"},
        )

        got = await db_client.get_document("docs", "d1")
        assert got.data["updated_at"] == "2026-02-28T00:00:00Z"

    async def test_missing_document_raises_resource_not_found(self, db_client):
        with pytest.raises(ResourceNotFoundError) as exc_info:
            await db_client.update_with_array_union(
                collection="docs",
                document_id="nonexistent",
                array_field="history",
                items_to_add=[{"role": "user", "content": "hi"}],
                additional_updates={},
            )
        assert exc_info.value.get_http_status() == 404


class TestBatchWrite:
    async def test_set_operations(self, db_client):
        result = await db_client.batch_write([
            BatchWriteOperation(op_type=BatchWriteOpType.SET, collection="docs", doc_id="d1", data={"x": 1}),
            BatchWriteOperation(op_type=BatchWriteOpType.SET, collection="docs", doc_id="d2", data={"x": 2}),
        ])
        assert result.success is True

        assert (await db_client.get_document("docs", "d1")).data["x"] == 1
        assert (await db_client.get_document("docs", "d2")).data["x"] == 2

    async def test_update_operation(self, db_client):
        await db_client.create_document("docs", "d1", {"a": 1, "b": 2})
        result = await db_client.batch_write([
            BatchWriteOperation(
                op_type=BatchWriteOpType.UPDATE,
                collection="docs",
                doc_id="d1",
                data={"b": 99},
                merge=True,
            ),
        ])
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["a"] == 1
        assert got.data["b"] == 99

    async def test_delete_operation(self, db_client):
        await db_client.create_document("docs", "d1", {"x": 1})
        result = await db_client.batch_write([
            BatchWriteOperation(op_type=BatchWriteOpType.DELETE, collection="docs", doc_id="d1"),
        ])
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data is None

    async def test_mixed_operations_executed_in_order(self, db_client):
        result = await db_client.batch_write([
            BatchWriteOperation(op_type=BatchWriteOpType.SET, collection="docs", doc_id="d1", data={"v": 1}),
            BatchWriteOperation(op_type=BatchWriteOpType.UPDATE, collection="docs", doc_id="d1", data={"v": 2}, merge=True),
            BatchWriteOperation(op_type=BatchWriteOpType.SET, collection="docs", doc_id="d2", data={"v": 3}),
            BatchWriteOperation(op_type=BatchWriteOpType.DELETE, collection="docs", doc_id="d2"),
        ])
        assert result.success is True

        got_d1 = await db_client.get_document("docs", "d1")
        assert got_d1.data["v"] == 2

        got_d2 = await db_client.get_document("docs", "d2")
        assert got_d2.data is None

    async def test_empty_operations_returns_success(self, db_client):
        result = await db_client.batch_write([])
        assert result.success is True

    async def test_default_op_type_is_set(self, db_client):
        result = await db_client.batch_write([
            BatchWriteOperation(collection="docs", doc_id="d1", data={"x": 42}),
        ])
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["x"] == 42

    async def test_batch_write_propagates_error_mid_batch(self, db_client):
        call_count = 0

        async def fail_on_second(method, path, **kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 2:
                raise NetworkError("VSODB HTTP 500: mid-batch failure", component="g8ee")

        db_client._request_void = fail_on_second
        with pytest.raises(NetworkError):
            await db_client.batch_write([
                BatchWriteOperation(op_type=BatchWriteOpType.SET, collection="docs", doc_id="d1", data={"x": 1}),
                BatchWriteOperation(op_type=BatchWriteOpType.SET, collection="docs", doc_id="d2", data={"x": 2}),
            ])
        assert call_count == 2


class TestQueryCollectionOptions:
    async def test_order_by_desc(self, db_client):
        await db_client.create_document("items", "i1", {"score": "1"})
        await db_client.create_document("items", "i2", {"score": "3"})
        await db_client.create_document("items", "i3", {"score": "2"})

        result = await db_client.query_collection("items", field_filters=[], order_by={"score": "desc"}, limit=0, select_fields=[])
        assert result.success is True
        scores = [d["score"] for d in result.data]
        assert scores == sorted(scores, reverse=True)

    async def test_order_by_asc(self, db_client):
        await db_client.create_document("items", "i1", {"score": "3"})
        await db_client.create_document("items", "i2", {"score": "1"})
        await db_client.create_document("items", "i3", {"score": "2"})

        result = await db_client.query_collection("items", field_filters=[], order_by={"score": "asc"}, limit=0, select_fields=[])
        assert result.success is True
        scores = [d["score"] for d in result.data]
        assert scores == sorted(scores)

    async def test_limit_restricts_result_count(self, db_client):
        for i in range(5):
            await db_client.create_document("items", f"i{i}", {"n": i})

        result = await db_client.query_collection("items", field_filters=[], order_by={}, limit=3, select_fields=[])
        assert result.success is True
        assert len(result.data) == 3

    async def test_limit_larger_than_collection_returns_all(self, db_client):
        await db_client.create_document("items", "i1", {"n": 1})
        await db_client.create_document("items", "i2", {"n": 2})

        result = await db_client.query_collection("items", field_filters=[], order_by={}, limit=100, select_fields=[])
        assert result.success is True
        assert len(result.data) == 2


class TestUpdateDocumentArrayEdgeCases:
    async def test_array_union_when_existing_doc_returns_none_succeeds(self, db_client):
        async def return_none_on_get(method, path, **kwargs):
            return None

        db_client._request_json = return_none_on_get
        db_client._request_void = return_none_on_get
        result = await db_client.update_document(
            "docs", "d1", {"tags": ArrayUnion(["x"], max_length=None)}
        )
        assert result.success is True

    async def test_array_union_when_existing_doc_returns_none_initializes_from_empty(self, db_client):
        patched: dict = {}

        async def return_none_on_get(method, path, **kwargs):
            return None

        async def capture_patch(method, path, **kwargs):
            patched.update(kwargs.get("json", {}))

        db_client._request_json = return_none_on_get
        db_client._request_void = capture_patch
        await db_client.update_document("docs", "d1", {"tags": ArrayUnion(["x"], max_length=None)})
        assert patched["tags"] == ["x"]

    async def test_array_remove_on_non_list_field_produces_empty(self, db_client):
        await db_client.create_document("docs", "d1", {"tags": 42})
        result = await db_client.update_document(
            "docs", "d1", {"tags": ArrayRemove(["x"])}
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["tags"] == []


class TestUpdateWithArrayUnionEdgeCases:
    async def test_none_field_initializes_as_list(self, db_client):
        await db_client.create_document("docs", "d1", {"items": None})
        result = await db_client.update_with_array_union(
            collection="docs",
            document_id="d1",
            array_field="items",
            items_to_add=[{"v": 1}],
            additional_updates={},
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["items"] == [{"v": 1}]

    async def test_non_list_field_resets_to_list(self, db_client):
        await db_client.create_document("docs", "d1", {"items": 42})
        result = await db_client.update_with_array_union(
            collection="docs",
            document_id="d1",
            array_field="items",
            items_to_add=[{"v": 1}],
            additional_updates={},
        )
        assert result.success is True

        got = await db_client.get_document("docs", "d1")
        assert got.data["items"] == [{"v": 1}]


class TestErrorPropagation:
    async def test_create_propagates_network_error(self, db_client_http_error):
        with pytest.raises(NetworkError):
            await db_client_http_error.create_document("col", "d1", {"x": 1})

    async def test_get_propagates_network_error(self, db_client_http_error):
        with pytest.raises(NetworkError):
            await db_client_http_error.get_document("col", "d1")

    async def test_update_propagates_network_error(self, db_client_http_error):
        with pytest.raises(NetworkError):
            await db_client_http_error.update_document("col", "d1", {"x": 1})

    async def test_delete_propagates_network_error(self, db_client_http_error):
        with pytest.raises(NetworkError):
            await db_client_http_error.delete_document("col", "d1")

    async def test_query_propagates_network_error(self, db_client_http_error):
        with pytest.raises(NetworkError):
            await db_client_http_error.query_collection("col", field_filters=[], order_by={}, limit=0, select_fields=[])

    async def test_batch_write_propagates_network_error(self, db_client_http_error):
        with pytest.raises(NetworkError):
            await db_client_http_error.batch_write([
                BatchWriteOperation(op_type=BatchWriteOpType.SET, collection="col", doc_id="d1", data={"x": 1})
            ])

    async def test_update_with_array_union_propagates_network_error(self, db_client_http_error):
        with pytest.raises(NetworkError):
            await db_client_http_error.update_with_array_union(
                collection="col",
                document_id="d1",
                array_field="items",
                items_to_add=[{"v": 1}],
                additional_updates={},
            )

    async def test_count_documents_propagates_network_error(self, db_client_http_error):
        with pytest.raises(NetworkError):
            await db_client_http_error.count_documents("col", field_filters=[])
