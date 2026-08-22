// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/timesvc"
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
		return nil, fmt.Errorf("gateway: document store: get: %w", err)
	}
	return scanDocument(collection, id, dataJSON, createdAtStr, updatedAtStr)
}

// DocCreate creates a document only if it does not already exist. data must be valid JSON.
// Timestamps are managed by the service - created_at is set once on insert.
func (s *DocumentStoreService) DocCreate(collection, id string, data json.RawMessage) error {
	var userDoc map[string]json.RawMessage
	if err := json.Unmarshal(data, &userDoc); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
	}
	if userDoc == nil {
		userDoc = make(map[string]json.RawMessage)
	}
	delete(userDoc, "id")
	delete(userDoc, "created_at")
	delete(userDoc, "updated_at")

	dataJSON, err := json.Marshal(userDoc)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}

	now := time.Now().UTC()
	nowStr := timesvc.FormatTimestamp(now)

	_, err = s.db.ExecWithRetry(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		collection, id, string(dataJSON), nowStr, nowStr,
	)
	if err != nil && sqliteutil.IsUniqueConstraintError(err) {
		return constants.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("gateway: document store: create: %w", err)
	}
	return nil
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
		return fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
	}
	if userDoc == nil {
		userDoc = make(map[string]json.RawMessage)
	}
	delete(userDoc, "id")
	delete(userDoc, "created_at")
	delete(userDoc, "updated_at")

	dataJSON, err := json.Marshal(userDoc)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}

	now := time.Now().UTC()
	createdAtStr := timesvc.FormatTimestamp(now)
	updatedAtStr := timesvc.FormatTimestamp(now)

	if !createdAt.IsZero() {
		createdAtStr = timesvc.FormatTimestamp(createdAt)
	}
	if !updatedAt.IsZero() {
		updatedAtStr = timesvc.FormatTimestamp(updatedAt)
	}

	_, err = s.db.ExecWithRetry(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(collection, id) DO UPDATE SET
		   data = excluded.data,
		   updated_at = excluded.updated_at`,
		collection, id, string(dataJSON), createdAtStr, updatedAtStr,
	)
	if err != nil {
		return fmt.Errorf("gateway: document store: set with timestamps: %w", err)
	}
	return nil
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
		return nil, fmt.Errorf("gateway: document store: update: unmarshal existing: %w", err)
	}

	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(fields, &incoming); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalFields, err)
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
		return nil, fmt.Errorf("gateway: document store: update: marshal: %w", err)
	}

	now := time.Now().UTC()
	nowStr := timesvc.FormatTimestamp(now)

	_, err = s.db.ExecWithRetry(
		"UPDATE documents SET data = ?, updated_at = ? WHERE collection = ? AND id = ?",
		string(dataJSON), nowStr, collection, id,
	)
	if err != nil {
		return nil, fmt.Errorf("gateway: document store: update: %w", err)
	}

	return scanDocument(collection, id, string(dataJSON), createdAtStr, nowStr)
}

// DocConditionalUpdate atomically updates a document's JSON fields only if the
// existing data satisfies a JSON path condition. This prevents TOCTOU races by
// performing the check and write in a single SQL statement.
// Returns (true, nil) if the update was applied, (false, nil) if the condition
// was not met or the document was not found.
func (s *DocumentStoreService) DocConditionalUpdate(collection, id string, setFields map[string]interface{}, conditionField string, conditionValue interface{}) (bool, error) {
	now := time.Now().UTC()
	nowStr := timesvc.FormatTimestamp(now)

	// Build json_set chain for each field to set.
	// Values are wrapped in json(?) so that Go true/false become JSON booleans
	// (not integers 1/0) and strings are properly quoted in the JSON document.
	dataExpr := "data"
	args := []interface{}{}
	for field, val := range setFields {
		jsonVal, err := json.Marshal(val)
		if err != nil {
			return false, fmt.Errorf("gateway: document store: conditional update: marshal value: %w", err)
		}
		dataExpr = "json_set(" + dataExpr + ", ?, json(?))"
		args = append(args, "$."+field, string(jsonVal))
	}

	query := "UPDATE documents SET data = " + dataExpr + ", updated_at = ? WHERE collection = ? AND id = ? AND json_extract(data, ?) = ?"
	args = append(args, nowStr, collection, id, "$."+conditionField, conditionValue)

	result, err := s.db.ExecWithRetry(query, args...)
	if err != nil {
		return false, fmt.Errorf("gateway: document store: conditional update: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("gateway: document store: conditional update: rows affected: %w", err)
	}
	return n > 0, nil
}

// DocDelete removes a document, returning only an error. It satisfies the
// governance.TransactionAuditStore interface. A not-found result is not an
// error — the document is simply already absent. Callers that need the
// deleted/not-found distinction should use DocDeleteWithResult.
func (s *DocumentStoreService) DocDelete(collection, id string) error {
	_, err := s.DocDeleteWithResult(collection, id)
	return err
}

// DocDeleteWithResult removes a document. Returns (true, nil) if deleted,
// (false, nil) if not found.
func (s *DocumentStoreService) DocDeleteWithResult(collection, id string) (bool, error) {
	result, err := s.db.ExecWithRetry(
		"DELETE FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	)
	if err != nil {
		return false, fmt.Errorf("gateway: document store: delete: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("gateway: document store: delete: rows affected: %w", err)
	}
	return n > 0, nil
}

// DocDeleteNamespace removes all documents in a collection.
// Returns the count of deleted documents.
func (s *DocumentStoreService) DocDeleteNamespace(collection string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM documents WHERE collection = ?", collection)
	if err != nil {
		return 0, fmt.Errorf("gateway: document store: delete namespace: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("gateway: document store: delete namespace: rows affected: %w", err)
	}
	return n, nil
}

// GetField extracts a single field value from a document using dot notation.
// This is used for JIT field resolution with governed access controls.
func (s *DocumentStoreService) GetField(collection, id, fieldPath string) (mcp.FieldValue, error) {
	// Use json_quote(json_extract(...)) so SQLite re-encodes the extracted value as
	// valid JSON regardless of its native type (TEXT, INTEGER, REAL, NULL).
	// json_extract alone returns SQL TEXT without quotes for JSON strings, which is
	// not valid JSON. json_quote wraps strings in quotes and leaves numbers/booleans as-is.
	// json_quote(NULL) returns the text 'null' (not SQL NULL), so we get valid JSON.
	query := "SELECT json_quote(json_extract(data, ?)) FROM documents WHERE collection = ? AND id = ?"
	jsonPath := "$." + fieldPath

	var encoded *string
	err := s.db.QueryRowWithRetry(query, jsonPath, collection, id).Scan(&encoded)
	if err == sql.ErrNoRows {
		return mcp.FieldValue{}, constants.ErrNotFound
	}
	if err != nil {
		return mcp.FieldValue{}, fmt.Errorf("%w: %w", constants.ErrDocumentStoreExtractField, err)
	}
	if encoded == nil {
		return mcp.FieldValue{}, constants.ErrNotFound
	}

	var out interface{}
	if err := json.Unmarshal([]byte(*encoded), &out); err != nil {
		return mcp.FieldValue{}, fmt.Errorf("%w: %w", constants.ErrDocumentStoreDecodeField, err)
	}

	return mcp.ConvertToFieldValue(out), nil
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
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreInvalidFilterField, err)
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
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreInvalidFilterValue, err)
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
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreInvalidOrderByField, err)
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
			return docRow{}, fmt.Errorf("gateway: document store: query: scan: %w", err)
		}
		return row, nil
	})
	if err != nil {
		return nil, fmt.Errorf("gateway: document store: query: %w", err)
	}

	results := make([]*models.Document, 0, len(rows))
	for _, row := range rows {
		doc, err := scanDocument(collection, row.docID, row.dataJSON, row.createdAtStr, row.updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("gateway: document store: query: %w", err)
		}
		results = append(results, doc)
	}
	return results, nil
}

// scanDocument converts raw database row data into a typed Document with native time.Time timestamps.
func scanDocument(collection, id, dataJSON, createdAtStr, updatedAtStr string) (*models.Document, error) {
	createdAt, err := timesvc.ParseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreParseCreatedAt, err)
	}
	updatedAt, err := timesvc.ParseTimestamp(updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreParseUpdatedAt, err)
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalData, err)
	}

	return &models.Document{
		Collection: collection,
		ID:         id,
		Data:       data,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}
