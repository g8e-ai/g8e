# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for DocumentService.

DocumentService is the single g8ee owner of Gateway document access. It keeps no
cache: every read goes to the client, and every failed write raises DatabaseError.
"""

import pytest

from app.constants import BatchWriteOpType
from app.db.document_service import DocumentService
from app.errors import DatabaseError
from app.models.base import G8eBaseModel
from app.models.cache import (
    BatchCreateDocumentOperation,
    BatchWriteOperation,
    CacheOperationResult,
    FieldFilter,
)
from tests.fakes.fake_operator_clients import FakeDBClient

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


class _Sample(G8eBaseModel):
    name: str
    count: int


@pytest.fixture
def client() -> FakeDBClient:
    return FakeDBClient()


@pytest.fixture
def service(client: FakeDBClient) -> DocumentService:
    return DocumentService(client)


def _fail(client: FakeDBClient, method: str, error: str = "boom") -> None:
    getattr(client, method).side_effect = None
    getattr(client, method).return_value = CacheOperationResult(success=False, error=error)


class TestCreate:
    async def test_creates_and_reads_back(self, service):
        result = await service.create_document("col", "d1", {"a": 1})

        assert result.success
        assert result.document_id == "d1"
        assert await service.get_document_data("col", "d1") == {"a": 1}

    async def test_model_is_dumped_to_json_mode(self, service, client):
        await service.create_document("col", "d1", _Sample(name="x", count=2))

        assert client.create_document.call_args.kwargs["data"] == {"name": "x", "count": 2}

    async def test_existing_document_raises_without_writing(self, service, client):
        await service.create_document("col", "d1", {"a": 1})
        client.create_document.reset_mock()

        with pytest.raises(DatabaseError, match="already exists"):
            await service.create_document("col", "d1", {"a": 2})

        client.create_document.assert_not_called()
        assert await service.get_document_data("col", "d1") == {"a": 1}

    async def test_failed_write_raises(self, service, client):
        _fail(client, "create_document")

        with pytest.raises(DatabaseError, match="boom"):
            await service.create_document("col", "d1", {"a": 1})


class TestUpdate:
    async def test_merge_update(self, service):
        await service.create_document("col", "d1", {"a": 1, "b": 2})

        await service.update_document("col", "d1", {"b": 3})

        assert await service.get_document_data("col", "d1") == {"a": 1, "b": 3}

    async def test_model_is_dumped_to_json_mode(self, service, client):
        await service.update_document("col", "d1", _Sample(name="x", count=2), merge=False)

        assert client.update_document.call_args.kwargs["data"] == {"name": "x", "count": 2}
        assert client.update_document.call_args.kwargs["merge"] is False

    async def test_failed_write_raises(self, service, client):
        _fail(client, "update_document")

        with pytest.raises(DatabaseError, match="boom"):
            await service.update_document("col", "d1", {"a": 1})


class TestDelete:
    async def test_delete(self, service):
        await service.create_document("col", "d1", {"a": 1})

        await service.delete_document("col", "d1")

        assert await service.get_document_data("col", "d1") is None

    async def test_failed_delete_raises(self, service, client):
        _fail(client, "delete_document")

        with pytest.raises(DatabaseError, match="boom"):
            await service.delete_document("col", "d1")


class TestReads:
    async def test_every_read_hits_the_client(self, service, client):
        await service.create_document("col", "d1", {"a": 1})
        client.get_document.reset_mock()

        await service.get_document_data("col", "d1")
        await service.get_document_data("col", "d1")

        assert client.get_document.call_count == 2

    async def test_a_write_is_visible_to_the_next_read(self, service):
        await service.create_document("col", "d1", {"a": 1})
        assert await service.get_document_data("col", "d1") == {"a": 1}

        await service.update_document("col", "d1", {"a": 2})

        assert await service.get_document_data("col", "d1") == {"a": 2}

    async def test_missing_document_is_none(self, service):
        assert await service.get_document_data("col", "missing") is None

    async def test_get_document_returns_client_result(self, service):
        await service.create_document("col", "d1", {"a": 1})

        result = await service.get_document("col", "d1")

        assert result.success
        assert result.data == {"a": 1}


class TestQuery:
    async def test_query_collection_dumps_filters(self, service, client):
        await service.create_document("col", "d1", {"status": "active"})
        await service.create_document("col", "d2", {"status": "inactive"})

        result = await service.query_collection(
            "col",
            field_filters=[FieldFilter(field="status", op="==", value="active")],
            order_by={},
            limit=10,
        )

        assert [d["status"] for d in result.data] == ["active"]
        sent = client.query_collection.call_args.kwargs
        assert sent["field_filters"] == [{"field": "status", "op": "==", "value": "active"}]
        assert sent["select_fields"] == []

    async def test_query_documents_returns_rows(self, service):
        await service.create_document("col", "d1", {"status": "active"})

        rows = await service.query_documents(
            "col", [{"field": "status", "op": "==", "value": "active"}]
        )

        assert rows == [{"status": "active"}]


class TestArrayAndBatch:
    async def test_array_append(self, service):
        await service.create_document("col", "d1", {"items": [1]})

        await service.append_to_array("col", "d1", "items", [2], {})

        assert (await service.get_document_data("col", "d1"))["items"] == [1, 2]

    async def test_array_append_failure_raises(self, service, client):
        _fail(client, "update_with_array_union")

        with pytest.raises(DatabaseError, match="boom"):
            await service.append_to_array("col", "d1", "items", [2], {})

    async def test_batch_create_empty_is_a_noop(self, service, client):
        result = await service.batch_create_documents([])

        assert result.success
        assert result.count == 0
        client.batch_write.assert_not_called()

    async def test_batch_create_issues_one_set_per_document(self, service, client):
        result = await service.batch_create_documents(
            [
                BatchCreateDocumentOperation(collection="col", document_id="d1", data={"a": 1}),
                BatchCreateDocumentOperation(collection="col", document_id="d2", data={"a": 2}),
            ]
        )

        assert result.count == 2
        ops: list[BatchWriteOperation] = client.batch_write.call_args.args[0]
        assert [(o.op_type, o.doc_id, o.merge) for o in ops] == [
            (BatchWriteOpType.SET, "d1", False),
            (BatchWriteOpType.SET, "d2", False),
        ]

    async def test_batch_failure_raises(self, service, client):
        _fail(client, "batch_write")

        with pytest.raises(DatabaseError, match="boom"):
            await service.batch_write([])


class TestClose:
    async def test_close_swallows_client_errors(self, service, client):
        client.close.side_effect = RuntimeError("already closed")

        await service.close()

        client.close.assert_awaited_once()
