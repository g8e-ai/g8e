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
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/internal/timesvc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDocumentStoreTestDB creates a minimal SQLite DB with just the documents schema
// for unit testing DocumentStoreService without pulling in the full CanonicalDBService.
func newDocumentStoreTestDB(t *testing.T) *sqliteutil.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "document_store_test.db")
	cfg := sqliteutil.DefaultDBConfig(dbPath)
	db, err := sqliteutil.OpenDB(cfg, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Create minimal schema for document store operations
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS documents (
			collection TEXT NOT NULL,
			id TEXT NOT NULL,
			data TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (collection, id)
		)
	`)
	require.NoError(t, err)
	return db
}

// newDocumentStoreService creates a DocumentStoreService with test DB and logger.
func newDocumentStoreService(t *testing.T) *DocumentStoreService {
	t.Helper()
	db := newDocumentStoreTestDB(t)
	return NewDocumentStoreService(db, testutil.NewTestLogger())
}

func TestNewDocumentStoreService(t *testing.T) {
	db := newDocumentStoreTestDB(t)
	logger := testutil.NewTestLogger()

	svc := NewDocumentStoreService(db, logger)

	assert.NotNil(t, svc)
	assert.Equal(t, db, svc.db)
	assert.Equal(t, logger, svc.logger)
}

func TestDocGet_Success(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Insert a document directly
	now := time.Now().UTC()
	nowStr := timesvc.FormatTimestamp(now)
	_, err := svc.db.Exec(
		"INSERT INTO documents (collection, id, data, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"users", "u1", `{"name":"alice","role":"admin"}`, nowStr, nowStr,
	)
	require.NoError(t, err)

	doc, err := svc.DocGet("users", "u1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "users", doc.Collection)
	assert.Equal(t, "u1", doc.ID)
	assert.Equal(t, "alice", docField(t, doc, "name"))
	assert.Equal(t, "admin", docField(t, doc, "role"))
	assert.False(t, doc.CreatedAt.IsZero())
	assert.False(t, doc.UpdatedAt.IsZero())
}

func TestDocGet_NotFound(t *testing.T) {
	svc := newDocumentStoreService(t)

	doc, err := svc.DocGet("users", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestDocGet_DatabaseError(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Close the DB to cause errors
	_ = svc.db.Close()

	_, err := svc.DocGet("users", "u1")
	require.Error(t, err)
}

func TestDocCreate_Success(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"name": "bob", "role": "user"})
	err := svc.DocCreate("users", "u2", data)
	require.NoError(t, err)

	doc, err := svc.DocGet("users", "u2")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "bob", docField(t, doc, "name"))
	assert.Equal(t, "user", docField(t, doc, "role"))
}

func TestDocCreate_AlreadyExists(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"name": "charlie"})
	err := svc.DocCreate("users", "u3", data)
	require.NoError(t, err)

	// Attempt to create duplicate
	err = svc.DocCreate("users", "u3", data)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrAlreadyExists)
}

func TestDocCreate_StripsSystemFields(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]json.RawMessage{
		"name":       json.RawMessage(`"dave"`),
		"id":         json.RawMessage(`"should-be-stripped"`),
		"created_at": json.RawMessage(`"should-be-stripped"`),
		"updated_at": json.RawMessage(`"should-be-stripped"`),
	})
	err := svc.DocCreate("users", "u4", data)
	require.NoError(t, err)

	doc, err := svc.DocGet("users", "u4")
	require.NoError(t, err)
	assert.Equal(t, "u4", doc.ID)
	assert.NotContains(t, doc.Data, "id")
	assert.NotContains(t, doc.Data, "created_at")
	assert.NotContains(t, doc.Data, "updated_at")
}

func TestDocCreate_InvalidJSON(t *testing.T) {
	svc := newDocumentStoreService(t)

	err := svc.DocCreate("users", "u5", json.RawMessage(`{invalid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal document")
}

func TestDocCreate_NilData(t *testing.T) {
	svc := newDocumentStoreService(t)

	err := svc.DocCreate("users", "u6", json.RawMessage(`null`))
	require.NoError(t, err)

	doc, err := svc.DocGet("users", "u6")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Empty(t, doc.Data)
}

func TestDocSet_Create(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"name": "eve"})
	err := svc.DocSet("users", "u7", data)
	require.NoError(t, err)

	doc, err := svc.DocGet("users", "u7")
	require.NoError(t, err)
	assert.Equal(t, "eve", docField(t, doc, "name"))
}

func TestDocSet_Update(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create initial document
	data1 := mustDocJSON(t, map[string]string{"name": "frank", "role": "user"})
	err := svc.DocSet("users", "u8", data1)
	require.NoError(t, err)

	doc1, err := svc.DocGet("users", "u8")
	require.NoError(t, err)
	createdAt := doc1.CreatedAt

	// Small delay to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Update with new data
	data2 := mustDocJSON(t, map[string]string{"name": "frank", "role": "admin"})
	err = svc.DocSet("users", "u8", data2)
	require.NoError(t, err)

	doc2, err := svc.DocGet("users", "u8")
	require.NoError(t, err)
	assert.Equal(t, "admin", docField(t, doc2, "role"))
	assert.True(t, doc2.CreatedAt.Equal(createdAt), "created_at must not change on upsert")
	assert.True(t, doc2.UpdatedAt.After(doc1.UpdatedAt), "updated_at must advance on upsert")
}

func TestDocSet_StripsSystemFields(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]json.RawMessage{
		"name":       json.RawMessage(`"grace"`),
		"id":         json.RawMessage(`"should-be-stripped"`),
		"created_at": json.RawMessage(`"should-be-stripped"`),
		"updated_at": json.RawMessage(`"should-be-stripped"`),
	})
	err := svc.DocSet("users", "u9", data)
	require.NoError(t, err)

	doc, err := svc.DocGet("users", "u9")
	require.NoError(t, err)
	assert.Equal(t, "u9", doc.ID)
	assert.NotContains(t, doc.Data, "id")
	assert.NotContains(t, doc.Data, "created_at")
	assert.NotContains(t, doc.Data, "updated_at")
}

func TestDocSet_InvalidJSON(t *testing.T) {
	svc := newDocumentStoreService(t)

	err := svc.DocSet("users", "u10", json.RawMessage(`{invalid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal document")
}

func TestDocSetWithTimestamps_CustomTimestamps(t *testing.T) {
	svc := newDocumentStoreService(t)

	customCreatedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	customUpdatedAt := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	data := mustDocJSON(t, map[string]string{"name": "henry"})
	err := svc.DocSetWithTimestamps("users", "u11", data, customCreatedAt, customUpdatedAt)
	require.NoError(t, err)

	doc, err := svc.DocGet("users", "u11")
	require.NoError(t, err)
	assert.Equal(t, customCreatedAt, doc.CreatedAt)
	assert.Equal(t, customUpdatedAt, doc.UpdatedAt)
}

func TestDocSetWithTimestamps_ZeroTimestamps(t *testing.T) {
	svc := newDocumentStoreService(t)

	before := time.Now().UTC().Truncate(time.Microsecond)

	data := mustDocJSON(t, map[string]string{"name": "iris"})
	err := svc.DocSetWithTimestamps("users", "u12", data, time.Time{}, time.Time{})
	require.NoError(t, err)

	doc, err := svc.DocGet("users", "u12")
	require.NoError(t, err)
	assert.True(t, doc.CreatedAt.After(before) || doc.CreatedAt.Equal(before))
	assert.True(t, doc.UpdatedAt.After(before) || doc.UpdatedAt.Equal(before))
}

func TestDocUpdate_Success(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create initial document
	data1 := mustDocJSON(t, map[string]string{"name": "jack", "role": "user", "temp": "value"})
	err := svc.DocSet("users", "u13", data1)
	require.NoError(t, err)

	doc1, err := svc.DocGet("users", "u13")
	require.NoError(t, err)
	createdAt := doc1.CreatedAt

	// Small delay to ensure timestamp changes
	time.Sleep(10 * time.Millisecond)

	// Update with partial data
	data2 := mustDocJSON(t, map[string]string{"role": "admin"})
	updated, err := svc.DocUpdate("users", "u13", data2)
	require.NoError(t, err)

	assert.Equal(t, "jack", docField(t, updated, "name"))
	assert.Equal(t, "admin", docField(t, updated, "role"))
	assert.Equal(t, "value", docField(t, updated, "temp"))
	assert.True(t, updated.CreatedAt.Equal(createdAt), "created_at must not change on update")
	assert.True(t, updated.UpdatedAt.After(doc1.UpdatedAt), "updated_at must advance on update")
}

func TestDocUpdate_NotFound(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"role": "admin"})
	_, err := svc.DocUpdate("users", "nonexistent", data)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestDocUpdate_DeleteFieldWithNull(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create document with field to delete
	data1 := mustDocJSON(t, map[string]string{"name": "kate", "temp": "remove_me"})
	err := svc.DocSet("users", "u14", data1)
	require.NoError(t, err)

	// Update with null to delete field
	data2 := mustDocJSON(t, map[string]json.RawMessage{"temp": nil})
	updated, err := svc.DocUpdate("users", "u14", data2)
	require.NoError(t, err)

	_, hasTemp := updated.Data["temp"]
	assert.False(t, hasTemp)
	assert.Equal(t, "kate", docField(t, updated, "name"))
}

func TestDocUpdate_IgnoresSystemFields(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create initial document
	data1 := mustDocJSON(t, map[string]string{"name": "leo"})
	err := svc.DocSet("users", "u15", data1)
	require.NoError(t, err)

	// Try to update system fields (should be ignored)
	data2 := mustDocJSON(t, map[string]json.RawMessage{
		"id":         json.RawMessage(`"new-id"`),
		"created_at": json.RawMessage(`"2024-01-01T00:00:00Z"`),
		"updated_at": json.RawMessage(`"2024-01-01T00:00:00Z"`),
		"name":       json.RawMessage(`"leonardo"`),
	})
	updated, err := svc.DocUpdate("users", "u15", data2)
	require.NoError(t, err)

	assert.Equal(t, "u15", updated.ID)
	assert.Equal(t, "leonardo", docField(t, updated, "name"))
	assert.NotContains(t, updated.Data, "id")
	assert.NotContains(t, updated.Data, "created_at")
	assert.NotContains(t, updated.Data, "updated_at")
}

func TestDocUpdate_InvalidJSON(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create document
	data1 := mustDocJSON(t, map[string]string{"name": "mike"})
	err := svc.DocSet("users", "u16", data1)
	require.NoError(t, err)

	// Try to update with invalid JSON
	_, err = svc.DocUpdate("users", "u16", json.RawMessage(`{invalid json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal fields")
}

func TestDocDelete_Success(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create document
	data := mustDocJSON(t, map[string]string{"name": "nancy"})
	err := svc.DocSet("users", "u17", data)
	require.NoError(t, err)

	// Delete document
	deleted, err := svc.DocDelete("users", "u17")
	require.NoError(t, err)
	assert.True(t, deleted)

	// Verify deletion
	doc, err := svc.DocGet("users", "u17")
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestDocDelete_NotFound(t *testing.T) {
	svc := newDocumentStoreService(t)

	deleted, err := svc.DocDelete("users", "nonexistent")
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestDocDeleteNamespace_Success(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create multiple documents in a namespace
	for i := 0; i < 5; i++ {
		data := mustDocJSON(t, map[string]string{"id": string(rune('a' + i))})
		err := svc.DocSet("test_ns", string(rune('a'+i)), data)
		require.NoError(t, err)
	}

	// Create documents in another namespace
	data := mustDocJSON(t, map[string]string{"id": "other"})
	err := svc.DocSet("other_ns", "other", data)
	require.NoError(t, err)

	// Delete namespace
	deleted, err := svc.DocDeleteNamespace("test_ns")
	require.NoError(t, err)
	assert.Equal(t, int64(5), deleted)

	// Verify deletion
	doc, err := svc.DocGet("test_ns", "a")
	require.NoError(t, err)
	assert.Nil(t, doc)

	// Verify other namespace untouched
	doc, err = svc.DocGet("other_ns", "other")
	require.NoError(t, err)
	assert.NotNil(t, doc)
}

func TestDocDeleteNamespace_EmptyNamespace(t *testing.T) {
	svc := newDocumentStoreService(t)

	deleted, err := svc.DocDeleteNamespace("empty_ns")
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

func TestDocDeleteNamespace_NonExistent(t *testing.T) {
	svc := newDocumentStoreService(t)

	deleted, err := svc.DocDeleteNamespace("nonexistent_ns")
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)
}

func TestGetField_Success(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create document with multiple field types
	data := mustDocJSON(t, map[string]interface{}{
		"name":  "olivia",
		"age":   30,
		"admin": true,
	})
	err := svc.DocSet("users", "u18", data)
	require.NoError(t, err)

	// Get string field
	field, err := svc.GetField("users", "u18", "name")
	require.NoError(t, err)
	require.NotNil(t, field.Str)
	assert.Equal(t, "olivia", *field.Str)

	// Get number field
	field, err = svc.GetField("users", "u18", "age")
	require.NoError(t, err)
	require.NotNil(t, field.Float64)
	assert.InEpsilon(t, float64(30), *field.Float64, 0.0)

	// Get boolean field - SQLite json_extract may return float64 for true/false
	field, err = svc.GetField("users", "u18", "admin")
	require.NoError(t, err)
	// JSON true unmarshals to bool in Go, but SQLite json_extract may return float64(1)
	// Accept either representation
	if field.Bool != nil {
		assert.True(t, *field.Bool)
	} else if field.Float64 != nil {
		assert.Equal(t, float64(1), *field.Float64)
	} else {
		assert.Fail(t, "unexpected type for boolean field")
	}
}

func TestGetField_NotFound(t *testing.T) {
	svc := newDocumentStoreService(t)

	_, err := svc.GetField("users", "nonexistent", "name")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestGetField_FieldNotFound(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"name": "peter"})
	err := svc.DocSet("users", "u19", data)
	require.NoError(t, err)

	// When a field doesn't exist, json_extract returns SQL NULL, and
	// json_quote(NULL) returns the JSON text "null", which unmarshals to Go nil.
	// convertToFieldValue(nil) produces FieldValue{Null: true}.
	field, err := svc.GetField("users", "u19", "nonexistent_field")
	require.NoError(t, err)
	assert.True(t, field.Null)
}

func TestGetField_DocumentNotFound(t *testing.T) {
	svc := newDocumentStoreService(t)

	_, err := svc.GetField("users", "nonexistent_doc", "name")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotFound)
}

func TestDocQuery_NoFilters(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create documents
	for i := 0; i < 3; i++ {
		data := mustDocJSON(t, map[string]string{"name": string(rune('a' + i))})
		err := svc.DocSet("items", string(rune('a'+i)), data)
		require.NoError(t, err)
	}

	results, err := svc.DocQuery("items", nil, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestDocQuery_WithFilter(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create documents with different statuses
	data1 := mustDocJSON(t, map[string]string{"status": "active", "name": "a"})
	data2 := mustDocJSON(t, map[string]string{"status": "inactive", "name": "b"})
	data3 := mustDocJSON(t, map[string]string{"status": "active", "name": "c"})
	require.NoError(t, svc.DocSet("items", "a", data1))
	require.NoError(t, svc.DocSet("items", "b", data2))
	require.NoError(t, svc.DocSet("items", "c", data3))

	filters := []models.DocFilter{
		{Field: "status", Op: "==", Value: json.RawMessage(`"active"`)},
	}

	results, err := svc.DocQuery("items", filters, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestDocQuery_WithLimit(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create documents
	for i := 0; i < 5; i++ {
		data := mustDocJSON(t, map[string]string{"id": string(rune('a' + i))})
		err := svc.DocSet("items", string(rune('a'+i)), data)
		require.NoError(t, err)
	}

	results, err := svc.DocQuery("items", nil, "", 2)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestDocQuery_WithOrderBy(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create documents with priority
	data1 := mustDocJSON(t, map[string]int{"priority": 3})
	data2 := mustDocJSON(t, map[string]int{"priority": 1})
	data3 := mustDocJSON(t, map[string]int{"priority": 2})
	require.NoError(t, svc.DocSet("items", "a", data1))
	require.NoError(t, svc.DocSet("items", "b", data2))
	require.NoError(t, svc.DocSet("items", "c", data3))

	results, err := svc.DocQuery("items", nil, "priority ASC", 0)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, float64(1), docField(t, results[0], "priority"))
	assert.Equal(t, float64(2), docField(t, results[1], "priority"))
	assert.Equal(t, float64(3), docField(t, results[2], "priority"))
}

func TestDocQuery_WithOrderByDesc(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create documents with priority
	data1 := mustDocJSON(t, map[string]int{"priority": 1})
	data2 := mustDocJSON(t, map[string]int{"priority": 3})
	data3 := mustDocJSON(t, map[string]int{"priority": 2})
	require.NoError(t, svc.DocSet("items", "a", data1))
	require.NoError(t, svc.DocSet("items", "b", data2))
	require.NoError(t, svc.DocSet("items", "c", data3))

	results, err := svc.DocQuery("items", nil, "priority DESC", 0)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, float64(3), docField(t, results[0], "priority"))
	assert.Equal(t, float64(2), docField(t, results[1], "priority"))
	assert.Equal(t, float64(1), docField(t, results[2], "priority"))
}

func TestDocQuery_MultipleFilters(t *testing.T) {
	svc := newDocumentStoreService(t)

	// Create documents
	data1 := mustDocJSON(t, map[string]interface{}{"status": "active", "priority": 1})
	data2 := mustDocJSON(t, map[string]interface{}{"status": "active", "priority": 2})
	data3 := mustDocJSON(t, map[string]interface{}{"status": "inactive", "priority": 1})
	require.NoError(t, svc.DocSet("items", "a", data1))
	require.NoError(t, svc.DocSet("items", "b", data2))
	require.NoError(t, svc.DocSet("items", "c", data3))

	filters := []models.DocFilter{
		{Field: "status", Op: "==", Value: json.RawMessage(`"active"`)},
		{Field: "priority", Op: "==", Value: json.RawMessage(`1`)},
	}

	results, err := svc.DocQuery("items", filters, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "a", results[0].ID)
}

func TestDocQuery_InvalidFilterField(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"name": "test"})
	require.NoError(t, svc.DocSet("items", "a", data))

	filters := []models.DocFilter{
		{Field: "name; DROP TABLE documents--", Op: "==", Value: json.RawMessage(`"test"`)},
	}

	_, err := svc.DocQuery("items", filters, "", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid filter field")
}

func TestDocQuery_InvalidOrderByField(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"name": "test"})
	require.NoError(t, svc.DocSet("items", "a", data))

	_, err := svc.DocQuery("items", nil, "name; DROP TABLE documents--", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid orderBy field")
}

func TestDocQuery_UnknownOperatorSkipped(t *testing.T) {
	svc := newDocumentStoreService(t)

	data1 := mustDocJSON(t, map[string]string{"name": "a"})
	data2 := mustDocJSON(t, map[string]string{"name": "b"})
	require.NoError(t, svc.DocSet("items", "a", data1))
	require.NoError(t, svc.DocSet("items", "b", data2))

	filters := []models.DocFilter{
		{Field: "name", Op: "LIKE", Value: json.RawMessage(`"a"`)},
	}

	results, err := svc.DocQuery("items", filters, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 2, "unknown operator should be skipped, returning all docs")
}

func TestDocQuery_NumericComparison(t *testing.T) {
	svc := newDocumentStoreService(t)

	data1 := mustDocJSON(t, map[string]int{"value": 10})
	data2 := mustDocJSON(t, map[string]int{"value": 20})
	data3 := mustDocJSON(t, map[string]int{"value": 30})
	require.NoError(t, svc.DocSet("items", "a", data1))
	require.NoError(t, svc.DocSet("items", "b", data2))
	require.NoError(t, svc.DocSet("items", "c", data3))

	t.Run("greater than", func(t *testing.T) {
		filters := []models.DocFilter{
			{Field: "value", Op: ">", Value: json.RawMessage(`15`)},
		}
		results, err := svc.DocQuery("items", filters, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("less than", func(t *testing.T) {
		filters := []models.DocFilter{
			{Field: "value", Op: "<", Value: json.RawMessage(`25`)},
		}
		results, err := svc.DocQuery("items", filters, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("greater or equal", func(t *testing.T) {
		filters := []models.DocFilter{
			{Field: "value", Op: ">=", Value: json.RawMessage(`20`)},
		}
		results, err := svc.DocQuery("items", filters, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("less or equal", func(t *testing.T) {
		filters := []models.DocFilter{
			{Field: "value", Op: "<=", Value: json.RawMessage(`20`)},
		}
		results, err := svc.DocQuery("items", filters, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("not equal", func(t *testing.T) {
		filters := []models.DocFilter{
			{Field: "value", Op: "!=", Value: json.RawMessage(`20`)},
		}
		results, err := svc.DocQuery("items", filters, "", 0)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}

func TestDocQuery_EmptyCollection(t *testing.T) {
	svc := newDocumentStoreService(t)

	results, err := svc.DocQuery("empty", nil, "", 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestDocQuery_EmptyFilterField(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"name": "test"})
	require.NoError(t, svc.DocSet("items", "a", data))

	filters := []models.DocFilter{
		{Field: "", Op: "==", Value: json.RawMessage(`"test"`)},
	}

	results, err := svc.DocQuery("items", filters, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 1, "empty field should be skipped")
}

func TestDocQuery_EmptyFilterOp(t *testing.T) {
	svc := newDocumentStoreService(t)

	data := mustDocJSON(t, map[string]string{"name": "test"})
	require.NoError(t, svc.DocSet("items", "a", data))

	filters := []models.DocFilter{
		{Field: "name", Op: "", Value: json.RawMessage(`"test"`)},
	}

	results, err := svc.DocQuery("items", filters, "", 0)
	require.NoError(t, err)
	assert.Len(t, results, 1, "empty operator should be skipped")
}

func TestScanDocument_Success(t *testing.T) {
	now := time.Now().UTC()
	nowStr := timesvc.FormatTimestamp(now)

	doc, err := scanDocument("users", "u1", `{"name":"alice"}`, nowStr, nowStr)
	require.NoError(t, err)
	assert.Equal(t, "users", doc.Collection)
	assert.Equal(t, "u1", doc.ID)
	assert.Equal(t, "alice", docField(t, doc, "name"))
	assert.False(t, doc.CreatedAt.IsZero())
	assert.False(t, doc.UpdatedAt.IsZero())
}

func TestScanDocument_InvalidTimestamp(t *testing.T) {

	_, err := scanDocument("users", "u1", `{"name":"alice"}`, "invalid-timestamp", "2024-01-01T00:00:00Z")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse created_at")
}

func TestScanDocument_InvalidData(t *testing.T) {
	now := time.Now().UTC()
	nowStr := timesvc.FormatTimestamp(now)

	_, err := scanDocument("users", "u1", `{invalid json}`, nowStr, nowStr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal document data")
}
