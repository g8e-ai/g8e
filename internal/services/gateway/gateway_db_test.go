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

//go:build integration

package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
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
	dataDir := t.TempDir()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	ks := newTestKeystore(t, secretsDir, logger)
	db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, constants.VaultDirname), logger, true, "", false, ks)
	require.NoError(t, err)
	defer db.Close()

	assert.NotNil(t, db.GetDB(), "GetDB should return non-nil database")
}

func TestCanonicalDBService_Wait(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	ks := newTestKeystore(t, secretsDir, logger)
	db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, constants.VaultDirname), logger, true, "", false, ks)
	require.NoError(t, err)

	// Close the database to stop background workers
	db.Close()

	// Wait should complete successfully after Close
	db.Wait()
}

func TestCanonicalDBService_SSEEventsListAllSince(t *testing.T) {
	t.Parallel()
	dataDir := t.TempDir()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()

	ks := newTestKeystore(t, secretsDir, logger)
	db, err := OpenCanonicalDBService(dataDir, secretsDir, filepath.Join(dataDir, constants.VaultDirname), logger, true, "", false, ks)
	require.NoError(t, err)
	defer db.Close()

	// Append some SSE events
	route := SSERoute{WebSessionID: "test-session"}
	for i := 0; i < 5; i++ {
		err := db.SSEStore.SSEEventsAppend(route, fmt.Sprintf("event-type-%d", i), fmt.Sprintf(`{"data":"%d"}`, i), "test-producer")
		require.NoError(t, err)
	}

	// List all events since ID 0 with limit 100
	rows, err := db.SSEStore.SSEEventsListAllSince(0, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 5, "Should return all 5 events")

	// List events since ID 3 with limit 100
	rows, err = db.SSEStore.SSEEventsListAllSince(3, 100)
	require.NoError(t, err)
	assert.Len(t, rows, 2, "Should return 2 events after ID 3")
}

func newTestDB(t *testing.T) *CanonicalDBService {
	t.Helper()
	dir := t.TempDir()
	secretsDir := t.TempDir()
	logger := testutil.NewTestLogger()
	ks := newTestKeystore(t, secretsDir, logger)
	db, err := OpenCanonicalDBService(dir, secretsDir, filepath.Join(dir, "vault"), logger, true, "", false, ks)
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
	require.Error(t, err)
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
// Schema initialization (idempotent)
// ---------------------------------------------------------------------------

func TestSchemaIdempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	secretsDir := t.TempDir()

	logger := testutil.NewTestLogger()
	ks1 := newTestKeystore(t, secretsDir, logger)
	db1, err := OpenCanonicalDBService(dir, secretsDir, filepath.Join(dir, "vault"), logger, true, "", false, ks1)
	require.NoError(t, err)
	require.NoError(t, db1.DocStore.DocSet("test", "1", mustDocJSON(t, map[string]string{"val": "first"})))
	db1.Close()

	// Re-open same database - schema init should not fail or lose data
	ks2 := newTestKeystore(t, secretsDir, logger)
	db2, err := OpenCanonicalDBService(dir, secretsDir, filepath.Join(dir, "vault"), logger, true, "", false, ks2)
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
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "nested", "deep", "data")
	secretsDir := t.TempDir()

	logger := testutil.NewTestLogger()
	ks := newTestKeystore(t, secretsDir, logger)
	db, err := OpenCanonicalDBService(dir, secretsDir, filepath.Join(dir, "vault"), logger, true, "", false, ks)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = os.Stat(filepath.Join(dir, constants.DbFilename))
	require.NoError(t, err)
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
// BlobStore overwrite
// ---------------------------------------------------------------------------

func TestSSEEventsCount_EmptyTable(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	count, err := db.SSEStore.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSSEEventsAppendAndCount(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.SSEStore.SSEEventsAppend(SSERoute{WebSessionID: "sess-1"}, "TEXT", `{"chunk":"hello"}`, ""))
	require.NoError(t, db.SSEStore.SSEEventsAppend(SSERoute{WebSessionID: "sess-1"}, "TEXT", `{"chunk":"world"}`, ""))
	require.NoError(t, db.SSEStore.SSEEventsAppend(SSERoute{CLISessionID: "sess-2"}, "DONE", `{}`, ""))

	count, err := db.SSEStore.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestSSEEventsWipe_DeletesAllRows(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.SSEStore.SSEEventsAppend(SSERoute{WebSessionID: "sess-1"}, "TEXT", `{"chunk":"a"}`, ""))
	require.NoError(t, db.SSEStore.SSEEventsAppend(SSERoute{CLISessionID: "sess-2"}, "DONE", `{}`, ""))

	deleted, err := db.SSEStore.SSEEventsWipe()
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	count, err := db.SSEStore.SSEEventsCount()
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestSSEEventsWipe_EmptyTableReturnsZero(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	deleted, err := db.SSEStore.SSEEventsWipe()
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

// ---------------------------------------------------------------------------
// BlobStore maintenance
// ---------------------------------------------------------------------------

func TestBlobStoreService_RunMaintenance(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	require.NoError(t, db.BlobStore.BlobPut("ns", "keep", []byte("data"), "text/plain", 0))
	require.NoError(t, db.BlobStore.BlobPut("ns", "expire", []byte("data"), "text/plain", 1))

	// Wait for expiry with polling
	require.Eventually(t, func() bool {
		_, _, found := db.BlobStore.BlobGet("ns", "expire")
		return !found
	}, 2*time.Second, 100*time.Millisecond, "expire blob should expire")

	// Run maintenance
	require.NoError(t, db.BlobStore.RunMaintenance())

	_, _, kept := db.BlobStore.BlobGet("ns", "keep")
	assert.True(t, kept, "non-expired blob must survive cleanup")

	_, _, expired := db.BlobStore.BlobGet("ns", "expire")
	assert.False(t, expired, "expired blob should be removed by maintenance")
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
	require.NotNil(t, field.Str)
	assert.Equal(t, "test-doc", *field.Str)

	// Get another field
	field, err = db.DocStore.GetField("test_collection", "doc1", "value")
	require.NoError(t, err)
	require.NotNil(t, field.Float64)
	assert.InEpsilon(t, float64(42), *field.Float64, 0.0) // JSON numbers are unmarshaled as float64

	// Get field from non-existent document
	_, err = db.DocStore.GetField("test_collection", "nonexistent-doc", "name")
	require.Error(t, err)
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
	require.Error(t, err)
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
	err = db.ReplayStore.FinalizeNonce("test123")
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
	err := db.ReplayStore.FinalizeNonce("nonexistent")
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
	err = db.ReplayStore.ReleaseNonce("test456")
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
	err := db.ReplayStore.ReleaseNonce("nonexistent")
	require.NoError(t, err)
}

func TestBlobDelete(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Create a blob
	blobData := []byte("test blob data")
	err := db.BlobStore.BlobPut("test_namespace", "blob1", blobData, "application/octet-stream", 0)
	require.NoError(t, err)

	// Delete the blob
	deleted, err := db.BlobStore.BlobDelete("test_namespace", "blob1")
	require.NoError(t, err)
	assert.True(t, deleted)

	// Verify blob was deleted
	_, _, found := db.BlobStore.BlobGet("test_namespace", "blob1")
	assert.False(t, found)
}

func TestBlobDelete_NonExistent(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	// Delete non-existent blob
	deleted, err := db.BlobStore.BlobDelete("test_namespace", "nonexistent")
	require.NoError(t, err)
	assert.False(t, deleted)
}
