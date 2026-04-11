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

import hashlib
import json
from unittest.mock import AsyncMock

import pytest

from app.constants import (
    CACHE_TTL_DEFAULT,
    CACHE_TTL_MEDIUM,
    CACHE_TTL_SHORT,
    DB_COLLECTION_INVESTIGATIONS,
    DB_COLLECTION_OPERATORS,
    DB_COLLECTION_USERS,
    BatchWriteOpType,
    ComponentName,
    KVKey,
    KVKeyPrefix,
    OperatorStatus,
)
from app.errors import DatabaseError, ValidationError
from app.models.cache import (
    BatchWriteOperation,
    CacheOperationResult,
    DocumentResult,
)
from app.services.cache.cache_aside import CacheAsideService
from app.models.operators import OperatorDocument, OperatorSystemInfo
from tests.fakes.fake_vsodb_clients import FakeDBClient, FakeKVClient

pytestmark = pytest.mark.unit


class TestCacheAsideService:

    @pytest.fixture
    def mock_kv_cache_client(self):
        return FakeKVClient()

    @pytest.fixture
    def mock_db_client(self):
        return FakeDBClient()

    @pytest.fixture
    def service(self, mock_kv_cache_client, mock_db_client):
        from app.db.kv_service import KVService
        from app.db.db_service import DBService
        return CacheAsideService(
            kv=KVService(mock_kv_cache_client),
            db=DBService(mock_db_client),
            component_name=ComponentName.G8EE,
            default_ttl=CACHE_TTL_DEFAULT,
        )

    def test_make_key_format(self, service):
        key = service._make_key("users", "user-123")
        assert key == KVKey.doc("users", "user-123")

    def test_get_ttl_known_collection(self, service):
        assert service._get_ttl_for_collection(DB_COLLECTION_USERS) == CACHE_TTL_DEFAULT

    def test_get_ttl_unknown_collection_returns_default(self, service):
        assert service._get_ttl_for_collection("unknown_collection") == CACHE_TTL_DEFAULT

    def test_get_ttl_investigations(self, service):
        assert service._get_ttl_for_collection(DB_COLLECTION_INVESTIGATIONS) == CACHE_TTL_MEDIUM

    async def test_create_document_success(self, service, mock_kv_cache_client, mock_db_client):
        data = {"id": "user-1", "name": "Alice"}
        # Mock DB check for existence (returns not found)
        mock_db_client.get_document.return_value = DocumentResult(success=True, data=None)
        
        result = await service.create_document(DB_COLLECTION_USERS, "user-1", data)

        assert result.success is True
        assert result.document_id == "user-1"
        assert result.cache_invalidated is True
        mock_db_client.create_document.assert_called_once_with(
            collection=DB_COLLECTION_USERS,
            data=data,
            document_id="user-1",
        )

    async def test_create_document_invalidates_kv_cache(self, service, mock_kv_cache_client, mock_db_client):
        data = {"id": "user-2", "name": "Admin"}
        mock_db_client.get_document.return_value = DocumentResult(success=True, data=None)
        
        key = service._make_key(DB_COLLECTION_USERS, "user-2")
        mock_kv_cache_client.seed_json(key, {"old": "data"})
        
        await service.create_document(DB_COLLECTION_USERS, "user-2", data)

        cached = await mock_kv_cache_client.get_json(key)
        assert cached is None

    async def test_create_document_fails_if_exists(self, service, mock_db_client):
        data = {"id": "user-3"}
        # Ensure the side_effect is cleared so it uses the mock value
        mock_db_client.get_document.side_effect = None
        mock_db_client.get_document.return_value = DocumentResult(success=True, data={"id": "user-3"})
        
        with pytest.raises(DatabaseError, match="already exists"):
            await service.create_document(DB_COLLECTION_USERS, "user-3", data)

    async def test_create_document_db_failure_raises_database_error(self, service, mock_db_client):
        mock_db_client.get_document.return_value = DocumentResult(success=True, data=None)
        mock_db_client.create_document.side_effect = None
        mock_db_client.create_document.return_value = CacheOperationResult(
            success=False, error="write failed"
        )
        with pytest.raises(DatabaseError):
            await service.create_document(DB_COLLECTION_USERS, "user-x", {"id": "user-x"})

    async def test_create_document_db_exception_propagates(self, service, mock_db_client):
        mock_db_client.get_document.return_value = DocumentResult(success=True, data=None)
        mock_db_client.create_document.side_effect = Exception("connection lost")
        with pytest.raises(Exception, match="connection lost"):
            await service.create_document(DB_COLLECTION_USERS, "user-x", {"id": "user-x"})

    async def test_create_document_kv_failure_still_succeeds(self, service, mock_kv_cache_client, mock_db_client):
        mock_db_client.get_document.return_value = DocumentResult(success=True, data=None)
        mock_kv_cache_client.delete = AsyncMock(return_value=0)
        data = {"id": "user-4"}
        result = await service.create_document(DB_COLLECTION_USERS, "user-4", data)
        assert result.success is True
        assert result.cache_invalidated is True

    async def test_update_document_success_invalidates_cache(self, service, mock_kv_cache_client, mock_db_client):
        key = service._make_key(DB_COLLECTION_USERS, "user-5")
        mock_kv_cache_client.seed_json(key, {"id": "user-5", "name": "old"})

        result = await service.update_document(DB_COLLECTION_USERS, "user-5", {"name": "new"})

        assert result.success is True
        assert result.cache_invalidated is True
        cached = await mock_kv_cache_client.get_json(key)
        assert cached is None

    async def test_update_document_db_failure_raises_database_error(self, service, mock_db_client):
        mock_db_client.update_document.side_effect = None
        mock_db_client.update_document.return_value = CacheOperationResult(
            success=False, error="update failed"
        )
        with pytest.raises(DatabaseError):
            await service.update_document(DB_COLLECTION_USERS, "user-x", {"name": "new"})

    async def test_update_document_passes_merge_flag(self, service, mock_db_client):
        await service.update_document(DB_COLLECTION_USERS, "user-6", {"x": 1}, merge=False)
        mock_db_client.update_document.assert_called_once_with(
            collection=DB_COLLECTION_USERS,
            document_id="user-6",
            data={"x": 1},
            merge=False,
        )

    async def test_delete_document_success_removes_from_cache(self, service, mock_kv_cache_client, mock_db_client):
        key = service._make_key(DB_COLLECTION_USERS, "user-7")
        mock_kv_cache_client.seed_json(key, {"id": "user-7"})

        result = await service.delete_document(DB_COLLECTION_USERS, "user-7")

        assert result.success is True
        cached = await mock_kv_cache_client.get_json(key)
        assert cached is None

    async def test_delete_document_db_failure_raises_database_error(self, service, mock_db_client):
        mock_db_client.delete_document.side_effect = None
        mock_db_client.delete_document.return_value = CacheOperationResult(
            success=False, error="delete failed"
        )
        with pytest.raises(DatabaseError):
            await service.delete_document(DB_COLLECTION_USERS, "user-x")

    async def test_get_document_cache_hit(self, service, mock_kv_cache_client, mock_db_client):
        key = service._make_key(DB_COLLECTION_USERS, "user-8")
        cached_data = {"id": "user-8", "name": "Cached"}
        mock_kv_cache_client.seed_json(key, cached_data)

        result = await service.get_document(DB_COLLECTION_USERS, "user-8")

        assert result == cached_data
        mock_db_client.get_document.assert_not_called()

    async def test_get_document_cache_miss_reads_db_and_populates_cache(self, service, mock_kv_cache_client, mock_db_client):
        db_data = {"name": "From DB"}
        mock_db_client.get_document.side_effect = None
        mock_db_client.get_document.return_value = DocumentResult(
            success=True,
            data={**db_data, "id": "user-9"},
        )

        result = await service.get_document(DB_COLLECTION_USERS, "user-9")

        assert result is not None
        assert result["name"] == "From DB"
        assert result["id"] == "user-9"

        key = service._make_key(DB_COLLECTION_USERS, "user-9")
        cached = await mock_kv_cache_client.get_json(key)
        assert cached is not None
        assert cached["id"] == "user-9"

    async def test_get_document_db_returns_not_found_returns_none(self, service, mock_db_client):
        mock_db_client.get_document.return_value = DocumentResult(success=False, data=None)
        result = await service.get_document(DB_COLLECTION_USERS, "nonexistent")
        assert result is None

    async def test_get_document_does_not_swallow_db_exception(self, service, mock_db_client):
        mock_db_client.get_document.side_effect = Exception("db timeout")
        with pytest.raises(Exception, match="db timeout"):
            await service.get_document(DB_COLLECTION_USERS, "user-x")

    async def test_get_document_serializes_datetime_fields_before_kv_write(self, service, mock_kv_cache_client, mock_db_client):
        from datetime import UTC, datetime
        ts = datetime(2025, 6, 1, 12, 0, 0, tzinfo=UTC)
        mock_db_client.get_document.side_effect = None
        mock_db_client.get_document.return_value = DocumentResult(
            success=True,
            data={"id": "user-dt", "name": "Alice", "created_at": ts, "updated_at": ts},
        )

        result = await service.get_document(DB_COLLECTION_USERS, "user-dt")

        assert result is not None
        assert result["created_at"] == "2025-06-01T12:00:00Z"
        assert result["updated_at"] == "2025-06-01T12:00:00Z"

        key = service._make_key(DB_COLLECTION_USERS, "user-dt")
        cached = await mock_kv_cache_client.get_json(key)
        assert cached is not None
        assert cached["created_at"] == "2025-06-01T12:00:00Z"

    async def test_get_query_result_cache_hit(self, service, mock_kv_cache_client):
        query_params = {"filters": [{"field": "status", "op": "==", "value": OperatorStatus.ACTIVE}]}
        query_str = json.dumps(query_params, sort_keys=True)
        filter_hash = hashlib.md5(query_str.encode()).hexdigest()
        key = KVKey.query(DB_COLLECTION_USERS, filter_hash)

        cached_results = [{"id": "user-10"}, {"id": "user-11"}]
        mock_kv_cache_client.seed_json(key, cached_results)

        result = await service.get_query_result(DB_COLLECTION_USERS, query_params)
        assert result == cached_results

    async def test_get_query_result_cache_miss_returns_none(self, service):
        result = await service.get_query_result(DB_COLLECTION_USERS, {"filters": []})
        assert result is None

    async def test_get_query_result_uses_ttl_query_cache_default(self, service, mock_kv_cache_client):
        query_params = {"q": "test"}
        query_str = json.dumps(query_params, sort_keys=True)
        filter_hash = hashlib.md5(query_str.encode()).hexdigest()
        key = KVKey.query(DB_COLLECTION_USERS, filter_hash)
        mock_kv_cache_client.seed_json(key, [{"id": "r1"}])

        result = await service.get_query_result(DB_COLLECTION_USERS, query_params)
        assert result is not None

    async def test_set_query_result_stores_in_cache(self, service, mock_kv_cache_client):
        query_params = {"filters": []}
        results = [{"id": "user-12"}]
        success = await service.set_query_result(DB_COLLECTION_USERS, query_params, results)

        assert success is True

        query_str = json.dumps(query_params, sort_keys=True)
        filter_hash = hashlib.md5(query_str.encode()).hexdigest()
        key = KVKey.query(DB_COLLECTION_USERS, filter_hash)
        cached = await mock_kv_cache_client.get_json(key)
        assert cached == results

    async def test_set_query_result_uses_ttl_query_cache_default(self, service, mock_kv_cache_client):
        results = [{"id": "r1"}]
        await service.set_query_result(DB_COLLECTION_USERS, {"x": 1}, results)
        call_kwargs = mock_kv_cache_client.set_json.call_args[1]
        assert call_kwargs["ex"] == CACHE_TTL_SHORT

    async def test_set_query_result_uses_custom_ttl(self, service, mock_kv_cache_client):
        await service.set_query_result(DB_COLLECTION_USERS, {"x": 2}, [{"id": "r"}], ttl=60)
        call_kwargs = mock_kv_cache_client.set_json.call_args[1]
        assert call_kwargs["ex"] == 60

    async def test_invalidate_document_removes_key(self, service, mock_kv_cache_client):
        key = service._make_key(DB_COLLECTION_USERS, "user-13")
        mock_kv_cache_client.seed_json(key, {"id": "user-13"})

        result = await service.invalidate_document(DB_COLLECTION_USERS, "user-13")

        assert result is True
        cached = await mock_kv_cache_client.get_json(key)
        assert cached is None

    async def test_invalidate_document_returns_false_if_not_cached(self, service):
        result = await service.invalidate_document(DB_COLLECTION_USERS, "nonexistent")
        assert result is False

    async def test_invalidate_document_exception_propagates(self, service, mock_kv_cache_client):
        mock_kv_cache_client.delete = AsyncMock(side_effect=Exception("kv error"))
        with pytest.raises(Exception, match="kv error"):
            await service.invalidate_document(DB_COLLECTION_USERS, "user-x")

    async def test_invalidate_query_cache_delegates_to_delete_pattern(self, service, mock_kv_cache_client):
        mock_kv_cache_client.delete_pattern = AsyncMock(return_value=3)
        deleted = await service.invalidate_query_cache(DB_COLLECTION_USERS)

        assert deleted == 3
        mock_kv_cache_client.delete_pattern.assert_called_once_with(
            f"{KVKeyPrefix.CACHE_QUERY}{DB_COLLECTION_USERS}:*"
        )

    async def test_invalidate_query_cache_exception_propagates(self, service, mock_kv_cache_client):
        mock_kv_cache_client.delete_pattern = AsyncMock(side_effect=Exception("kv error"))
        with pytest.raises(Exception, match="kv error"):
            await service.invalidate_query_cache(DB_COLLECTION_USERS)

    async def test_batch_create_documents_empty_returns_zero(self, service):
        result = await service.batch_create_documents([])
        assert result.success is True
        assert result.count == 0

    async def test_batch_create_documents_success(self, service, mock_kv_cache_client, mock_db_client):
        from app.models.cache import BatchCreateDocumentOperation
        operations = [
            BatchCreateDocumentOperation(collection=DB_COLLECTION_USERS, document_id="u1", data={"id": "u1"}),
            BatchCreateDocumentOperation(collection=DB_COLLECTION_USERS, document_id="u2", data={"id": "u2"}),
        ]

        # Seed cache to verify invalidation
        for op in operations:
            key = service._make_key(op.collection, op.document_id)
            mock_kv_cache_client.seed_json(key, {"old": "data"})

        result = await service.batch_create_documents(operations)

        assert result.success is True
        assert result.count == 2

        for op in operations:
            key = service._make_key(op.collection, op.document_id)
            cached = await mock_kv_cache_client.get_json(key)
            assert cached is None

    async def test_batch_create_documents_db_failure_raises_database_error(self, service, mock_db_client):
        from app.models.cache import BatchCreateDocumentOperation, BatchOperationResult
        mock_db_client.batch_write.side_effect = None
        mock_db_client.batch_write.return_value = BatchOperationResult(
            success=False, error="batch write failed"
        )
        operations = [
            BatchCreateDocumentOperation(collection=DB_COLLECTION_USERS, document_id="u1", data={"id": "u1"}),
        ]
        with pytest.raises(DatabaseError):
            await service.batch_create_documents(operations)

    async def test_batch_create_documents_db_exception_propagates(self, service, mock_db_client):
        from app.models.cache import BatchCreateDocumentOperation
        mock_db_client.batch_write.side_effect = Exception("batch error")
        operations = [
            BatchCreateDocumentOperation(collection=DB_COLLECTION_USERS, document_id="u1", data={"id": "u1"}),
        ]
        with pytest.raises(Exception, match="batch error"):
            await service.batch_create_documents(operations)

    async def test_batch_create_documents_formats_db_operations_correctly(self, service, mock_db_client):
        from app.models.cache import BatchCreateDocumentOperation
        operations = [
            BatchCreateDocumentOperation(collection=DB_COLLECTION_USERS, document_id="u3", data={"id": "u3"}),
        ]
        await service.batch_create_documents(operations)

        call_args = mock_db_client.batch_write.call_args[0][0]
        op = call_args[0]
        assert isinstance(op, BatchWriteOperation)
        assert op.op_type == BatchWriteOpType.SET
        assert op.collection == DB_COLLECTION_USERS
        assert op.doc_id == "u3"
        assert op.merge is False

    def test_component_name_defaults_to_g8ee_enum(self, mock_kv_cache_client, mock_db_client):
        from app.db.kv_service import KVService
        from app.db.db_service import DBService
        svc = CacheAsideService(kv=KVService(mock_kv_cache_client), db=DBService(mock_db_client))
        assert svc.component_name == ComponentName.G8EE

    def test_component_name_accepts_enum(self, mock_kv_cache_client, mock_db_client):
        from app.db.kv_service import KVService
        from app.db.db_service import DBService
        svc = CacheAsideService(
            kv=KVService(mock_kv_cache_client),
            db=DBService(mock_db_client),
            component_name=ComponentName.VSA,
        )
        assert svc.component_name == ComponentName.VSA

    async def test_cache_document_writes_to_kv(self, service, mock_kv_cache_client):
        data = {"id": "user-20", "name": "Direct"}
        result = await service.cache_document(DB_COLLECTION_USERS, "user-20", data)

        assert result is True
        key = service._make_key(DB_COLLECTION_USERS, "user-20")
        cached = await mock_kv_cache_client.get_json(key)
        assert cached == data

    async def test_cache_document_uses_collection_ttl(self, service, mock_kv_cache_client):
        await service.cache_document(DB_COLLECTION_USERS, "user-21", {"id": "user-21"})
        call_kwargs = mock_kv_cache_client.set_json.call_args[1]
        assert call_kwargs["ex"] == CACHE_TTL_DEFAULT

    async def test_cache_document_uses_custom_ttl(self, service, mock_kv_cache_client):
        await service.cache_document(DB_COLLECTION_USERS, "user-22", {"id": "user-22"}, ttl=120)
        call_kwargs = mock_kv_cache_client.set_json.call_args[1]
        assert call_kwargs["ex"] == 120

    async def test_invalidate_collection_delegates_to_delete_pattern(self, service, mock_kv_cache_client):
        mock_kv_cache_client.delete_pattern = AsyncMock(return_value=5)
        deleted = await service.invalidate_collection(DB_COLLECTION_USERS)

        assert deleted == 5
        mock_kv_cache_client.delete_pattern.assert_called_once_with(
            f"{KVKeyPrefix.CACHE_DOC}{DB_COLLECTION_USERS}:*"
        )

    async def test_clear_all_deletes_doc_and_query_keys(self, service, mock_kv_cache_client):
        call_counts = {"doc": 0, "query": 0}

        async def delete_pattern_side_effect(pattern):
            if KVKeyPrefix.CACHE_DOC in pattern and KVKeyPrefix.CACHE_QUERY not in pattern:
                call_counts["doc"] += 1
                return 4
            call_counts["query"] += 1
            return 2

        mock_kv_cache_client.delete_pattern = AsyncMock(side_effect=delete_pattern_side_effect)
        total = await service.clear_all()

        assert total == 6
        assert call_counts["doc"] == 1
        assert call_counts["query"] == 1

    async def test_get_stats_healthy(self, service, mock_kv_cache_client):
        doc_key = service._make_key(DB_COLLECTION_USERS, "user-stat")
        mock_kv_cache_client.seed_json(doc_key, {"id": "user-stat"})

        stats = await service.get_stats()

        assert stats.enabled is True
        assert stats.healthy is True
        assert stats.document_keys >= 1
        assert stats.default_ttl == CACHE_TTL_DEFAULT
        assert stats.total_keys == stats.document_keys + stats.query_keys

    async def test_get_stats_kv_exception_returns_unhealthy(self, service, mock_kv_cache_client):
        mock_kv_cache_client.keys = AsyncMock(side_effect=Exception("kv down"))

        stats = await service.get_stats()

        assert stats.enabled is True
        assert stats.healthy is False
        assert stats.error is not None
