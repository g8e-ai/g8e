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

package listen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
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

func newTestDB(t *testing.T) *ListenDBService {
	t.Helper()
	dir := t.TempDir()
	sslDir := t.TempDir()
	db, err := NewListenDBService(dir, sslDir, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// ---------------------------------------------------------------------------
// Document Store
// ---------------------------------------------------------------------------

func TestDocSetAndGet(t *testing.T) {
	db := newTestDB(t)

	err := db.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice", "role": "admin"}))
	require.NoError(t, err)

	doc, err := db.DocGet("users", "u1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "alice", docField(t, doc, "name"))
	assert.Equal(t, "admin", docField(t, doc, "role"))
	assert.Equal(t, "u1", doc.ID)
	assert.False(t, doc.CreatedAt.IsZero())
	assert.False(t, doc.UpdatedAt.IsZero())
}

func TestDocGetNotFound(t *testing.T) {
	db := newTestDB(t)

	doc, err := db.DocGet("users", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestDocUpdate(t *testing.T) {
	db := newTestDB(t)

	err := db.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice", "role": "user"}))
	require.NoError(t, err)

	updated, err := db.DocUpdate("users", "u1", mustDocJSON(t, map[string]string{"role": "admin"}))
	require.NoError(t, err)
	assert.Equal(t, "admin", docField(t, updated, "role"))
	assert.Equal(t, "alice", docField(t, updated, "name"))
}

func TestDocUpdateNotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := db.DocUpdate("users", "nonexistent", mustDocJSON(t, map[string]string{"role": "admin"}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDocUpdateDeleteField(t *testing.T) {
	db := newTestDB(t)

	err := db.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice", "temp": "remove_me"}))
	require.NoError(t, err)

	updated, err := db.DocUpdate("users", "u1", mustDocJSON(t, map[string]interface{}{"temp": nil}))
	require.NoError(t, err)
	_, hasTmp := updated.Data["temp"]
	assert.False(t, hasTmp)
}

func TestDocDelete(t *testing.T) {
	db := newTestDB(t)

	err := db.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice"}))
	require.NoError(t, err)

	deleted, err := db.DocDelete("users", "u1")
	require.NoError(t, err)
	assert.True(t, deleted)

	doc, err := db.DocGet("users", "u1")
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestDocDeleteNotFound(t *testing.T) {
	db := newTestDB(t)

	deleted, err := db.DocDelete("users", "non-existent-id")
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestDocQuery(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.DocSet("operators", "op1", mustDocJSON(t, map[string]string{"status": "active", "name": "op-a"})))
	require.NoError(t, db.DocSet("operators", "op2", mustDocJSON(t, map[string]string{"status": "offline", "name": "op-b"})))
	require.NoError(t, db.DocSet("operators", "op3", mustDocJSON(t, map[string]string{"status": "active", "name": "op-c"})))

	filters := []models.DocFilter{
		{Field: "status", Op: "==", Value: json.RawMessage(`"active"`)},
	}

	results, err := db.DocQuery("operators", filters, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestDocQueryWithOrderAndLimit(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.DocSet("items", "a", mustDocJSON(t, map[string]int{"priority": 3})))
	require.NoError(t, db.DocSet("items", "b", mustDocJSON(t, map[string]int{"priority": 1})))
	require.NoError(t, db.DocSet("items", "c", mustDocJSON(t, map[string]int{"priority": 2})))

	results, err := db.DocQuery("items", nil, "priority DESC", 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestDocQueryEmptyCollection(t *testing.T) {
	db := newTestDB(t)

	results, err := db.DocQuery("empty_collection", nil, "", 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestDocQueryFilterValueUnmarshaling(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.DocSet("things", "t1", mustDocJSON(t, map[string]interface{}{"label": "foo", "count": 5})))
	require.NoError(t, db.DocSet("things", "t2", mustDocJSON(t, map[string]interface{}{"label": "bar", "count": 10})))
	require.NoError(t, db.DocSet("things", "t3", mustDocJSON(t, map[string]interface{}{"label": "foo", "count": 20})))

	t.Run("string equality", func(t *testing.T) {
		results, err := db.DocQuery("things", []models.DocFilter{
			{Field: "label", Op: "==", Value: json.RawMessage(`"foo"`)},
		}, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("numeric greater-than", func(t *testing.T) {
		results, err := db.DocQuery("things", []models.DocFilter{
			{Field: "count", Op: ">", Value: json.RawMessage(`7`)},
		}, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("numeric equality", func(t *testing.T) {
		results, err := db.DocQuery("things", []models.DocFilter{
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
	db := newTestDB(t)

	err := db.KVSet("session:abc", `{"user":"alice"}`, 0)
	require.NoError(t, err)

	val, found := db.KVGet("session:abc")
	assert.True(t, found)
	assert.Equal(t, `{"user":"alice"}`, val)
}

func TestKVGetNotFound(t *testing.T) {
	db := newTestDB(t)

	_, found := db.KVGet("nonexistent")
	assert.False(t, found)
}

func TestKVSetWithTTL(t *testing.T) {
	db := newTestDB(t)

	err := db.KVSet("temp:key", "value", 1)
	require.NoError(t, err)

	val, found := db.KVGet("temp:key")
	assert.True(t, found)
	assert.Equal(t, "value", val)

	// Wait for expiry
	time.Sleep(1100 * time.Millisecond)

	_, found = db.KVGet("temp:key")
	assert.False(t, found)
}

func TestKVDelete(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("key1", "val1", 0))
	require.NoError(t, db.KVDelete("key1"))

	_, found := db.KVGet("key1")
	assert.False(t, found)
}

func TestKVDeletePattern(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("cache:user:1", "a", 0))
	require.NoError(t, db.KVSet("cache:user:2", "b", 0))
	require.NoError(t, db.KVSet("cache:config:1", "c", 0))

	count, err := db.KVDeletePattern("cache:user:*")
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	_, found := db.KVGet("cache:config:1")
	assert.True(t, found)
}

func TestKVKeys(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("session:a", "1", 0))
	require.NoError(t, db.KVSet("session:b", "2", 0))
	require.NoError(t, db.KVSet("other:c", "3", 0))

	keys, err := db.KVKeys("session:*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

func TestKVKeys_SpecialCharacters(t *testing.T) {
	db := newTestDB(t)

	// Keys with dots - SQL GLOB treats dots as literal characters
	require.NoError(t, db.KVSet("cache.doc", "1", 0))
	require.NoError(t, db.KVSet("cache.doc.backup", "2", 0))
	require.NoError(t, db.KVSet("cache:txt", "3", 0))

	// Pattern with literal dot should match exactly
	keys, err := db.KVKeys("cache.doc")
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "cache.doc", keys[0])

	// Pattern with wildcard after dot should match both
	keys, err = db.KVKeys("cache.doc*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Keys with brackets - SQL GLOB treats brackets as character class delimiters
	// To match literal brackets, we can use a pattern that matches the prefix
	require.NoError(t, db.KVSet("array.0", "4", 0))
	require.NoError(t, db.KVSet("array.1", "5", 0))

	keys, err = db.KVKeys("array.*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Keys with plus signs - SQL GLOB treats plus as literal
	require.NoError(t, db.KVSet("user+id", "6", 0))
	require.NoError(t, db.KVSet("user+name", "7", 0))

	keys, err = db.KVKeys("user+*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	// Keys with dollar signs - SQL GLOB treats dollar as literal
	require.NoError(t, db.KVSet("$var1", "8", 0))
	require.NoError(t, db.KVSet("$var2", "9", 0))

	keys, err = db.KVKeys("$var*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
}

func TestKVExists(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("exists:key", "val", 0))
	assert.True(t, db.KVExists("exists:key"))
	assert.False(t, db.KVExists("missing:key"))
}

func TestKVTTL(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("ttl:key", "val", 60))
	ttl := db.KVTTL("ttl:key")
	assert.True(t, ttl > 50 && ttl <= 60)

	assert.Equal(t, -2, db.KVTTL("nonexistent"))
}

func TestKVExpire(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("exp:key", "val", 0))
	assert.Equal(t, -1, db.KVTTL("exp:key"))

	ok := db.KVExpire("exp:key", 30)
	assert.True(t, ok)

	ttl := db.KVTTL("exp:key")
	assert.True(t, ttl > 0 && ttl <= 30)

	ok = db.KVExpire("nonexistent", 30)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// Schema initialization (idempotent)
// ---------------------------------------------------------------------------

func TestSchemaIdempotent(t *testing.T) {
	dir := t.TempDir()
	sslDir := t.TempDir()

	db1, err := NewListenDBService(dir, sslDir, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, db1.DocSet("test", "1", mustDocJSON(t, map[string]string{"val": "first"})))
	db1.Close()

	// Re-open same database — schema init should not fail or lose data
	db2, err := NewListenDBService(dir, sslDir, testutil.NewTestLogger())
	require.NoError(t, err)
	defer db2.Close()

	doc, err := db2.DocGet("test", "1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "first", docField(t, doc, "val"))
}

// ---------------------------------------------------------------------------
// Data directory creation
// ---------------------------------------------------------------------------

func TestCreateDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deep", "data")
	sslDir := t.TempDir()

	db, err := NewListenDBService(dir, sslDir, testutil.NewTestLogger())
	require.NoError(t, err)
	defer db.Close()

	_, err = os.Stat(filepath.Join(dir, "g8e.db"))
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// DocSet upsert behaviour
// ---------------------------------------------------------------------------

func TestDocSet_UpsertReplacesDataAndUpdatesTimestamp(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "alice"})))

	doc1, err := db.DocGet("users", "u1")
	require.NoError(t, err)
	createdAt1 := doc1.CreatedAt
	updatedAt1 := doc1.UpdatedAt

	time.Sleep(2 * time.Millisecond)

	require.NoError(t, db.DocSet("users", "u1", mustDocJSON(t, map[string]string{"name": "admin"})))

	doc2, err := db.DocGet("users", "u1")
	require.NoError(t, err)

	assert.Equal(t, "admin", docField(t, doc2, "name"))
	assert.True(t, doc2.CreatedAt.Equal(createdAt1), "created_at must not change on upsert")
	assert.True(t, doc2.UpdatedAt.After(updatedAt1), "updated_at must advance on upsert")
}

// ---------------------------------------------------------------------------
// DocUpdate timestamp preservation
// ---------------------------------------------------------------------------

func TestDocUpdate_PreservesCreatedAt(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.DocSet("things", "t1", mustDocJSON(t, map[string]string{"x": "original"})))

	doc1, err := db.DocGet("things", "t1")
	require.NoError(t, err)
	createdAt := doc1.CreatedAt

	time.Sleep(2 * time.Millisecond)

	doc2, err := db.DocUpdate("things", "t1", mustDocJSON(t, map[string]string{"x": "updated"}))
	require.NoError(t, err)

	assert.True(t, doc2.CreatedAt.Equal(createdAt), "created_at must not change on update")
	assert.Equal(t, "updated", docField(t, doc2, "x"))
}

// ---------------------------------------------------------------------------
// DocQuery injection guards
// ---------------------------------------------------------------------------

func TestDocQuery_InvalidFilterFieldReturnsError(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.DocSet("items", "i1", mustDocJSON(t, map[string]string{"name": "x"})))

	_, err := db.DocQuery("items", []models.DocFilter{
		{Field: "name; DROP TABLE documents--", Op: "==", Value: json.RawMessage(`"x"`)},
	}, "", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter field")
}

func TestDocQuery_InvalidOrderByFieldReturnsError(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.DocSet("items", "i1", mustDocJSON(t, map[string]string{"name": "x"})))

	_, err := db.DocQuery("items", nil, "name; DROP TABLE documents--", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid orderBy field")
}

func TestDocQuery_UnknownOpIsSkipped(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.DocSet("items", "i1", mustDocJSON(t, map[string]string{"name": "x"})))
	require.NoError(t, db.DocSet("items", "i2", mustDocJSON(t, map[string]string{"name": "y"})))

	results, err := db.DocQuery("items", []models.DocFilter{
		{Field: "name", Op: "LIKE", Value: json.RawMessage(`"x"`)},
	}, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 2, "unknown op must be skipped, returning all docs")
}

// ---------------------------------------------------------------------------
// KVSet overwrite
// ---------------------------------------------------------------------------

func TestKVSet_OverwriteReplacesValue(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("key1", "first", 0))
	require.NoError(t, db.KVSet("key1", "second", 0))

	val, found := db.KVGet("key1")
	require.True(t, found)
	assert.Equal(t, "second", val)
}

// ---------------------------------------------------------------------------
// KVTTL — no-expiry path
// ---------------------------------------------------------------------------

func TestKVTTL_NoExpiry(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("persistent", "val", 0))
	assert.Equal(t, -1, db.KVTTL("persistent"))
}

// ---------------------------------------------------------------------------
// KVScan
// ---------------------------------------------------------------------------

func TestKVScan_BasicScan(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("scan:a", "1", 0))
	require.NoError(t, db.KVSet("scan:b", "2", 0))
	require.NoError(t, db.KVSet("scan:c", "3", 0))
	require.NoError(t, db.KVSet("other:d", "4", 0))

	next, keys, err := db.KVScan("scan:*", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, next, "no next page when all results fit")
	assert.Len(t, keys, 3)
}

func TestKVScan_Pagination(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, db.KVSet(fmt.Sprintf("page:%d", i), "v", 0))
	}

	next1, page1, err := db.KVScan("page:*", 0, 2)
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	assert.Equal(t, 2, next1, "next cursor must be 2 after first page")

	next2, page2, err := db.KVScan("page:*", next1, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 2)
	assert.Equal(t, 4, next2)

	next3, page3, err := db.KVScan("page:*", next2, 2)
	require.NoError(t, err)
	assert.Len(t, page3, 1)
	assert.Equal(t, 0, next3, "next cursor must be 0 on last page")
}

func TestKVScan_EmptyResult(t *testing.T) {
	db := newTestDB(t)

	next, keys, err := db.KVScan("nothing:*", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, next)
	assert.Empty(t, keys)
}

func TestKVScan_DefaultCountApplied(t *testing.T) {
	db := newTestDB(t)

	for i := 0; i < 5; i++ {
		require.NoError(t, db.KVSet(fmt.Sprintf("dc:%d", i), "v", 0))
	}

	_, keys, err := db.KVScan("dc:*", 0, 0)
	require.NoError(t, err)
	assert.Len(t, keys, 5, "count=0 must default to 100 and return all 5 keys")
}

func TestKVScan_ExcludesExpiredKeys(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("live:key", "val", 0))
	require.NoError(t, db.KVSet("exp:key", "val", 1))

	time.Sleep(1100 * time.Millisecond)

	_, keys, err := db.KVScan("*", 0, 100)
	require.NoError(t, err)
	for _, k := range keys {
		assert.NotEqual(t, "exp:key", k, "expired key must not appear in scan results")
	}
}

// ---------------------------------------------------------------------------
// SSE Events
// ---------------------------------------------------------------------------

func TestSSEEventsCount_EmptyTable(t *testing.T) {
	db := newTestDB(t)

	count, err := db.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSSEEventsAppendAndCount(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.SSEEventsAppend("sess-1", "TEXT", `{"chunk":"hello"}`))
	require.NoError(t, db.SSEEventsAppend("sess-1", "TEXT", `{"chunk":"world"}`))
	require.NoError(t, db.SSEEventsAppend("sess-2", "DONE", `{}`))

	count, err := db.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestSSEEventsWipe_DeletesAllRows(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.SSEEventsAppend("sess-1", "TEXT", `{"chunk":"a"}`))
	require.NoError(t, db.SSEEventsAppend("sess-2", "DONE", `{}`))

	deleted, err := db.SSEEventsWipe()
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	count, err := db.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSSEEventsWipe_EmptyTableReturnsZero(t *testing.T) {
	db := newTestDB(t)

	deleted, err := db.SSEEventsWipe()
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

// ---------------------------------------------------------------------------
// RunTTLCleanup
// ---------------------------------------------------------------------------

func TestRunTTLCleanup_RemovesExpiredKVEntries(t *testing.T) {
	db := newTestDB(t)

	require.NoError(t, db.KVSet("ttl:keep", "val", 0))
	require.NoError(t, db.KVSet("ttl:expire", "val", 1))

	time.Sleep(1100 * time.Millisecond)

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
		_, err := db.KVKeys("ttl:*")
		if err != nil {
			return false
		}
		_, found := db.KVGet("ttl:expire")
		return !found
	}, 5*time.Second, 100*time.Millisecond)

	_, kept := db.KVGet("ttl:keep")
	assert.True(t, kept, "non-expired key must survive cleanup")
}
