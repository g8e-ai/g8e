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
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hash"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
)

// gatewaySchema is the canonical operator SQLite schema, embedded at compile time
// from `schema.sql`. That file is the single source of truth - do not inline
// CREATE TABLE statements in Go code.
//
//go:embed db/schema.sql
var gatewaySchema string

// GatewaySchema returns the embedded gateway schema SQL.
// This is exported for use in integration tests that need to set up a test database.
func GatewaySchema() string {
	return gatewaySchema
}

// GatewayDBService provides the unified SQLite persistence layer for gateway mode.
// Three subsystems:
//   - Document store: collection/id based CRUD (replaces client+agent separate SQLite DBs)
//   - KV store with TTL: key/value with optional expiration
//   - SSE event buffer: per-session event ring buffer
type GatewayDBService struct {
	db         *sqliteutil.DB
	logger     *slog.Logger
	AuditVault *storage.AuditVaultService

	// Shutdown tracking
	mu      sync.Mutex
	running bool
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// OpenGatewayDBService opens (or creates) the unified SQLite database.
// testMode enables the in-memory keystore backend for unit tests.
func OpenGatewayDBService(dataDir string, secretsDir string, logger *slog.Logger, testMode bool) (*GatewayDBService, error) {
	dbPath := filepath.Join(dataDir, "g8e.db")
	cfg := sqliteutil.DefaultDBConfig(dbPath)

	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open gateway database: %w", err)
	}

	// Initialize Audit Vault for transaction-native audit recording
	auditVaultConfig := storage.DefaultAuditVaultConfig()
	auditVaultConfig.DataDir = dataDir
	auditVaultConfig.GitPath = system.GitEmbedded
	auditVault, err := storage.NewAuditVaultService(auditVaultConfig, logger)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize audit vault: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	svc := &GatewayDBService{
		db:         db,
		logger:     logger,
		AuditVault: auditVault,
		ctx:        ctx,
		cancel:     cancel,
		running:    true,
	}

	if testMode {
		if err := svc.initTestSchema(secretsDir); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}
	} else {
		if err := svc.initSchema(secretsDir); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}
	}

	// Initialize state root if missing
	if err := svc.initStateRoot(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize state root: %w", err)
	}

	// Start background maintenance
	svc.wg.Add(1)
	go svc.RunMaintenance(svc.ctx)

	logger.Info("Gateway database initialized", "path", dbPath)
	return svc, nil
}

func (s *GatewayDBService) initTestSchema(secretsDir string) error {
	_, err := s.db.ExecWithRetry(gatewaySchema)
	if err != nil {
		return err
	}
	// Migration: Add producer_id column to sse_events table if it doesn't exist
	_, err = s.db.ExecWithRetry("ALTER TABLE sse_events ADD COLUMN producer_id TEXT")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		s.logger.Warn("Failed to add producer_id column to sse_events (may already exist)", "error", err)
	}
	backend, err := keystore.NewTestBackend()
	if err != nil {
		return err
	}
	ks, err := keystore.NewWithBackend(secretsDir, s.logger, backend)
	if err != nil {
		return err
	}
	if err := ks.Initialize(); err != nil {
		return err
	}
	if err := ks.EnsurePermissions(); err != nil {
		return err
	}
	sm := &SecretManager{
		db:         s.db,
		secretsDir: secretsDir,
		logger:     s.logger,
		keystore:   ks,
	}
	return sm.InitAppSettings()
}

func (s *GatewayDBService) initStateRoot() error {
	var count int
	err := s.db.QueryRowWithRetry("SELECT COUNT(*) FROM state_root").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		root, err := s.calculateStateRoot()
		if err != nil {
			return err
		}
		_, err = s.db.ExecWithRetry(
			"INSERT INTO state_root (id, root, updated_at) VALUES (1, ?, ?)",
			root,
			sqliteutil.FormatTimestamp(time.Now().UTC()),
		)
		return err
	}
	return nil
}

// GetCurrentStateRoot returns the current state merkle root.
func (s *GatewayDBService) GetCurrentStateRoot() (string, error) {
	root, err := s.calculateStateRoot()
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecWithRetry(
		`INSERT INTO state_root (id, root, updated_at)
		 VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET root = excluded.root, updated_at = excluded.updated_at`,
		root,
		sqliteutil.FormatTimestamp(time.Now().UTC()),
	)
	if err != nil {
		return "", err
	}
	return root, nil
}

func (s *GatewayDBService) calculateStateRoot() (string, error) {
	h := sha256.New()

	// 1. Documents (Authoritative)
	// Exclude metadata-only timestamps (created_at, updated_at) to ensure
	// the state root only changes when the content actually changes.
	if err := s.hashTableToStream(h, "SELECT collection, id, data FROM documents ORDER BY collection, id", nil, func(r *sql.Rows) error {
		var collection, id, data string
		if err := r.Scan(&collection, &id, &data); err != nil {
			return err
		}
		return writeRowToHash(h, "documents", collection, id, data)
	}); err != nil {
		s.logger.Error("Failed to query documents for state root calculation", "error", err)
		return "", err
	}

	now := sqliteutil.NowTimestamp()

	// 2. KV Store (Authoritative)
	// Filter for active entries only. Exclude created_at.
	// expires_at IS included because it affects the active state of the entry.
	if err := s.hashTableToStream(h, "SELECT key, value, COALESCE(expires_at, '') FROM kv_store WHERE expires_at IS NULL OR expires_at > ? ORDER BY key", []interface{}{now}, func(r *sql.Rows) error {
		var key, value, expiresAt string
		if err := r.Scan(&key, &value, &expiresAt); err != nil {
			return err
		}
		return writeRowToHash(h, "kv_store", key, value, expiresAt)
	}); err != nil {
		s.logger.Error("Failed to query kv_store for state root calculation", "error", err)
		return "", err
	}

	// 3. Blobs (Authoritative)
	// Filter for active entries only. Exclude created_at.
	// data is included (as hex for determinism).
	if err := s.hashTableToStream(h, "SELECT namespace, id, size, content_type, hex(data), COALESCE(expires_at, '') FROM blobs WHERE expires_at IS NULL OR expires_at > ? ORDER BY namespace, id", []interface{}{now}, func(r *sql.Rows) error {
		var namespace, id, contentType, dataHex, expiresAt string
		var size int64
		if err := r.Scan(&namespace, &id, &size, &contentType, &dataHex, &expiresAt); err != nil {
			return err
		}
		return writeRowToHash(h, "blobs", namespace, id, size, contentType, dataHex, expiresAt)
	}); err != nil {
		s.logger.Error("Failed to query blobs for state root calculation", "error", err)
		return "", err
	}

	// 4. Nonces and SSE events are EXCLUDED (volatile/metadata)

	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

// hashTableToStream executes a query and streams each row to the hash writer.
// This avoids materializing all rows in memory, which is critical for large databases.
func (s *GatewayDBService) hashTableToStream(h hash.Hash, query string, args []interface{}, scan func(*sql.Rows) error) error {
	rows, err := s.db.QueryWithRetry(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// writeRowToHash writes a row's values to the hash in a deterministic format.
// The format matches the previous JSON structure for compatibility:
// {"table":"table_name","values":[v1,v2,...]}
func writeRowToHash(h hash.Hash, table string, values ...interface{}) error {
	// Write deterministic JSON-like format directly to hash
	// This avoids allocating intermediate JSON strings
	h.Write([]byte(`{"table":"`))
	h.Write([]byte(table))
	h.Write([]byte(`","values":[`))

	for i, v := range values {
		if i > 0 {
			h.Write([]byte(","))
		}
		switch val := v.(type) {
		case string:
			h.Write([]byte(`"`))
			h.Write([]byte(val))
			h.Write([]byte(`"`))
		case int64:
			h.Write([]byte(fmt.Sprintf("%d", val)))
		default:
			return fmt.Errorf("unsupported type %T for state root hashing", v)
		}
	}

	h.Write([]byte("]}\n"))
	return nil
}

// ReserveNonce atomically reserves a nonce for early replay protection.
// Returns true if the nonce was already reserved/used (replay detected).
// If not used, it reserves the nonce and returns false.
func (s *GatewayDBService) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	// 1. Check if exists
	var existing string
	err := s.db.QueryRowWithRetry("SELECT nonce FROM nonces WHERE nonce = ?", nonce).Scan(&existing)
	if err == nil {
		return true, nil // Replay detected
	}
	if err != sql.ErrNoRows {
		return false, err
	}

	// 2. Not used, insert as reserved
	expStr := sqliteutil.FormatTimestamp(expiresAt)
	_, err = s.db.ExecWithRetry("INSERT INTO nonces (nonce, expires_at, status) VALUES (?, ?, 'reserved')", nonce, expStr)
	if err != nil {
		// Concurrent insert might fail with constraint violation - that's a replay
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return true, nil
		}
		return false, err
	}

	return false, nil
}

// FinalizeNonce marks a reserved nonce as fully consumed.
func (s *GatewayDBService) FinalizeNonce(nonce string) error {
	_, err := s.db.ExecWithRetry("UPDATE nonces SET status = 'used' WHERE nonce = ? AND status = 'reserved'", nonce)
	if err != nil {
		return fmt.Errorf("failed to finalize nonce: %w", err)
	}
	return nil
}

// ReleaseNonce removes a reservation for a failed transaction.
func (s *GatewayDBService) ReleaseNonce(nonce string) error {
	_, err := s.db.ExecWithRetry("DELETE FROM nonces WHERE nonce = ? AND status = 'reserved'", nonce)
	if err != nil {
		return fmt.Errorf("failed to release nonce: %w", err)
	}
	return nil
}

// RunMaintenance periodically removes expired entries.
func (s *GatewayDBService) RunMaintenance(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := sqliteutil.NowTimestamp()
			// KV store
			_, _ = s.db.ExecWithRetry("DELETE FROM kv_store WHERE expires_at IS NOT NULL AND expires_at < ?", now)
			// Blobs
			_, _ = s.db.ExecWithRetry("DELETE FROM blobs WHERE expires_at IS NOT NULL AND expires_at < ?", now)
			// Nonces
			_, _ = s.db.ExecWithRetry("DELETE FROM nonces WHERE expires_at < ?", now)
			// Suspended transactions
			_, _ = s.db.ExecWithRetry("DELETE FROM suspended_transactions WHERE expires_at < ?", now)
		}
	}
}

func (s *GatewayDBService) initSchema(secretsDir string) error {
	_, err := s.db.ExecWithRetry(gatewaySchema)
	if err != nil {
		return err
	}

	// Migration: Add producer_id column to sse_events table if it doesn't exist
	_, err = s.db.ExecWithRetry("ALTER TABLE sse_events ADD COLUMN producer_id TEXT")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		s.logger.Warn("Failed to add producer_id column to sse_events (may already exist)", "error", err)
	}

	sm, err := NewSecretManager(s.db, secretsDir, s.logger)
	if err != nil {
		return err
	}
	if err := sm.InitAppSettings(); err != nil {
		return err
	}

	// Migration: Move plaintext service certificate private keys to keystore
	if err := s.migratePlaintextServiceKeys(secretsDir, sm); err != nil {
		return err
	}

	return nil
}

// migratePlaintextServiceKeys moves existing plaintext service certificate private keys
// to the keystore and deletes the plaintext files. This is a one-time migration.
func (s *GatewayDBService) migratePlaintextServiceKeys(secretsDir string, sm *SecretManager) error {
	// Check if migration marker exists in keystore
	_, err := sm.keystore.DecryptSecret("migration_plaintext_keys_migrated")
	if err == nil {
		// Already migrated
		return nil
	}

	pkiDir := filepath.Join(filepath.Dir(secretsDir), "pki")
	keyPaths := []string{
		filepath.Join(pkiDir, "issued", "hub", "operator-gateway.key"),
	}

	migratedCount := 0
	for _, keyPath := range keyPaths {
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			continue
		}

		// Read the plaintext key
		keyPEM, err := os.ReadFile(keyPath)
		if err != nil {
			s.logger.Warn("[Migration] Failed to read plaintext key file", "path", keyPath, "error", err)
			continue
		}

		block, _ := pem.Decode(keyPEM)
		if block == nil {
			s.logger.Warn("[Migration] Failed to decode PEM key file", "path", keyPath)
			continue
		}

		// Determine service name from path
		var serviceName string
		if strings.Contains(keyPath, "operator-gateway.key") {
			serviceName = "operator-gateway"
		} else {
			s.logger.Warn("[Migration] Unknown service key file", "path", keyPath)
			continue
		}

		// Store in keystore
		if err := sm.StoreServicePrivateKey(serviceName, block.Bytes); err != nil {
			s.logger.Warn("[Migration] Failed to store key in keystore", "service", serviceName, "error", err)
			continue
		}

		// Delete plaintext file
		if err := os.Remove(keyPath); err != nil {
			s.logger.Warn("[Migration] Failed to delete plaintext key file", "path", keyPath, "error", err)
			continue
		}

		s.logger.Info("[Migration] Migrated plaintext service key to keystore", "service", serviceName, "path", keyPath)
		migratedCount++
	}

	if migratedCount > 0 {
		// Mark migration as complete
		_ = sm.keystore.EncryptSecret("migration_plaintext_keys_migrated", "true")
		s.logger.Info("[Migration] Completed plaintext service key migration", "count", migratedCount)
	}

	return nil
}

// GetDB returns the underlying SQLite database connection.
func (s *GatewayDBService) GetDB() *sqliteutil.DB {
	return s.db
}

// Close closes the database connection and waits for background workers.
func (s *GatewayDBService) Close() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.cancel()
	s.mu.Unlock()

	// Wait for background workers with a timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All workers finished cleanly
	case <-time.After(30 * time.Second):
		s.logger.Warn("GatewayDBService shutdown timeout, forcing close")
	}

	if err := s.db.Close(); err != nil {
		s.logger.Error("Database close error", "error", err)
		return err
	}
	return nil
}

// Wait blocks until all background workers have finished.
func (s *GatewayDBService) Wait() {
	s.wg.Wait()
}

// =============================================================================
// Document Store - collection/id based CRUD
// =============================================================================

// DocGet retrieves a document by collection and id.
// Returns a typed Document with native time.Time timestamps, or nil if not found.
func (s *GatewayDBService) DocGet(collection, id string) (*models.Document, error) {
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
func (s *GatewayDBService) DocCreate(collection, id string, data json.RawMessage) error {
	var userDoc map[string]json.RawMessage
	if err := json.Unmarshal(data, &userDoc); err != nil {
		return fmt.Errorf("failed to unmarshal document: %w", err)
	}
	if userDoc == nil {
		userDoc = make(map[string]json.RawMessage)
	}
	delete(userDoc, "id")
	delete(userDoc, "created_at")
	delete(userDoc, "updated_at")

	dataJSON, err := json.Marshal(userDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	now := time.Now().UTC()
	nowStr := sqliteutil.FormatTimestamp(now)

	_, err = s.db.ExecWithRetry(
		`INSERT INTO documents (collection, id, data, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		collection, id, string(dataJSON), nowStr, nowStr,
	)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return fmt.Errorf("document already exists")
	}
	return err
}

// DocSet creates or replaces a document. data must be valid JSON.
// Timestamps are managed by the service - created_at is set once on insert and
// never overwritten. updated_at is refreshed on every upsert.
func (s *GatewayDBService) DocSet(collection, id string, data json.RawMessage) error {
	return s.DocSetWithTimestamps(collection, id, data, time.Time{}, time.Time{})
}

// DocSetWithTimestamps creates or replaces a document with custom timestamps.
// This is a test-only hook for setting specific created_at/updated_at values.
// For production use, call DocSet instead which auto-manages timestamps.
// Zero-valued timestamps are replaced with time.Now().UTC().
func (s *GatewayDBService) DocSetWithTimestamps(collection, id string, data json.RawMessage, createdAt, updatedAt time.Time) error {
	var userDoc map[string]json.RawMessage
	if err := json.Unmarshal(data, &userDoc); err != nil {
		return fmt.Errorf("failed to unmarshal document: %w", err)
	}
	if userDoc == nil {
		userDoc = make(map[string]json.RawMessage)
	}
	delete(userDoc, "id")
	delete(userDoc, "created_at")
	delete(userDoc, "updated_at")

	dataJSON, err := json.Marshal(userDoc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
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
func (s *GatewayDBService) DocUpdate(collection, id string, fields json.RawMessage) (*models.Document, error) {
	var existingJSON string
	var createdAtStr, updatedAtStr string
	err := s.db.QueryRowWithRetry(
		"SELECT data, created_at, updated_at FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	).Scan(&existingJSON, &createdAtStr, &updatedAtStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document not found: %s/%s", collection, id)
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
		return nil, fmt.Errorf("failed to unmarshal fields: %w", err)
	}

	for k, v := range incoming {
		if k == "id" || k == "created_at" || k == "updated_at" {
			continue
		}
		if string(v) == "null" {
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
func (s *GatewayDBService) DocDelete(collection, id string) (bool, error) {
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
func (s *GatewayDBService) DocDeleteNamespace(collection string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM documents WHERE collection = ?", collection)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetField extracts a single field value from a document using dot notation.
// This is used for JIT field resolution with governed access controls.
func (s *GatewayDBService) GetField(collection, id, fieldPath string) (interface{}, error) {
	var dataJSON string
	err := s.db.QueryRowWithRetry(
		"SELECT data FROM documents WHERE collection = ? AND id = ?",
		collection, id,
	).Scan(&dataJSON)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document not found: %s/%s", collection, id)
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
		return nil, fmt.Errorf("failed to extract field %s: %w", fieldPath, err)
	}

	// Parse the extracted value back into a Go type
	var result interface{}
	if err := json.Unmarshal([]byte(fieldValue), &result); err != nil {
		// If it's a simple string, return it directly
		return fieldValue, nil
	}

	return result, nil
}

// DocQuery returns documents matching field conditions.
// Supported ops: ==, !=, <, >, <=, >=. orderBy is "field" or "field DESC". limit 0 means no limit.
func (s *GatewayDBService) DocQuery(collection string, filters []models.DocFilter, orderBy string, limit int) ([]*models.Document, error) {
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
			return nil, fmt.Errorf("invalid filter field: %w", err)
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
			return nil, fmt.Errorf("invalid filter value: %w", err)
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
			return nil, fmt.Errorf("invalid orderBy field: %w", err)
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

// scanDocument parses a raw SQLite row into a typed Document.
// This is the single point where TEXT timestamps are converted to time.Time.
// GetTrustedSigner retrieves an L2 signer public key from the database.
// Implements governance.SignerStore.
func (s *GatewayDBService) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
	doc, err := s.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get trusted signer %s: %w", keyID, err)
	}
	if doc == nil {
		return nil, nil
	}

	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, err
	}

	var signer models.TrustedSigner
	if err := json.Unmarshal(data, &signer); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trusted signer: %w", err)
	}

	if !signer.Enabled {
		return nil, nil
	}

	pubBytes, err := hex.DecodeString(signer.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode public key hex: %w", err)
	}

	if len(pubBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: %d", len(pubBytes))
	}

	return ed25519.PublicKey(pubBytes), nil
}

// AddTrustedSigner adds or updates a trusted L2 signer in the database.
func (s *GatewayDBService) AddTrustedSigner(signer models.TrustedSigner) error {
	if signer.ID == "" {
		return fmt.Errorf("signer ID is required")
	}
	if signer.PublicKey == "" {
		return fmt.Errorf("signer public key is required")
	}

	if signer.AddedAt.IsZero() {
		signer.AddedAt = time.Now().UTC()
	}

	data, err := json.Marshal(signer)
	if err != nil {
		return err
	}

	return s.DocSet(marshaler.CollectionName(constants.CollectionTrustedSigners), signer.ID, data)
}

// ListTrustedSigners returns all trusted L2 signers in the database.
func (s *GatewayDBService) ListTrustedSigners() ([]models.TrustedSigner, error) {
	docs, err := s.DocQuery(marshaler.CollectionName(constants.CollectionTrustedSigners), nil, "id", 0)
	if err != nil {
		return nil, err
	}

	results := make([]models.TrustedSigner, 0, len(docs))
	for _, doc := range docs {
		data, err := json.Marshal(doc.Data)
		if err != nil {
			continue
		}
		var signer models.TrustedSigner
		if err := json.Unmarshal(data, &signer); err != nil {
			continue
		}
		// id is not in the data map usually, so we set it from doc.ID
		signer.ID = doc.ID
		results = append(results, signer)
	}
	return results, nil
}

// DeleteTrustedSigner removes a trusted L2 signer from the database.
func (s *GatewayDBService) DeleteTrustedSigner(keyID string) (bool, error) {
	return s.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), keyID)
}

// HasTrustedSigners returns true if at least one trusted L2 signer is provisioned in the database.
func (s *GatewayDBService) HasTrustedSigners() (bool, error) {
	filters := []models.DocFilter{
		{Field: "enabled", Op: "==", Value: json.RawMessage("true")},
	}
	docs, err := s.DocQuery(marshaler.CollectionName(constants.CollectionTrustedSigners), filters, "", 1)
	if err != nil {
		return false, err
	}
	return len(docs) > 0, nil
}

// GetAppPolicy retrieves an AppPolicy by app_id from the database.
// Implements governance.AppPolicyStore.
func (s *GatewayDBService) GetAppPolicy(appID string) (*models.AppPolicy, error) {
	doc, err := s.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app policy %s: %w", appID, err)
	}
	if doc == nil {
		return nil, nil
	}

	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, err
	}

	var policy models.AppPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal app policy: %w", err)
	}

	return &policy, nil
}

func scanDocument(collection, id, dataJSON, createdAtStr, updatedAtStr string) (*models.Document, error) {
	createdAt, err := sqliteutil.ParseTimestamp(createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("invalid created_at for document %s/%s: %w", collection, id, err)
	}
	updatedAt, err := sqliteutil.ParseTimestamp(updatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("invalid updated_at for document %s/%s: %w", collection, id, err)
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
		return nil, fmt.Errorf("invalid data JSON for document %s/%s: %w", collection, id, err)
	}
	if data == nil {
		data = make(map[string]json.RawMessage)
	}

	return &models.Document{
		ID:         id,
		Collection: collection,
		Data:       data,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

// =============================================================================
// KV Store with TTL
// =============================================================================

// KVGet retrieves a value by key. Returns ("", false) if not found or expired.
func (s *GatewayDBService) KVGet(key string) (string, bool) {
	// Use a single query that filters out expired keys, avoiding the need
	// for a separate lazy-delete goroutine (which risked deadlocks).
	// Expired entries are cleaned up by RunTTLCleanup instead.
	var value string
	err := s.db.QueryRowWithRetry(
		"SELECT value FROM kv_store WHERE key = ? AND (expires_at IS NULL OR expires_at > ?)",
		key, sqliteutil.NowTimestamp(),
	).Scan(&value)
	if err != nil {
		return "", false
	}
	return value, true
}

// KVSet stores a key/value pair. ttlSeconds <= 0 means no expiration.
func (s *GatewayDBService) KVSet(key, value string, ttlSeconds int) error {
	now := sqliteutil.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.ExecWithRetry(
		`INSERT INTO kv_store (key, value, created_at, expires_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		key, value, now, expiresAt,
	)
	return err
}

// KVDelete removes a key.
func (s *GatewayDBService) KVDelete(key string) error {
	_, err := s.db.ExecWithRetry("DELETE FROM kv_store WHERE key = ?", key)
	return err
}

// KVDeletePattern removes all keys matching a glob pattern (uses SQL GLOB).
func (s *GatewayDBService) KVDeletePattern(pattern string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM kv_store WHERE key GLOB ?", pattern)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// KVKeys returns all keys matching a glob pattern.
func (s *GatewayDBService) KVKeys(pattern string) ([]string, error) {
	keys, err := sqliteutil.MaterializeRows(s.db,
		"SELECT key FROM kv_store WHERE key GLOB ? AND (expires_at IS NULL OR expires_at > ?)",
		[]interface{}{pattern, sqliteutil.NowTimestamp()},
		func(r *sql.Rows) (string, error) {
			var k string
			if err := r.Scan(&k); err != nil {
				return "", err
			}
			return k, nil
		})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// KVScan returns keys matching a glob pattern using cursor-based pagination.
// cursor is a row offset (0 = start). count is the page size (default 100).
// Returns (nextCursor, keys, error). nextCursor == 0 means scan is complete.
func (s *GatewayDBService) KVScan(pattern string, cursor, count int) (int, []string, error) {
	if count <= 0 {
		count = 100
	}
	// Fetch count+1 to detect whether a next page exists
	keys, err := sqliteutil.MaterializeRows(s.db,
		"SELECT key FROM kv_store WHERE key GLOB ? AND (expires_at IS NULL OR expires_at > ?) ORDER BY key LIMIT ? OFFSET ?",
		[]interface{}{pattern, sqliteutil.NowTimestamp(), count + 1, cursor},
		func(r *sql.Rows) (string, error) {
			var k string
			if err := r.Scan(&k); err != nil {
				return "", err
			}
			return k, nil
		})
	if err != nil {
		return 0, nil, err
	}

	if len(keys) > count {
		return cursor + count, keys[:count], nil
	}
	return 0, keys, nil
}

// KVExists checks if a key exists and is not expired.
func (s *GatewayDBService) KVExists(key string) bool {
	_, found := s.KVGet(key)
	return found
}

// KVTTL returns the remaining TTL in seconds for a key. -1 if no expiry, -2 if not found.
func (s *GatewayDBService) KVTTL(key string) int {
	var expiresAt sql.NullString
	err := s.db.QueryRowWithRetry(
		"SELECT expires_at FROM kv_store WHERE key = ?", key,
	).Scan(&expiresAt)
	if err != nil {
		return -2
	}
	if !expiresAt.Valid {
		return -1
	}
	exp, err := time.Parse(time.RFC3339Nano, expiresAt.String)
	if err != nil {
		return -2
	}
	remaining := int(time.Until(exp).Seconds())
	if remaining < 0 {
		return -2
	}
	return remaining
}

// KVExpire sets a TTL on an existing key. Returns false if key not found.
func (s *GatewayDBService) KVExpire(key string, ttlSeconds int) bool {
	exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
	result, err := s.db.ExecWithRetry(
		"UPDATE kv_store SET expires_at = ? WHERE key = ?", exp, key,
	)
	if err != nil {
		return false
	}
	n, _ := result.RowsAffected()
	return n > 0
}

// SSERoute is the routing target for an SSE event row. Exactly one of the
// three id fields MUST be non-empty. The Gateway refuses to talk about a
// bare session id - every routing key is tagged at the type level so a
// web_session_id can never be mis-delivered as a cli_session_id (or vice
// versa) and a user_id (background fan-out) can never be mistaken for a
// per-session id.
type SSERoute struct {
	WebSessionID string
	CLISessionID string
	UserID       string
}

// validate ensures exactly one routing id is set.
func (r SSERoute) validate() error {
	n := 0
	if r.WebSessionID != "" {
		n++
	}
	if r.CLISessionID != "" {
		n++
	}
	if r.UserID != "" {
		n++
	}
	switch n {
	case 0:
		return fmt.Errorf("sse route requires exactly one of web_session_id, cli_session_id, user_id")
	case 1:
		return nil
	default:
		return fmt.Errorf("sse route is mutually-exclusive: set exactly one of web_session_id, cli_session_id, user_id")
	}
}

// SSEEventsAppend inserts a row into the sse_events table. The route MUST set
// exactly one of WebSessionID, CLISessionID, UserID. The producer_id is the
// app identity (SPIFFE ID) that produced the event for attribution.
func (s *GatewayDBService) SSEEventsAppend(route SSERoute, eventType, payload, producerID string) error {
	if err := route.validate(); err != nil {
		return err
	}
	now := sqliteutil.NowTimestamp()
	_, err := s.db.ExecWithRetry(
		"INSERT INTO sse_events (web_session_id, cli_session_id, user_id, event_type, payload, producer_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		nullIfEmpty(route.WebSessionID), nullIfEmpty(route.CLISessionID), nullIfEmpty(route.UserID), eventType, payload, nullIfEmpty(producerID), now,
	)
	return err
}

// nullIfEmpty returns sql.NullString{Valid: false} for empty strings so the
// CHECK constraint on sse_events sees a NULL rather than an empty string.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// SSEEventsWipe deletes all rows from the sse_events table. Returns the number of rows deleted.
func (s *GatewayDBService) SSEEventsWipe() (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM sse_events")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SSEEventsCount returns the total number of rows in the sse_events table.
func (s *GatewayDBService) SSEEventsCount() (int64, error) {
	var count int64
	err := s.db.QueryRowWithRetry("SELECT COUNT(*) FROM sse_events").Scan(&count)
	return count, err
}

// SSEEventsListSince returns up to `limit` events with id > sinceID, ordered by
// id ascending. The route MUST set exactly one of WebSessionID, CLISessionID,
// UserID. SSEEventsListAllSince is the admin-only "all routes" variant.
func (s *GatewayDBService) SSEEventsListSince(route SSERoute, sinceID int64, limit int) ([]models.SSEEventRow, error) {
	if err := route.validate(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var query string
	var args []interface{}
	switch {
	case route.WebSessionID != "":
		query = "SELECT id, web_session_id, cli_session_id, user_id, event_type, payload, created_at FROM sse_events WHERE web_session_id = ? AND id > ? ORDER BY id ASC LIMIT ?"
		args = []interface{}{route.WebSessionID, sinceID, limit}
	case route.CLISessionID != "":
		query = "SELECT id, web_session_id, cli_session_id, user_id, event_type, payload, created_at FROM sse_events WHERE cli_session_id = ? AND id > ? ORDER BY id ASC LIMIT ?"
		args = []interface{}{route.CLISessionID, sinceID, limit}
	default:
		query = "SELECT id, web_session_id, cli_session_id, user_id, event_type, payload, created_at FROM sse_events WHERE user_id = ? AND id > ? ORDER BY id ASC LIMIT ?"
		args = []interface{}{route.UserID, sinceID, limit}
	}

	return sqliteutil.MaterializeRows(s.db, query, args, func(r *sql.Rows) (models.SSEEventRow, error) {
		var row models.SSEEventRow
		var web, cli, user sql.NullString
		if err := r.Scan(&row.ID, &web, &cli, &user, &row.EventType, &row.Payload, &row.CreatedAt); err != nil {
			return models.SSEEventRow{}, err
		}
		row.WebSessionID = web.String
		row.CLISessionID = cli.String
		row.UserID = user.String
		return row, nil
	})
}

// SSEEventsListAllSince is an admin/debug helper that returns events across
// every routing target with id > sinceID. Production paths MUST use
// SSEEventsListSince with a typed route.
func (s *GatewayDBService) SSEEventsListAllSince(sinceID int64, limit int) ([]models.SSEEventRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	return sqliteutil.MaterializeRows(s.db,
		"SELECT id, web_session_id, cli_session_id, user_id, event_type, payload, created_at FROM sse_events WHERE id > ? ORDER BY id ASC LIMIT ?",
		[]interface{}{sinceID, limit},
		func(r *sql.Rows) (models.SSEEventRow, error) {
			var row models.SSEEventRow
			var web, cli, user sql.NullString
			if err := r.Scan(&row.ID, &web, &cli, &user, &row.EventType, &row.Payload, &row.CreatedAt); err != nil {
				return models.SSEEventRow{}, err
			}
			row.WebSessionID = web.String
			row.CLISessionID = cli.String
			row.UserID = user.String
			return row, nil
		})
}

// RunTTLCleanup periodically removes expired KV entries and expired blobs.
func (s *GatewayDBService) RunTTLCleanup(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := sqliteutil.NowTimestamp()
			_, _ = s.db.ExecWithRetry("DELETE FROM kv_store WHERE expires_at IS NOT NULL AND expires_at < ?", now)
			_, _ = s.db.ExecWithRetry("DELETE FROM blobs WHERE expires_at IS NOT NULL AND expires_at < ?", now)
		}
	}
}

// =============================================================================
// Blob Store - raw binary storage keyed by namespace + id
// =============================================================================

// BlobRecord is the metadata returned for a stored blob (data excluded).
type BlobRecord struct {
	ID          string
	Namespace   string
	Size        int64
	ContentType string
	CreatedAt   time.Time
}

// BlobPut stores raw bytes under namespace/id. ttlSeconds == 0 means no expiration.
// Negative ttlSeconds means the blob is immediately expired (will never be returned by BlobGet).
// An existing blob at the same namespace/id is replaced.
func (s *GatewayDBService) BlobPut(namespace, id string, data []byte, contentType string, ttlSeconds int) error {
	now := sqliteutil.NowTimestamp()
	var expiresAt *string
	if ttlSeconds > 0 {
		exp := sqliteutil.FormatTimestamp(time.Now().Add(time.Duration(ttlSeconds) * time.Second))
		expiresAt = &exp
	} else if ttlSeconds < 0 {
		exp := sqliteutil.FormatTimestamp(time.Now().Add(-1 * time.Second))
		expiresAt = &exp
	}

	_, err := s.db.ExecWithRetry(
		`INSERT INTO blobs (namespace, id, size, content_type, data, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(namespace, id) DO UPDATE SET
		   size         = excluded.size,
		   content_type = excluded.content_type,
		   data         = excluded.data,
		   expires_at   = excluded.expires_at`,
		namespace, id, int64(len(data)), contentType, data, now, expiresAt,
	)
	return err
}

// BlobGet retrieves the raw bytes and content type for a blob.
// Returns (nil, "", false) if not found or expired.
func (s *GatewayDBService) BlobGet(namespace, id string) ([]byte, string, bool) {
	var data []byte
	var contentType string
	err := s.db.QueryRowWithRetry(
		"SELECT data, content_type FROM blobs WHERE namespace = ? AND id = ? AND (expires_at IS NULL OR expires_at > ?)",
		namespace, id, sqliteutil.NowTimestamp(),
	).Scan(&data, &contentType)
	if err != nil {
		return nil, "", false
	}
	return data, contentType, true
}

// BlobMeta retrieves metadata for a blob without loading the data.
// Returns (nil, false) if not found or expired.
func (s *GatewayDBService) BlobMeta(namespace, id string) (*BlobRecord, bool) {
	var rec BlobRecord
	var createdAtStr string
	err := s.db.QueryRowWithRetry(
		"SELECT id, namespace, size, content_type, created_at FROM blobs WHERE namespace = ? AND id = ? AND (expires_at IS NULL OR expires_at > ?)",
		namespace, id, sqliteutil.NowTimestamp(),
	).Scan(&rec.ID, &rec.Namespace, &rec.Size, &rec.ContentType, &createdAtStr)
	if err != nil {
		return nil, false
	}
	t, err := sqliteutil.ParseTimestamp(createdAtStr)
	if err != nil {
		return nil, false
	}
	rec.CreatedAt = t
	return &rec, true
}

// BlobDelete removes a single blob. Returns (true, nil) if deleted, (false, nil) if not found.
func (s *GatewayDBService) BlobDelete(namespace, id string) (bool, error) {
	result, err := s.db.ExecWithRetry(
		"DELETE FROM blobs WHERE namespace = ? AND id = ?",
		namespace, id,
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

// BlobDeleteNamespace removes all blobs under a namespace.
// Returns the count of deleted blobs.
func (s *GatewayDBService) BlobDeleteNamespace(namespace string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM blobs WHERE namespace = ?", namespace)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// =============================================================================
// Suspended Transactions - L3 approval queue
// =============================================================================

// StoreSuspendedTransaction stores a transaction awaiting L3 approval.
func (s *GatewayDBService) StoreSuspendedTransaction(tx *models.SuspendedTransaction) error {
	now := sqliteutil.FormatTimestamp(tx.CreatedAt)
	expires := sqliteutil.FormatTimestamp(tx.ExpiresAt)

	var toolArgsStr string
	if tx.ToolArguments != nil {
		toolArgsStr = string(tx.ToolArguments)
	}

	var approvedAtStr *string
	if tx.ApprovedAt != nil {
		ts := sqliteutil.FormatTimestamp(*tx.ApprovedAt)
		approvedAtStr = &ts
	}

	_, err := s.db.ExecWithRetry(
		`INSERT INTO suspended_transactions 
		 (transaction_hash, envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id, approved, approved_at, approved_by, approval_signature, expected_cert_fingerprint)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(transaction_hash) DO UPDATE SET
		   envelope = excluded.envelope,
		   expires_at = excluded.expires_at,
		   approved = excluded.approved,
		   approved_at = excluded.approved_at,
		   approved_by = excluded.approved_by,
		   approval_signature = excluded.approval_signature,
		   expected_cert_fingerprint = excluded.expected_cert_fingerprint`,
		tx.TransactionHash, string(tx.Envelope), now, expires, tx.ToolName, toolArgsStr, tx.UserID, tx.OperatorID,
		tx.Approved, approvedAtStr, tx.ApprovedBy, tx.ApprovalSignature, tx.ExpectedCertFingerprint,
	)
	return err
}

// GetSuspendedTransaction retrieves a suspended transaction by hash.
// Returns (nil, false) if not found or expired.
func (s *GatewayDBService) GetSuspendedTransaction(txHash string) (*models.SuspendedTransaction, bool) {
	var envelopeStr, createdAtStr, expiresAtStr, toolName, toolArgsStr, userID, operatorID, approvedBy, approvalSignature, expectedCertFingerprint string
	var approved int
	var approvedAtStr sql.NullString
	err := s.db.QueryRowWithRetry(
		"SELECT envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id, approved, approved_at, approved_by, approval_signature, expected_cert_fingerprint FROM suspended_transactions WHERE transaction_hash = ? AND expires_at > ?",
		txHash, sqliteutil.NowTimestamp(),
	).Scan(&envelopeStr, &createdAtStr, &expiresAtStr, &toolName, &toolArgsStr, &userID, &operatorID, &approved, &approvedAtStr, &approvedBy, &approvalSignature, &expectedCertFingerprint)
	if err != nil {
		return nil, false
	}

	createdAt, err := sqliteutil.ParseTimestamp(createdAtStr)
	if err != nil {
		return nil, false
	}
	expiresAt, err := sqliteutil.ParseTimestamp(expiresAtStr)
	if err != nil {
		return nil, false
	}

	var toolArgs json.RawMessage
	if toolArgsStr != "" {
		toolArgs = json.RawMessage(toolArgsStr)
	}

	var approvedAt *time.Time
	if approvedAtStr.Valid {
		ts, err := sqliteutil.ParseTimestamp(approvedAtStr.String)
		if err != nil {
			return nil, false
		}
		approvedAt = &ts
	}

	return &models.SuspendedTransaction{
		TransactionHash:         txHash,
		Envelope:                json.RawMessage(envelopeStr),
		CreatedAt:               createdAt,
		ExpiresAt:               expiresAt,
		ToolName:                toolName,
		ToolArguments:           toolArgs,
		UserID:                  userID,
		OperatorID:              operatorID,
		Approved:                approved == 1,
		ApprovedAt:              approvedAt,
		ApprovedBy:              approvedBy,
		ApprovalSignature:       approvalSignature,
		ExpectedCertFingerprint: expectedCertFingerprint,
	}, true
}

// ListSuspendedTransactions retrieves all non-expired suspended transactions.
// Optionally filters by user_id if provided.
func (s *GatewayDBService) ListSuspendedTransactions(userID string) ([]*models.SuspendedTransaction, error) {
	var query string
	var args []interface{}

	if userID != "" {
		query = "SELECT transaction_hash, envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id FROM suspended_transactions WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC"
		args = []interface{}{userID, sqliteutil.NowTimestamp()}
	} else {
		query = "SELECT transaction_hash, envelope, created_at, expires_at, tool_name, tool_arguments, user_id, operator_id FROM suspended_transactions WHERE expires_at > ? ORDER BY created_at DESC"
		args = []interface{}{sqliteutil.NowTimestamp()}
	}

	type suspendedTxRow struct {
		txHash       string
		envelopeStr  string
		createdAtStr string
		expiresAtStr string
		toolName     string
		toolArgsStr  string
		userID       string
		operatorID   string
	}

	rows, err := sqliteutil.MaterializeRows(s.db, query, args, func(r *sql.Rows) (suspendedTxRow, error) {
		var row suspendedTxRow
		if err := r.Scan(&row.txHash, &row.envelopeStr, &row.createdAtStr, &row.expiresAtStr, &row.toolName, &row.toolArgsStr, &row.userID, &row.operatorID); err != nil {
			return suspendedTxRow{}, err
		}
		return row, nil
	})
	if err != nil {
		return nil, err
	}

	transactions := make([]*models.SuspendedTransaction, 0, len(rows))
	for _, row := range rows {
		createdAt, err := sqliteutil.ParseTimestamp(row.createdAtStr)
		if err != nil {
			continue
		}
		expiresAt, err := sqliteutil.ParseTimestamp(row.expiresAtStr)
		if err != nil {
			continue
		}

		var toolArgs json.RawMessage
		if row.toolArgsStr != "" {
			toolArgs = json.RawMessage(row.toolArgsStr)
		}

		transactions = append(transactions, &models.SuspendedTransaction{
			TransactionHash: row.txHash,
			Envelope:        json.RawMessage(row.envelopeStr),
			CreatedAt:       createdAt,
			ExpiresAt:       expiresAt,
			ToolName:        row.toolName,
			ToolArguments:   toolArgs,
			UserID:          row.userID,
			OperatorID:      row.operatorID,
		})
	}

	return transactions, nil
}

// ApproveSuspendedTransaction marks a suspended transaction as approved with cryptographic signature.
// This is called by the CLI approval command when a human approves a transaction.
func (s *GatewayDBService) ApproveSuspendedTransaction(txHash, approvedBy, approvalSignature, expectedCertFingerprint string) error {
	now := time.Now().UTC()
	nowStr := sqliteutil.FormatTimestamp(now)

	result, err := s.db.ExecWithRetry(
		`UPDATE suspended_transactions 
		 SET approved = 1, approved_at = ?, approved_by = ?, approval_signature = ?, expected_cert_fingerprint = ?
		 WHERE transaction_hash = ? AND expires_at > ?`,
		nowStr, approvedBy, approvalSignature, expectedCertFingerprint, txHash, sqliteutil.NowTimestamp(),
	)
	if err != nil {
		return fmt.Errorf("failed to approve suspended transaction: %w", err)
	}

	// Check if any row was actually updated
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("transaction not found or expired")
	}

	return nil
}

// DeleteSuspendedTransaction removes a suspended transaction after approval/rejection.
func (s *GatewayDBService) DeleteSuspendedTransaction(txHash string) error {
	_, err := s.db.ExecWithRetry("DELETE FROM suspended_transactions WHERE transaction_hash = ?", txHash)
	return err
}

// CleanupExpiredSuspendedTransactions removes expired suspended transactions.
// Returns the count of deleted transactions.
func (s *GatewayDBService) CleanupExpiredSuspendedTransactions() (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM suspended_transactions WHERE expires_at < ?", sqliteutil.NowTimestamp())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
