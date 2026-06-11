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
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// DocumentStoreService provides collection/ID-based document CRUD operations.
type DocumentStoreService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
}

// NewDocumentStoreService creates a new document store service.
func NewDocumentStoreService(db *sqliteutil.DB, logger *slog.Logger) *DocumentStoreService {
	return &DocumentStoreService{
		db:     db,
		logger: logger,
	}
}

// DocGet retrieves a document by collection and id.
// Returns a typed Document with native time.Time timestamps, or nil if not found.
func (s *DocumentStoreService) DocGet(collection, id string) (*models.Document, error) {
	var dataJSON string
	var createdAtStr, updatedAtStr string
	err := s.db.QueryRowWithRetry(
		"SELECT data, created_at, updated_at FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	).Scan(&dataJSON, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return scanDocument(collection, id, dataJSON, createdAtStr, updatedAtStr)
}

// DocCreate creates a document only if it does not already exist. data must be valid JSON.
// Timestamps are managed by the service - created_at is set once on insert.
func (s *DocumentStoreService) DocCreate(collection, id string, data json.RawMessage) error {
	var userDoc map[string]json.RawMessage
	if err := json.Unmarshal(data, &userDoc); err != nil {
		return fmt.Errorf("DocumentStoreService: unmarshal document: %w", err)
	}
	if userDoc == nil {
		userDoc = make(map[string]json.RawMessage)
	}
	delete(userDoc, "id")
	delete(userDoc, "created_at")
	delete(userDoc, "updated_at")

	dataJSON, err := json.Marshal(userDoc)
	if err != nil {
		return fmt.Errorf("DocumentStoreService: marshal document: %w", err)
	}

	now := time.Now().UTC()
	nowStr := sqliteutil.FormatTimestamp(now)

	_, err = s.db.ExecWithRetry(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		collection, id, string(dataJSON), nowStr, nowStr,
	)
	if err != nil && sqliteutil.IsUniqueConstraintError(err) {
		return constants.ErrAlreadyExists
	}
	return err
}

// DocSet creates or replaces a document. data must be valid JSON.
// Timestamps are managed by the service - created_at is set once on insert and
// never overwritten. updated_at is refreshed on every upsert.
func (s *DocumentStoreService) DocSet(collection, id string, data json.RawMessage) error {
	return s.DocSetWithTimestamps(collection, id, data, time.Time{}, time.Time{})
}

// DocSetWithTimestamps creates or replaces a document with custom timestamps.
// This is a test-only hook for setting specific created_at/updated_at values.
// For production use, call DocSet instead which auto-manages timestamps.
// Zero-valued timestamps are replaced with time.Now().UTC().
func (s *DocumentStoreService) DocSetWithTimestamps(collection, id string, data json.RawMessage, createdAt, updatedAt time.Time) error {
	var userDoc map[string]json.RawMessage
	if err := json.Unmarshal(data, &userDoc); err != nil {
		return fmt.Errorf("DocumentStoreService: unmarshal document: %w", err)
	}
	if userDoc == nil {
		userDoc = make(map[string]json.RawMessage)
	}
	delete(userDoc, "id")
	delete(userDoc, "created_at")
	delete(userDoc, "updated_at")

	dataJSON, err := json.Marshal(userDoc)
	if err != nil {
		return fmt.Errorf("DocumentStoreService: marshal document: %w", err)
	}

	now := time.Now().UTC()
	createdAtStr := sqliteutil.FormatTimestamp(now)
	updatedAtStr := sqliteutil.FormatTimestamp(now)

	if !createdAt.IsZero() {
		createdAtStr = sqliteutil.FormatTimestamp(createdAt)
	}
	if !updatedAt.IsZero() {
		updatedAtStr = sqliteutil.FormatTimestamp(updatedAt)
	}

	_, err = s.db.ExecWithRetry(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(collection, id) DO UPDATE SET
		   data = excluded.data,
		   updated_at = excluded.updated_at`,
		collection, id, string(dataJSON), createdAtStr, updatedAtStr,
	)
	return err
}

// DocUpdate merges fields into an existing document. fields must be valid JSON.
// Returns the updated Document with native time.Time timestamps.
func (s *DocumentStoreService) DocUpdate(collection, id string, fields json.RawMessage) (*models.Document, error) {
	var existingJSON string
	var createdAtStr, updatedAtStr string
	err := s.db.QueryRowWithRetry(
		"SELECT data, created_at, updated_at FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	).Scan(&existingJSON, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, constants.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(existingJSON), &doc); err != nil {
		return nil, err
	}

	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(fields, &incoming); err != nil {
		return nil, fmt.Errorf("DocumentStoreService: unmarshal fields: %w", err)
	}

	for k, v := range incoming {
		if k == "id" || k == "created_at" || k == "updated_at" {
			continue
		}
		var nullCheck interface{}
		if err := json.Unmarshal(v, &nullCheck); err == nil && nullCheck == nil {
			delete(doc, k)
		} else {
			doc[k] = v
		}
	}

	dataJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	nowStr := sqliteutil.FormatTimestamp(now)

	_, err = s.db.ExecWithRetry(
		"UPDATE documents SET data = ?, updated_at = ? WHERE collection = ? AND id = ?",
		string(dataJSON), nowStr, collection, id,
	)
	if err != nil {
		return nil, err
	}

	return scanDocument(collection, id, string(dataJSON), createdAtStr, nowStr)
}

// DocDelete removes a document. Returns (true, nil) if deleted, (false, nil) if not found.
func (s *DocumentStoreService) DocDelete(collection, id string) (bool, error) {
	result, err := s.db.ExecWithRetry(
		"DELETE FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DocDeleteNamespace removes all documents in a collection.
// Returns the count of deleted documents.
func (s *DocumentStoreService) DocDeleteNamespace(collection string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM documents WHERE collection = ?", collection)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetField extracts a single field value from a document using dot notation.
// This is used for JIT field resolution with governed access controls.
func (s *DocumentStoreService) GetField(collection, id, fieldPath string) (json.RawMessage, error) {
	var dataJSON string
	err := s.db.QueryRowWithRetry(
		"SELECT data FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	).Scan(&dataJSON)
	if err == sql.ErrNoRows {
		return nil, constants.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Use SQL json_extract for efficient field extraction
	// This is safer than manual JSON parsing and leverages SQLite's JSON1 extension
	var fieldValue string
	query := "SELECT json_extract(data, ?) FROM documents WHERE collection = ? AND id = ?"

	// Convert dot notation to JSON path (e.g., "metadata.tags" -> "$.metadata.tags")
	jsonPath := "$." + fieldPath

	err = s.db.QueryRowWithRetry(query, jsonPath, collection, id).Scan(&fieldValue)
	if err != nil {
		return nil, fmt.Errorf("DocumentStoreService: extract field %s: %w", fieldPath, err)
	}

	// Return the raw string as json.RawMessage.
	// SQLite's json_extract returns SQL literals (true, false, null) as raw strings,
	// not JSON strings. The delegation wrapper in CanonicalDBService handles the
	// conversion from SQL literals to Go types.
	return json.RawMessage(fieldValue), nil
}

// DocQuery returns documents matching field conditions.
// Supported ops: ==, !=, <, >, <=, >=. orderBy is "field" or "field DESC". limit 0 means no limit.
func (s *DocumentStoreService) DocQuery(collection string, filters []models.DocFilter, orderBy string, limit int) ([]*models.Document, error) {
	var query strings.Builder
	query.WriteString("SELECT id, data, created_at, updated_at FROM documents WHERE collection = ?")
	args := []interface{}{collection}

	for _, f := range filters {
		if f.Field == "" || f.Op == "" {
			continue
		}

		var sqlOp string
		switch f.Op {
		case "==", "=":
			sqlOp = "="
		case "!=", "<", ">", "<=", ">=":
			sqlOp = f.Op
		default:
			continue
		}

		if err := sqliteutil.ValidateIdentifier(f.Field); err != nil {
			return nil, fmt.Errorf("DocumentStoreService: invalid filter field: %w", err)
		}

		// Use parameter for path and literals for operators to satisfy CodeQL.
		query.WriteString(" AND json_extract(data, ?) ")
		switch sqlOp {
		case "==", "=":
			query.WriteString("=")
		case "!=":
			query.WriteString("!=")
		case "<":
			query.WriteString("<")
		case ">":
			query.WriteString(">")
		case "<=":
			query.WriteString("<=")
		case ">=":
			query.WriteString(">=")
		}
		query.WriteString(" ?")

		var nativeVal interface{}
		if err := json.Unmarshal(f.Value, &nativeVal); err != nil {
			return nil, fmt.Errorf("DocumentStoreService: invalid filter value: %w", err)
		}
		args = append(args, "$."+f.Field, nativeVal)
	}

	if orderBy != "" {
		parts := strings.Fields(orderBy)
		orderField := parts[0]
		dir := "ASC"
		if len(parts) > 1 && strings.EqualFold(parts[1], "DESC") {
			dir = "DESC"
		}

		if err := sqliteutil.ValidateIdentifier(orderField); err != nil {
			return nil, fmt.Errorf("DocumentStoreService: invalid orderBy field: %w", err)
		}

		// Identifier is validated, dir is whitelisted to ASC/DESC.
		// Use validated hardcoded branch to satisfy CodeQL sql-injection rule.
		query.WriteString(" ORDER BY json_extract(data, ?)")
		if dir == "DESC" {
			query.WriteString(" DESC")
		} else {
			query.WriteString(" ASC")
		}
		args = append(args, "$."+orderField)
	}

	if limit > 0 {
		query.WriteString(" LIMIT ?")
		args = append(args, limit)
	}

	type docRow struct {
		docID        string
		dataJSON     string
		createdAtStr string
		updatedAtStr string
	}

	rows, err := sqliteutil.MaterializeRows(s.db, query.String(), args, func(r *sql.Rows) (docRow, error) {
		var row docRow
		if err := r.Scan(&row.docID, &row.dataJSON, &row.createdAtStr, &row.updatedAtStr); err != nil {
			return docRow{}, err
		}
		return row, nil
	})
	if err != nil {
		return nil, err
	}

	results := make([]*models.Document, 0, len(rows))
	for _, row := range rows {
		doc, err := scanDocument(collection, row.docID, row.dataJSON, row.createdAtStr, row.updatedAtStr)
		if err != nil {
			return nil, err
		}
		results = append(results, doc)
	}
	return results, nil
}
