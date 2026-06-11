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
// Callers should use the extracted service fields directly (e.g., db.DocStore.DocGet()).
type CanonicalDBService struct {
	db         *sqliteutil.DB
	logger     *slog.Logger
	AuditStore *storage.SQLAuditStore
	vault      *vault.Vault

	// Extracted services - initialized in OpenCanonicalDBService
	DocStore        *DocumentStoreService
	AppPolicyStore  *AppPolicyStoreService
	SignerStore     *SignerStoreService
	StateRootSvc    *StateRootService
	ReplayStore     *ReplayStoreService
	KVStore         *KVStoreService
	SSEStore        *SSEEventService
	BlobStore       *BlobStoreService

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
	svc.DocStore = NewDocumentStoreService(db, logger)
	svc.AppPolicyStore = NewAppPolicyStoreService(db, logger)
	svc.SignerStore = NewSignerStoreService(db, logger)
	svc.StateRootSvc = NewStateRootService(db, logger)
	svc.ReplayStore = NewReplayStoreService(db, logger)
	svc.KVStore = NewKVStoreService(db, logger)
	svc.SSEStore = NewSSEEventService(db, logger)
	svc.BlobStore = NewBlobStoreService(db, logger)

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
		root, err := s.StateRootSvc.CalculateStateRoot()
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

