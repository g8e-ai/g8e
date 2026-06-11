// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docField extracts a typed field value from a Document's Data map.
func docField(t *testing.T, doc *models.Document, field string) interface{} {
	t.Helper()
	raw, ok := doc.Data[field]
	if !ok {
		return nil
	}
	var v interface{}
	require.NoError(t, json.Unmarshal(raw, &v))
	return v
}

// mustDocJSON marshals a map to json.RawMessage for use with DocSet/DocUpdate.
func mustDocJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return json.RawMessage(b)
}

func TestGatewaySchema(t *testing.T) {
	t.Parallel()
	schema := GatewaySchema()
	assert.NotEmpty(t, schema, "GatewaySchema should return non-empty schema")
	assert.Contains(t, schema, "CREATE TABLE", "Schema should contain CREATE TABLE statements")
}

func TestCanonicalDBService_GetDB(t *testing.T) {
	t.Parallel()
	dataDir := tempDir(t)
	secretsDir := tempDir(t)
	logger := testutil.NewTestLogger()

	db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
	require.NoError(t, err)
	defer db.Close()

	assert.NotNil(t, db.GetDB(), "GetDB should return non-nil database")
}

func TestCanonicalDBService_Wait(t *testing.T) {
	t.Parallel()
	dataDir := tempDir(t)
	secretsDir := tempDir(t)
	logger := testutil.NewTestLogger()

	db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
	require.NoError(t, err)

	// Close the database to stop background workers
	db.Close()

	// Wait should complete successfully after Close
	db.Wait()
}

func TestCanonicalDBService_SSEEventsListAllSince(t *testing.T) {
	t.Parallel()
	dataDir := tempDir(t)
	secretsDir := tempDir(t)
	logger := testutil.NewTestLogger()

	db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, "vault"), logger, true, "", false)
	require.NoError(t, err)
	defer db.Close()

	// Append some SSE events
	route := SSERoute{WebSessionID: "test-session"}
	for i := 0; i < 5; i++ {
		err := db.SSEEventsAppend(route, fmt.Sprintf("event-type-%d", i), fmt.Sprintf(`{"data":"%d"}`, i), "test-producer")
		require.NoError(t, err)
	}

	// List all events since ID 0 with limit 100
	rows, err := db.SSEEventsListAllSince(0, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 5, "Should return all 5 events")

	// List events since ID 3 with limit 100
	rows, err = db.SSEEventsListAllSince(3, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 2, "Should return 2 events after ID 3")
}

func newTestDB(t *testing.T) *CanonicalDBService {
	t.Helper()
	dir := tempDir(t)
	secretsDir := tempDir(t)
	db, err := OpenCanonicalDBService(dir, secretsDir, filepath.Join(dir, "vault"), testutil.NewTestLogger(), true, "", false)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// Document Store
// ---------------------------------------------------------------------------

func TestDocSetAndGet(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	err := db.DocStore.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice", "role": "admin"}))
	require.NoError(t, err)

	doc, err := db.DocStore.DocGet("users", "u1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "alice", docField(t, doc, "name"))
	assert.Equal(t, "admin", docField(t, doc, "role"))
	assert.Equal(t, "u1", doc.ID)
	assert.False(t, doc.CreatedAt.IsZero())
	assert.False(t, doc.UpdatedAt.IsZero())
}

func TestDocGetNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	doc, err := db.DocStore.DocGet("users", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestDocUpdate(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	err := db.DocStore.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice", "role": "user"}))
	require.NoError(t, err)

	updated, err := db.DocStore.DocUpdate("users", "u1", mustDocJSON(t, map[string]string{"role": "admin"}))
	require.NoError(t, err)
	assert.Equal(t, "admin", docField(t, updated, "role"))
	assert.Equal(t, "alice", docField(t, updated, "name"))
}

func TestDocUpdateNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	_, err := db.DocStore.DocUpdate("users", "nonexistent", mustDocJSON(t, map[string]string{"role": "admin"}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDocUpdateDeleteField(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	err := db.DocStore.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice", "temp": "remove_me"}))
	require.NoError(t, err)

	updated, err := db.DocStore.DocUpdate("users", "u1", mustDocJSON(t, map[string]json.RawMessage{"temp": nil}))
	require.NoError(t, err)
	_, hasTmp := updated.Data["temp"]
	assert.False(t, hasTmp)
}

func TestDocDelete(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	err := db.DocStore.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice"}))
	require.NoError(t, err)

	deleted, err := db.DocStore.DocDelete("users", "u1")
	require.NoError(t, err)
	assert.True(t, deleted)

	doc, err := db.DocStore.DocGet("users", "u1")
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestDocDeleteNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	deleted, err := db.DocStore.DocDelete("users", "non-existent-id")
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestDocQuery(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.DocStore.DocSet("operators", "op1", mustDocJSON(t, map[string]string{"status": "active", "name": "op-a"})))
	require.NoError(t, db.DocStore.DocSet("operators", "op2", mustDocJSON(t, map[string]string{"status": "offline", "name": "op-b"})))
	require.NoError(t, db.DocStore.DocSet("operators", "op3", mustDocJSON(t, map[string]string{"status": "active", "name": "op-c"})))

	filters := []models.DocFilter{
		{Field: "status", Op: "==", Value: json.RawMessage(`"active"`)},
	}

	results, err := db.DocStore.DocQuery("operators", filters, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestDocQueryWithOrderAndLimit(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.DocStore.DocSet("items", "a", mustDocJSON(t, map[string]int{"priority": 3})))
	require.NoError(t, db.DocStore.DocSet("items", "b", mustDocJSON(t, map[string]int{"priority": 1})))
	require.NoError(t, db.DocStore.DocSet("items", "c", mustDocJSON(t, map[string]int{"priority": 2})))

	results, err := db.DocStore.DocQuery("items", nil, "priority DESC", 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestDocQueryEmptyCollection(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	results, err := db.DocStore.DocQuery("empty_collection", nil, "", 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestDocQueryFilterValueUnmarshaling(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.DocStore.DocSet("things", "t1", mustDocJSON(t, map[string]json.RawMessage{"label": json.RawMessage(`"foo"`), "count": json.RawMessage(`5`)})))
	require.NoError(t, db.DocStore.DocSet("things", "t2", mustDocJSON(t, map[string]json.RawMessage{"label": json.RawMessage(`"bar"`), "count": json.RawMessage(`10`)})))
	require.NoError(t, db.DocStore.DocSet("things", "t3", mustDocJSON(t, map[string]json.RawMessage{"label": json.RawMessage(`"foo"`), "count": json.RawMessage(`20`)})))

	t.Run("string equality", func(t *testing.T) {
		t.Parallel()
		results, err := db.DocStore.DocQuery("things", []models.DocFilter{
			{Field: "label", Op: "==", Value: json.RawMessage(`"foo"`)},
		}, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("numeric greater-than", func(t *testing.T) {
		t.Parallel()
		results, err := db.DocStore.DocQuery("things", []models.DocFilter{
			{Field: "count", Op: ">", Value: json.RawMessage(`7`)},
		}, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("numeric equality", func(t *testing.T) {
		t.Parallel()
		results, err := db.DocStore.DocQuery("things", []models.DocFilter{
			{Field: "count", Op: "==", Value: json.RawMessage(`10`)},
		}, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})
}

// ---------------------------------------------------------------------------
// KV Store
// ---------------------------------------------------------------------------

func TestKVSetAndGet(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	err := db.KVStore.KVSet("session:abc", `{"user":"alice"}`, 0)
	require.NoError(t, err)

	val, found := db.KVStore.KVGet("session:abc")
	require.True(t, found)
	assert.Equal(t, `{"user":"alice"}`, val)
}

func TestKVGetNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	_, found := db.KVStore.KVGet("nonexistent")
	assert.False(t, found)
}

func TestKVSetWithTTL(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	err := db.KVStore.KVSet("temp:key", "value", 1)
	require.NoError(t, err)

	val, found := db.KVStore.KVGet("temp:key")
	assert.True(t, found)
	assert.Equal(t, "value", val)

	// Wait for expiry with polling
	require.Eventually(t, func() bool {
		_, found := db.KVStore.KVGet("temp:key")
		return !found
	}, 2*time.Second, 100*time.Millisecond, "temp key should expire")
}

func TestKVDelete(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("key1", "val1", 0))
	require.NoError(t, db.KVStore.KVDelete("key1"))

	_, found := db.KVStore.KVGet("key1")
	assert.False(t, found)
}

func TestKVDeletePattern(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("cache:user:1", "a", 0))
	require.NoError(t, db.KVStore.KVSet("cache:user:2", "b", 0))
	require.NoError(t, db.KVStore.KVSet("cache:config:1", "c", 0))

	count, err := db.KVStore.KVDeletePattern("cache:user:*")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	_, found := db.KVStore.KVGet("cache:config:1")
	assert.True(t, found)
}

func TestKVKeys(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("session:a", "1", 0))
	require.NoError(t, db.KVStore.KVSet("session:b", "2", 0))
	require.NoError(t, db.KVStore.KVSet("other:c", "3", 0))

	keys, err := db.KVStore.KVKeys("session:*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

func TestKVKeys_SpecialCharacters(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Keys with dots - SQL GLOB treats dots as literal characters
	require.NoError(t, db.KVStore.KVSet("cache.doc", "1", 0))
	require.NoError(t, db.KVStore.KVSet("cache.doc.backup", "2", 0))
	require.NoError(t, db.KVStore.KVSet("cache:txt", "3", 0))

	// Pattern with literal dot should match exactly
	keys, err := db.KVStore.KVKeys("cache.doc")
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "cache.doc", keys[0])

	// Pattern with wildcard after dot should match both
	keys, err = db.KVStore.KVKeys("cache.doc*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Keys with brackets - SQL GLOB treats brackets as character class delimiters
	// To match literal brackets, we can use a pattern that matches the prefix
	require.NoError(t, db.KVStore.KVSet("array.0", "4", 0))
	require.NoError(t, db.KVStore.KVSet("array.1", "5", 0))

	keys, err = db.KVStore.KVKeys("array.*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Keys with plus signs - SQL GLOB treats plus as literal
	require.NoError(t, db.KVStore.KVSet("user+id", "6", 0))
	require.NoError(t, db.KVStore.KVSet("user+name", "7", 0))

	keys, err = db.KVStore.KVKeys("user+*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Keys with dollar signs - SQL GLOB treats dollar as literal
	require.NoError(t, db.KVStore.KVSet("$var1", "8", 0))
	require.NoError(t, db.KVStore.KVSet("$var2", "9", 0))

	keys, err = db.KVStore.KVKeys("$var*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

func TestKVExists(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("exists:key", "val", 0))
	assert.True(t, db.KVStore.KVExists("exists:key"))
	assert.False(t, db.KVStore.KVExists("missing:key"))
}

func TestKVTTL(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("ttl:key", "val", 60))
	ttl := db.KVStore.KVTTL("ttl:key")
	assert.True(t, ttl > 50 && ttl <= 60)

	assert.Equal(t, -2, db.KVStore.KVTTL("nonexistent"))
}

func TestKVExpire(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("exp:key", "val", 0))
	assert.Equal(t, -1, db.KVStore.KVTTL("exp:key"))

	ok := db.KVStore.KVExpire("exp:key", 30)
	assert.True(t, ok)

	ttl := db.KVStore.KVTTL("exp:key")
	assert.True(t, ttl > 0 && ttl <= 30)

	ok = db.KVStore.KVExpire("nonexistent", 30)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// Schema initialization (idempotent)
// ---------------------------------------------------------------------------

func TestSchemaIdempotent(t *testing.T) {
	t.Parallel()
	dir := tempDir(t)
	secretsDir := tempDir(t)

	db1, err := OpenCanonicalDBService(dir, secretsDir, filepath.Join(dir, "vault"), testutil.NewTestLogger(), true, "", false)
	require.NoError(t, err)
	require.NoError(t, db1.DocStore.DocSet("test", "1", mustDocJSON(t, map[string]string{"val": "first"})))
	db1.Close()

	// Re-open same database - schema init should not fail or lose data
	db2, err := OpenCanonicalDBService(dir, secretsDir, filepath.Join(dir, "vault"), testutil.NewTestLogger(), true, "", false)
	require.NoError(t, err)
	t.Cleanup(func() { db2.Close() })

	doc, err := db2.DocStore.DocGet("test", "1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "first", docField(t, doc, "val"))
}

// ---------------------------------------------------------------------------
// Data directory creation
// ---------------------------------------------------------------------------

func TestCreateDataDir(t *testing.T) {
	t.Parallel()
	tmpDir := tempDir(t)
	dir := filepath.Join(tmpDir, "nested", "deep", "data")
	secretsDir := tempDir(t)

	db, err := OpenCanonicalDBService(dir, secretsDir, filepath.Join(dir, "vault"), testutil.NewTestLogger(), true, "", false)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = os.Stat(filepath.Join(dir, "g8e.db"))
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// DocSet upsert behaviour
// ---------------------------------------------------------------------------

func TestDocSet_UpsertReplacesDataAndUpdatesTimestamp(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.DocStore.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice"})))

	doc1, err := db.DocStore.DocGet("users", "u1")
	require.NoError(t, err)
	createdAt1 := doc1.CreatedAt
	updatedAt1 := doc1.UpdatedAt

	// Small delay to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, db.DocStore.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "admin"})))

	doc2, err := db.DocStore.DocGet("users", "u1")
	require.NoError(t, err)

	assert.Equal(t, "admin", docField(t, doc2, "name"))
	assert.True(t, doc2.CreatedAt.Equal(createdAt1), "created_at must not change on upsert")
	assert.True(t, doc2.UpdatedAt.After(updatedAt1), "updated_at must advance on upsert")
}

// ---------------------------------------------------------------------------
// DocUpdate timestamp preservation
// ---------------------------------------------------------------------------

func TestDocUpdate_PreservesCreatedAt(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.DocStore.DocSet("things", "t1", mustDocJSON(t, map[string]string{"x": "original"})))

	doc1, err := db.DocStore.DocGet("things", "t1")
	require.NoError(t, err)
	createdAt := doc1.CreatedAt

	// Small delay to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	doc2, err := db.DocStore.DocUpdate("things", "t1", mustDocJSON(t, map[string]string{"x": "updated"}))
	require.NoError(t, err)

	assert.True(t, doc2.CreatedAt.Equal(createdAt), "created_at must not change on update")
	assert.Equal(t, "updated", docField(t, doc2, "x"))
}

// ---------------------------------------------------------------------------
// DocQuery injection guards
// ---------------------------------------------------------------------------

func TestDocQuery_InvalidFilterFieldReturnsError(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.DocStore.DocSet("items", "i1", mustDocJSON(t, map[string]string{"name": "x"})))

	_, err := db.DocStore.DocQuery("items", []models.DocFilter{
		{Field: "name; DROP TABLE documents--", Op: "==", Value: json.RawMessage(`"x"`)},
	}, "", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter field")
}

func TestDocQuery_InvalidOrderByFieldReturnsError(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.DocStore.DocSet("items", "i1", mustDocJSON(t, map[string]string{"name": "x"})))

	_, err := db.DocStore.DocQuery("items", nil, "name; DROP TABLE documents--", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid orderBy field")
}

func TestDocQuery_UnknownOpIsSkipped(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.DocStore.DocSet("items", "i1", mustDocJSON(t, map[string]string{"name": "x"})))
	require.NoError(t, db.DocStore.DocSet("items", "i2", mustDocJSON(t, map[string]string{"name": "y"})))

	results, err := db.DocStore.DocQuery("items", []models.DocFilter{
		{Field: "name", Op: "LIKE", Value: json.RawMessage(`"x"`)},
	}, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 2, "unknown op must be skipped, returning all docs")
}

// ---------------------------------------------------------------------------
// KVSet overwrite
// ---------------------------------------------------------------------------

func TestKVSet_OverwriteReplacesValue(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("key1", "first", 0))
	require.NoError(t, db.KVStore.KVSet("key1", "second", 0))

	val, found := db.KVStore.KVGet("key1")
	require.True(t, found)
	assert.Equal(t, "second", val)
}

// ---------------------------------------------------------------------------
// KVTTL - no-expiry path
// ---------------------------------------------------------------------------

func TestKVTTL_NoExpiry(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("persistent", "val", 0))
	assert.Equal(t, -1, db.KVStore.KVTTL("persistent"))
}

// ---------------------------------------------------------------------------
// KVScan
// ---------------------------------------------------------------------------

func TestKVScan_BasicScan(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("scan:a", "1", 0))
	require.NoError(t, db.KVStore.KVSet("scan:b", "2", 0))
	require.NoError(t, db.KVStore.KVSet("scan:c", "3", 0))
	require.NoError(t, db.KVStore.KVSet("other:d", "4", 0))

	next, keys, err := db.KVStore.KVScan("scan:*", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, next, "no next page when all results fit")
	assert.Len(t, keys, 3)
}

func TestKVScan_Pagination(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, db.KVStore.KVSet(fmt.Sprintf("page:%d", i), "v", 0))
	}

	next1, page1, err := db.KVStore.KVScan("page:*", 0, 2)
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	assert.Equal(t, 2, next1, "next cursor must be 2 after first page")

	next2, page2, err := db.KVStore.KVScan("page:*", next1, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
	assert.Equal(t, 4, next2)

	next3, page3, err := db.KVStore.KVScan("page:*", next2, 2)
	require.NoError(t, err)
	assert.Len(t, page3, 1)
	assert.Equal(t, 0, next3, "next cursor must be 0 on last page")
}

func TestKVScan_EmptyResult(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	next, keys, err := db.KVStore.KVScan("nothing:*", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, next)
	assert.Empty(t, keys)
}

func TestKVScan_DefaultCountApplied(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, db.KVStore.KVSet(fmt.Sprintf("dc:%d", i), "v", 0))
	}

	_, keys, err := db.KVStore.KVScan("dc:*", 0, 0)
	require.NoError(t, err)
	assert.Len(t, keys, 5, "count=0 must default to 100 and return all 5 keys")
}

func TestKVScan_ExcludesExpiredKeys(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("live:key", "val", 0))
	require.NoError(t, db.KVStore.KVSet("exp:key", "val", 1))

	// Wait for expiry with polling
	require.Eventually(t, func() bool {
		_, keys, err := db.KVStore.KVScan("*", 0, 100)
		require.NoError(t, err)
		for _, k := range keys {
			if k == "exp:key" {
				return false
			}
		}
		return true
	}, 2*time.Second, 100*time.Millisecond, "expired key must not appear in scan results")
}

// ---------------------------------------------------------------------------
// SSE Events
// ---------------------------------------------------------------------------

func TestSSEEventsCount_EmptyTable(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	count, err := db.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSSEEventsAppendAndCount(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.SSEEventsAppend(SSERoute{WebSessionID: "sess-1"}, "TEXT", `{"chunk":"hello"}`, ""))
	require.NoError(t, db.SSEEventsAppend(SSERoute{WebSessionID: "sess-1"}, "TEXT", `{"chunk":"world"}`, ""))
	require.NoError(t, db.SSEEventsAppend(SSERoute{CLISessionID: "sess-2"}, "DONE", `{}`, ""))

	count, err := db.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestSSEEventsWipe_DeletesAllRows(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.SSEEventsAppend(SSERoute{WebSessionID: "sess-1"}, "TEXT", `{"chunk":"a"}`, ""))
	require.NoError(t, db.SSEEventsAppend(SSERoute{CLISessionID: "sess-2"}, "DONE", `{}`, ""))

	deleted, err := db.SSEEventsWipe()
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	count, err := db.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSSEEventsWipe_EmptyTableReturnsZero(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	deleted, err := db.SSEEventsWipe()
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

// ---------------------------------------------------------------------------
// RunTTLCleanup
// ---------------------------------------------------------------------------

func TestRunTTLCleanup_RemovesExpiredKVEntries(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.KVStore.KVSet("ttl:keep", "val", 0))
	require.NoError(t, db.KVStore.KVSet("ttl:expire", "val", 1))

	// Wait for expiry with polling
	require.Eventually(t, func() bool {
		_, found := db.KVStore.KVGet("ttl:expire")
		return !found
	}, 2*time.Second, 100*time.Millisecond, "ttl:expire should expire")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		db.RunTTLCleanup(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	assert.Eventually(t, func() bool {
		_, err := db.KVStore.KVKeys("ttl:*")
		if err != nil {
			return false
		}
		_, found := db.KVStore.KVGet("ttl:expire")
		return !found
	}, 5*time.Second, 100*time.Millisecond)

	_, kept := db.KVStore.KVGet("ttl:keep")
	assert.True(t, kept, "non-expired key must survive cleanup")
}

func TestHasTrustedSigners(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Initially no signers
	has, err := db.SignerStore.HasTrustedSigners()
	require.NoError(t, err)
	assert.False(t, has)

	// Add an enabled signer
	signer := models.TrustedSigner{
		ID:        "test-signer-1",
		PublicKey: "abc123",
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	signerBytes, err := json.Marshal(signer)
	require.NoError(t, err)
	err = db.DocStore.DocSet("trusted_signers", "test-signer-1", signerBytes)
	require.NoError(t, err)

	has, err = db.SignerStore.HasTrustedSigners()
	require.NoError(t, err)
	assert.True(t, has)

	// Add a disabled signer
	disabledSigner := models.TrustedSigner{
		ID:        "test-signer-2",
		PublicKey: "def456",
		AddedAt:   time.Now().UTC(),
		Enabled:   false,
	}
	disabledSignerBytes, err := json.Marshal(disabledSigner)
	require.NoError(t, err)
	err = db.DocStore.DocSet("trusted_signers", "test-signer-2", disabledSignerBytes)
	require.NoError(t, err)

	// Should still have signers (enabled one exists)
	has, err = db.SignerStore.HasTrustedSigners()
	require.NoError(t, err)
	assert.True(t, has)

	// Delete the enabled signer
	_, err = db.DocStore.DocDelete("trusted_signers", "test-signer-1")
	require.NoError(t, err)

	// Now only disabled signer exists - should return false
	has, err = db.SignerStore.HasTrustedSigners()
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHasTrustedSigners_EmptyCollection(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	has, err := db.SignerStore.HasTrustedSigners()
	require.NoError(t, err)
	assert.False(t, has)
}

func TestGetField(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create a document with multiple fields
	doc := map[string]json.RawMessage{
		"name":  json.RawMessage(`"test-doc"`),
		"value": json.RawMessage(`42`),
		"flag":  json.RawMessage(`true`),
	}
	docBytes, err := json.Marshal(doc)
	require.NoError(t, err)
	err = db.DocStore.DocSet("test_collection", "doc1", docBytes)
	require.NoError(t, err)

	// Get existing field
	field, err := db.DocStore.GetField("test_collection", "doc1", "name")
	require.NoError(t, err)
	require.NotNil(t, field)
	assert.Equal(t, "test-doc", field)

	// Get another field
	field, err = db.DocStore.GetField("test_collection", "doc1", "value")
	require.NoError(t, err)
	require.NotNil(t, field)
	assert.Equal(t, float64(42), field) // JSON numbers are unmarshaled as float64

	// Get field from non-existent document
	field, err = db.DocStore.GetField("test_collection", "nonexistent-doc", "name")
	require.Error(t, err)
	assert.Nil(t, field)
}

func TestDocDeleteNamespace(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create multiple documents in a namespace
	for i := 0; i < 5; i++ {
		doc := map[string]string{"id": fmt.Sprintf("doc%d", i)}
		docBytes, err := json.Marshal(doc)
		require.NoError(t, err)
		err = db.DocStore.DocSet("test_namespace", fmt.Sprintf("doc%d", i), docBytes)
		require.NoError(t, err)
	}

	// Create documents in another namespace
	for i := 0; i < 3; i++ {
		doc := map[string]string{"id": fmt.Sprintf("other%d", i)}
		docBytes, err := json.Marshal(doc)
		require.NoError(t, err)
		err = db.DocStore.DocSet("other_namespace", fmt.Sprintf("other%d", i), docBytes)
		require.NoError(t, err)
	}

	// Delete namespace
	deleted, err := db.DocStore.DocDeleteNamespace("test_namespace")
	require.NoError(t, err)
	assert.Equal(t, int64(5), deleted)

	// Verify documents are deleted
	doc, err := db.DocStore.DocGet("test_namespace", "doc0")
	require.NoError(t, err)
	assert.Nil(t, doc)

	// Verify other namespace is untouched
	doc, err = db.DocStore.DocGet("other_namespace", "other0")
	require.NoError(t, err)
	assert.NotNil(t, doc)

	// Delete non-existent namespace
	deleted, err = db.DocStore.DocDeleteNamespace("nonexistent_namespace")
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

func TestDocDeleteNamespace_Empty(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	deleted, err := db.DocStore.DocDeleteNamespace("empty_namespace")
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

func TestDocCreate(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create a new document
	doc := map[string]string{"name": "test-doc"}
	docBytes, err := json.Marshal(doc)
	require.NoError(t, err)
	err = db.DocStore.DocCreate("test_collection", "doc1", docBytes)
	require.NoError(t, err)

	// Verify document was created
	retrievedDoc, err := db.DocStore.DocGet("test_collection", "doc1")
	require.NoError(t, err)
	require.NotNil(t, retrievedDoc)
	assert.Equal(t, "doc1", retrievedDoc.ID)

	// Attempt to create duplicate - should fail
	err = db.DocStore.DocCreate("test_collection", "doc1", docBytes)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestDocCreate_WithSystemFields(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create a document with system fields that should be stripped
	doc := map[string]json.RawMessage{
		"name":       json.RawMessage(`"test-doc"`),
		"id":         json.RawMessage(`"should-be-stripped"`),
		"created_at": json.RawMessage(`"should-be-stripped"`),
		"updated_at": json.RawMessage(`"should-be-stripped"`),
	}
	docBytes, err := json.Marshal(doc)
	require.NoError(t, err)
	err = db.DocStore.DocCreate("test_collection", "doc1", docBytes)
	require.NoError(t, err)

	// Verify system fields were stripped
	retrievedDoc, err := db.DocStore.DocGet("test_collection", "doc1")
	require.NoError(t, err)
	require.NotNil(t, retrievedDoc)
	assert.Equal(t, "doc1", retrievedDoc.ID)
	assert.NotContains(t, retrievedDoc.Data, "id")
	assert.NotContains(t, retrievedDoc.Data, "created_at")
	assert.NotContains(t, retrievedDoc.Data, "updated_at")
}

func TestFinalizeNonce(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create a nonce in the nonces table with expires_at
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	_, err := db.db.Exec("INSERT INTO nonces (nonce, status, expires_at) VALUES (?, 'reserved', ?)", "test123", expiresAt)
	require.NoError(t, err)

	// Finalize the nonce
	err = db.FinalizeNonce("test123")
	require.NoError(t, err)

	// Verify nonce was updated
	var status string
	err = db.db.QueryRow("SELECT status FROM nonces WHERE nonce = ?", "test123").Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "used", status)
}

func TestFinalizeNonce_NonExistent(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Finalize non-existent nonce should not error
	err := db.FinalizeNonce("nonexistent")
	require.NoError(t, err)
}

func TestReleaseNonce(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create a nonce in the nonces table
	expiresAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	_, err := db.db.Exec("INSERT INTO nonces (nonce, status, expires_at) VALUES (?, 'reserved', ?)", "test456", expiresAt)
	require.NoError(t, err)

	// Release the nonce
	err = db.ReleaseNonce("test456")
	require.NoError(t, err)

	// Verify nonce was deleted
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM nonces WHERE nonce = ?", "test456").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestReleaseNonce_NonExistent(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Release non-existent nonce should not error
	err := db.ReleaseNonce("nonexistent")
	require.NoError(t, err)
}

func TestBlobDelete(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create a blob
	blobData := []byte("test blob data")
	err := db.BlobPut("test_namespace", "blob1", blobData, "application/octet-stream", 0)
	require.NoError(t, err)

	// Delete the blob
	deleted, err := db.BlobDelete("test_namespace", "blob1")
	require.NoError(t, err)
	assert.True(t, deleted)

	// Verify blob was deleted
	_, _, found := db.BlobGet("test_namespace", "blob1")
	assert.False(t, found)
}

func TestBlobDelete_NonExistent(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Delete non-existent blob
	deleted, err := db.BlobDelete("test_namespace", "nonexistent")
	require.NoError(t, err)
	assert.False(t, deleted)
}

