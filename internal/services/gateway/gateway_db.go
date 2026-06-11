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

// Package gateway provides services for gateway mode (operator platform mode).
//
// This package contains mode-specific services that are only used in gateway mode,
// including GatewayModeService (the top-level orchestrator) and CanonicalDBService
// (shared with outbound mode for state root calculation).
//
// For more information on service modes, see docs/architecture/service_modes.md.
package gateway

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	_ "embed"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
)

// gatewaySchema is the canonical Operator SQLite schema, embedded at compile time
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

// CanonicalDBService provides the unified SQLite persistence layer for gateway mode.
// Three subsystems:
//   - Document store: collection/id based CRUD (replaces client+agent separate SQLite DBs)
//   - KV store with TTL: key/value with optional expiration
//   - SSE event buffer: per-session event ring buffer
//
// This service is used in both gateway mode (full database service) and outbound mode
// (state root calculation only).
//
// The service delegates domain logic to extracted single-responsibility services.
// These delegation wrappers maintain backward compatibility while allowing
// gradual migration to direct service usage.
type CanonicalDBService struct {
	db         *sqliteutil.DB
	logger     *slog.Logger
	AuditStore *storage.SQLAuditStore
	vault      *vault.Vault

	// Extracted services - initialized in OpenCanonicalDBService
	docStore        *DocumentStoreService
	appPolicyStore  *AppPolicyStoreService
	signerStore     *SignerStoreService
	stateRootSvc    *StateRootService
	replayStore     *ReplayStoreService

	// Shutdown tracking
	mu      sync.Mutex
	running bool
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc

	// State root caching to avoid full table scans
	cachedStateRoot    string
	cachedStateVersion int64
}

// OpenCanonicalDBService opens (or creates) the unified SQLite database.
// testMode enables the in-memory keystore backend for unit tests.
// vaultKeyPath is the path to the vault private key file (hex-encoded).
// vaultRequireUnlock requires the vault to be unlocked before starting.
func OpenCanonicalDBService(dataDir string, secretsDir string, vaultDir string, logger *slog.Logger, testMode bool, vaultKeyPath string, vaultRequireUnlock bool) (*CanonicalDBService, error) {
	dbPath := filepath.Join(dataDir, "g8e.db")
	cfg := sqliteutil.DefaultDBConfig(dbPath)

	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open gateway database: %w", err)
	}

	vaultConfig := &vault.VaultConfig{
		DataDir: vaultDir,
		Logger:  logger,
	}
	encryptionVault, err := vault.NewVault(vaultConfig)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize vault: %w", err)
	}

	// Unlock vault before initializing storage services
	// Encryption is required for secure data storage at rest
	if vaultKeyPath == "" && vaultRequireUnlock {
		vaultKeyPath = filepath.Join(vaultDir, "key")
	}

	if vaultKeyPath != "" {
		if !filepath.IsAbs(vaultKeyPath) {
			vaultKeyPath = filepath.Join(dataDir, vaultKeyPath)
		}

		privateKey, err := vault.ReadVaultKey(vaultKeyPath)
		if err != nil {
			if vaultRequireUnlock {
				db.Close()
				return nil, fmt.Errorf("failed to read vault key: %w", err)
			}
			logger.Info("Vault key not found, vault will remain locked", "path", vaultKeyPath, "error", err)
		} else {
			defer vault.SecureZero(privateKey)

			if err := encryptionVault.Unlock(privateKey); err != nil {
				if vaultRequireUnlock {
					db.Close()
					if errors.Is(err, vault.ErrVaultNotInit) {
						return nil, fmt.Errorf("vault not initialized at %s. Run 'g8e vault init' first", vaultDir)
					}
					if errors.Is(err, vault.ErrInvalidPrivateKey) {
						return nil, fmt.Errorf("invalid vault key at %s. Verify the key file is correct", vaultKeyPath)
					}
					return nil, fmt.Errorf("failed to unlock vault: %w", err)
				}
				logger.Info("Failed to unlock vault, vault will remain locked", "error", err)
			} else {
				logger.Info("Vault unlocked successfully", "vault_dir", vaultDir)
			}
		}
	} else {
		logger.Info("No vault key provided, vault will remain locked")
	}

	// Initialize SQLAuditStore for transaction-native audit recording
	auditStoreConfig := storage.DefaultAuditStoreConfig()
	auditStoreConfig.DataDir = dataDir
	auditStoreConfig.EncryptionVault = encryptionVault
	auditStore, err := storage.NewSQLAuditStore(auditStoreConfig, logger)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize audit store: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	svc := &CanonicalDBService{
		db:         db,
		logger:     logger,
		AuditStore: auditStore,
		vault:      encryptionVault,
		ctx:        ctx,
		cancel:     cancel,
		running:    true,
	}

	// Initialize extracted services with the same db connection
	svc.docStore = NewDocumentStoreService(db, logger)
	svc.appPolicyStore = NewAppPolicyStoreService(db, logger)
	svc.signerStore = NewSignerStoreService(db, logger)
	svc.stateRootSvc = NewStateRootService(db, logger)
	svc.replayStore = NewReplayStoreService(db, logger)

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

func (s *CanonicalDBService) initTestSchema(secretsDir string) error {
	_, err := s.db.ExecWithRetry(gatewaySchema)
	if err != nil {
		return err
	}
	// Migration: Add producer_id column to sse_events table if it doesn't exist
	_, err = s.db.ExecWithRetry("ALTER TABLE sse_events ADD COLUMN producer_id TEXT")
	if err != nil && !errors.Is(err, constants.ErrDuplicateColumn) && !sqliteutil.IsDuplicateColumnError(err) {
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
	if err := sm.InitAppSettings(); err != nil {
		return err
	}

	// Migration: Initialize state_version table for existing databases
	if err := s.migrateStateVersion(); err != nil {
		return err
	}

	return nil
}

func (s *CanonicalDBService) initStateRoot() error {
	var count int
	err := s.db.QueryRowWithRetry("SELECT COUNT(*) FROM state_root").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		root, err := s.stateRootSvc.CalculateStateRoot()
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
// Delegates to StateRootService.
func (s *CanonicalDBService) GetCurrentStateRoot() (string, error) {
	return s.stateRootSvc.GetCurrentStateRoot()
}

// ReserveNonce atomically reserves a nonce for early replay protection.
// Delegates to ReplayStoreService.
func (s *CanonicalDBService) ReserveNonce(nonce string, expiresAt time.Time) (bool, error) {
	return s.replayStore.ReserveNonce(nonce, expiresAt)
}

// FinalizeNonce marks a reserved nonce as fully consumed.
// Delegates to ReplayStoreService.
func (s *CanonicalDBService) FinalizeNonce(nonce string) error {
	return s.replayStore.FinalizeNonce(nonce)
}

// ReleaseNonce removes a reservation for a failed transaction.
// Delegates to ReplayStoreService.
func (s *CanonicalDBService) ReleaseNonce(nonce string) error {
	return s.replayStore.ReleaseNonce(nonce)
}

// RunMaintenance periodically removes expired entries.
func (s *CanonicalDBService) RunMaintenance(ctx context.Context) {
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
		}
	}
}

func (s *CanonicalDBService) initSchema(secretsDir string) error {
	_, err := s.db.ExecWithRetry(gatewaySchema)
	if err != nil {
		return err
	}

	// Migration: Add producer_id column to sse_events table if it doesn't exist
	_, err = s.db.ExecWithRetry("ALTER TABLE sse_events ADD COLUMN producer_id TEXT")
	if err != nil && !errors.Is(err, constants.ErrDuplicateColumn) && !sqliteutil.IsDuplicateColumnError(err) {
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

	// Migration: Initialize state_version table for existing databases
	if err := s.migrateStateVersion(); err != nil {
		return err
	}

	return nil
}

// migratePlaintextServiceKeys moves existing plaintext service certificate private keys
// to the keystore and deletes the plaintext files. This is a one-time migration.
func (s *CanonicalDBService) migratePlaintextServiceKeys(secretsDir string, sm *SecretManager) error {
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

// migrateStateVersion initializes the state_version table for existing databases.
// This is a one-time migration for databases created before the change tracking feature.
func (s *CanonicalDBService) migrateStateVersion() error {
	// Check if state_version table exists and has a row
	var count int
	err := s.db.QueryRowWithRetry("SELECT COUNT(*) FROM state_version").Scan(&count)
	if err != nil {
		// Table doesn't exist, will be created by schema
		return nil
	}
	if count > 0 {
		// Already initialized
		return nil
	}

	// Table exists but is empty, initialize it
	_, err = s.db.ExecWithRetry("INSERT INTO state_version (id, version) VALUES (1, 0)")
	if err != nil && !errors.Is(err, constants.ErrAlreadyExists) && !sqliteutil.IsUniqueConstraintError(err) {
		s.logger.Warn("Failed to initialize state_version", "error", err)
	}
	return nil
}

// GetDB returns the underlying SQLite database connection.
func (s *CanonicalDBService) GetDB() *sqliteutil.DB {
	return s.db
}

func (s *CanonicalDBService) GetVault() *vault.Vault {
	return s.vault
}

// Close closes the database connection and waits for background workers.
func (s *CanonicalDBService) Close() error {
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
		s.logger.Warn("CanonicalDBService shutdown timeout, forcing close")
	}

	if s.AuditStore != nil {
		if err := s.AuditStore.Close(); err != nil {
			s.logger.Error("AuditStore close error", "error", err)
		}
	}

	if err := s.db.Close(); err != nil {
		s.logger.Error("Database close error", "error", err)
		return err
	}
	return nil
}

// Wait blocks until all background workers have finished.
func (s *CanonicalDBService) Wait() {
	s.wg.Wait()
}

// =============================================================================
// Document Store - collection/id based CRUD (delegates to DocumentStoreService)
// =============================================================================

// DocGet retrieves a document by collection and id.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) DocGet(collection, id string) (*models.Document, error) {
	return s.docStore.DocGet(collection, id)
}

// DocCreate creates a document only if it does not already exist.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) DocCreate(collection, id string, data json.RawMessage) error {
	return s.docStore.DocCreate(collection, id, data)
}

// DocSet creates or replaces a document.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) DocSet(collection, id string, data json.RawMessage) error {
	return s.docStore.DocSet(collection, id, data)
}

// DocSetWithTimestamps creates or replaces a document with custom timestamps.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) DocSetWithTimestamps(collection, id string, data json.RawMessage, createdAt, updatedAt time.Time) error {
	return s.docStore.DocSetWithTimestamps(collection, id, data, createdAt, updatedAt)
}

// DocUpdate merges fields into an existing document.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) DocUpdate(collection, id string, fields json.RawMessage) (*models.Document, error) {
	return s.docStore.DocUpdate(collection, id, fields)
}

// DocDelete removes a document.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) DocDelete(collection, id string) (bool, error) {
	return s.docStore.DocDelete(collection, id)
}

// DocDeleteNamespace removes all documents in a collection.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) DocDeleteNamespace(collection string) (int64, error) {
	return s.docStore.DocDeleteNamespace(collection)
}

// GetField extracts a single field value from a document using dot notation.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) GetField(collection, id, fieldPath string) (interface{}, error) {
	// DocumentStoreService returns json.RawMessage, but CanonicalDBService returns interface{}
	// for backward compatibility. Convert here.
	result, err := s.docStore.GetField(collection, id, fieldPath)
	if err != nil {
		return nil, err
	}

	// SQLite's json_extract returns SQL literals (true, false, null) as raw strings.
	// Convert these to proper JSON before unmarshaling.
	resultStr := string(result)
	switch resultStr {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	}

	// Try to unmarshal to a Go type
	var unmarshaled interface{}
	if err := json.Unmarshal(result, &unmarshaled); err != nil {
		// If unmarshaling fails, return the raw string (matches original behavior)
		return resultStr, nil
	}
	return unmarshaled, nil
}

// DocQuery returns documents matching field conditions.
// Delegates to DocumentStoreService.
func (s *CanonicalDBService) DocQuery(collection string, filters []models.DocFilter, orderBy string, limit int) ([]*models.Document, error) {
	return s.docStore.DocQuery(collection, filters, orderBy, limit)
}

// scanDocument parses a raw SQLite row into a typed Document.
// This is the single point where TEXT timestamps are converted to time.Time.
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
// Signer Store - trusted L2 signer management (delegates to SignerStoreService)
// =============================================================================

// GetTrustedSigner retrieves an L2 signer public key from the database.
// Delegates to SignerStoreService.
func (s *CanonicalDBService) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
	return s.signerStore.GetTrustedSigner(keyID)
}

// AddTrustedSigner adds or updates a trusted L2 signer in the database.
// Delegates to SignerStoreService.
func (s *CanonicalDBService) AddTrustedSigner(signer models.TrustedSigner) error {
	return s.signerStore.AddTrustedSigner(signer)
}

// ListTrustedSigners returns all trusted L2 signers in the database.
// Delegates to SignerStoreService.
func (s *CanonicalDBService) ListTrustedSigners() ([]models.TrustedSigner, error) {
	return s.signerStore.ListTrustedSigners()
}

// DeleteTrustedSigner removes a trusted L2 signer from the database.
// Delegates to SignerStoreService.
func (s *CanonicalDBService) DeleteTrustedSigner(keyID string) (bool, error) {
	return s.signerStore.DeleteTrustedSigner(keyID)
}

// HasTrustedSigners returns true if at least one trusted L2 signer is provisioned.
// Delegates to SignerStoreService.
func (s *CanonicalDBService) HasTrustedSigners() (bool, error) {
	return s.signerStore.HasTrustedSigners()
}

// =============================================================================
// App Policy Store - app policy retrieval (delegates to AppPolicyStoreService)
// =============================================================================

// GetAppPolicy retrieves an AppPolicy by app_id from the database.
// Delegates to AppPolicyStoreService.
func (s *CanonicalDBService) GetAppPolicy(appID string) (*models.AppPolicy, error) {
	return s.appPolicyStore.GetAppPolicy(appID)
}

// =============================================================================
// KV Store with TTL
// =============================================================================

// KVGet retrieves a value by key. Returns ("", false) if not found or expired.
func (s *CanonicalDBService) KVGet(key string) (string, bool) {
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
func (s *CanonicalDBService) KVSet(key, value string, ttlSeconds int) error {
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
func (s *CanonicalDBService) KVDelete(key string) error {
	_, err := s.db.ExecWithRetry("DELETE FROM kv_store WHERE key = ?", key)
	return err
}

// KVDeletePattern removes all keys matching a glob pattern (uses SQL GLOB).
func (s *CanonicalDBService) KVDeletePattern(pattern string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM kv_store WHERE key GLOB ?", pattern)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// KVKeys returns all keys matching a glob pattern.
func (s *CanonicalDBService) KVKeys(pattern string) ([]string, error) {
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
func (s *CanonicalDBService) KVScan(pattern string, cursor, count int) (int, []string, error) {
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
func (s *CanonicalDBService) KVExists(key string) bool {
	_, found := s.KVGet(key)
	return found
}

// KVTTL returns the remaining TTL in seconds for a key. -1 if no expiry, -2 if not found.
func (s *CanonicalDBService) KVTTL(key string) int {
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
func (s *CanonicalDBService) KVExpire(key string, ttlSeconds int) bool {
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
func (s *CanonicalDBService) SSEEventsAppend(route SSERoute, eventType, payload, producerID string) error {
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
func (s *CanonicalDBService) SSEEventsWipe() (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM sse_events")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SSEEventsCount returns the total number of rows in the sse_events table.
func (s *CanonicalDBService) SSEEventsCount() (int64, error) {
	var count int64
	err := s.db.QueryRowWithRetry("SELECT COUNT(*) FROM sse_events").Scan(&count)
	return count, err
}

// SSEEventsListSince returns up to `limit` events with id > sinceID, ordered by
// id ascending. The route MUST set exactly one of WebSessionID, CLISessionID,
// UserID. SSEEventsListAllSince is the admin-only "all routes" variant.
func (s *CanonicalDBService) SSEEventsListSince(route SSERoute, sinceID int64, limit int) ([]models.SSEEventRow, error) {
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
func (s *CanonicalDBService) SSEEventsListAllSince(sinceID int64, limit int) ([]models.SSEEventRow, error) {
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
func (s *CanonicalDBService) RunTTLCleanup(ctx context.Context) {
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
func (s *CanonicalDBService) BlobPut(namespace, id string, data []byte, contentType string, ttlSeconds int) error {
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
func (s *CanonicalDBService) BlobGet(namespace, id string) ([]byte, string, bool) {
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
func (s *CanonicalDBService) BlobMeta(namespace, id string) (*BlobRecord, bool) {
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
func (s *CanonicalDBService) BlobDelete(namespace, id string) (bool, error) {
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
func (s *CanonicalDBService) BlobDeleteNamespace(namespace string) (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM blobs WHERE namespace = ?", namespace)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
