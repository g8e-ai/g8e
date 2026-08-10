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
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/vault"
	"github.com/g8e-ai/g8e/internal/timesvc"
)

// gatewaySchema is the canonical Operator SQLite schema, embedded at compile time
// from `schema.sql`. That file is the single source of truth - do not inline
// CREATE TABLE statements in Go code.
//
//go:embed db/schema.sql
var gatewaySchema string

// Stores holds the extracted single-responsibility store services.
// It is returned by OpenCanonicalDBService and passed to consumers that
// need specific stores. CanonicalDBService retains a private reference for
// lifecycle management (maintenance, close).
type Stores struct {
	DB             *sqliteutil.DB
	DocStore       *DocumentStoreService
	AppPolicyStore *AppPolicyStoreService
	SignerStore    *SignerStoreService
	ConsensusStore *ConsensusStoreService
	StateRootSvc   *StateRootService
	ReplayStore    *ReplayStoreService
	KVStore        *KVStoreService
	SSEStore       *SSEEventService
	BlobStore      *BlobStoreService
	AuditStore     *storage.SQLAuditStore
}

// CanonicalDBService manages the lifecycle of the unified SQLite persistence
// layer for gateway mode. It owns the database connection, vault, secret
// manager, and background maintenance. Domain logic is delegated to the
// extracted store services in Stores, which are returned separately by
// OpenCanonicalDBService for injection into consumers.
//
// This service is used in both gateway mode (full database service) and
// outbound mode (state root calculation only).
type CanonicalDBService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
	vault  *vault.Vault
	sm     *SecretManager

	// stores holds the extracted services for lifecycle management (maintenance, close).
	stores *Stores

	// Shutdown tracking
	mu      sync.Mutex
	running bool
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// OpenCanonicalDBService opens (or creates) the unified SQLite database.
// vaultKeyPath is the path to the vault private key file (hex-encoded).
// ks is an optional pre-initialized keystore (non-nil for tests to bypass OS keychain,
// nil for production which creates via OS keychain).
func OpenCanonicalDBService(dataDir string, vaultDir string, logger *slog.Logger, vaultKeyPath string, ks *keystore.Keystore, fileSvc fs.RuntimeFileService) (*CanonicalDBService, *Stores, error) {
	dbPath := filepath.Join(dataDir, constants.DbFilename)
	cfg := sqliteutil.DefaultDBConfig(dbPath)

	db, err := sqliteutil.OpenDB(cfg, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrDatabaseLocked, err)
	}

	vaultConfig := &vault.VaultConfig{
		DataDir: vaultDir,
		Logger:  logger,
	}
	encryptionVault, err := vault.NewVault(vaultConfig)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrVaultCreateFailed, err)
	}

	// Resolve vault key path.
	if vaultKeyPath == "" {
		vaultKeyPath = filepath.Join(vaultDir, constants.VaultKeyFilename)
	}
	if !filepath.IsAbs(vaultKeyPath) {
		vaultKeyPath = filepath.Join(dataDir, vaultKeyPath)
	}

	// Auto-initialize vault on first run. If no vault header exists, generate
	// a random key, create the vault header, and write the key file. This
	// mirrors the `g8e vault init` CLI command and ensures the vault is always
	// ready without requiring a separate initialization step — same pattern as
	// SQLite creating the database file on first open.
	if !vault.VaultHeaderExists(vaultDir) {
		relVaultDir, err := fileSvc.Rel(vaultDir)
		if err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("%w: %w", constants.ErrPathValidation, err)
		}
		if err := fileSvc.MkdirAll(context.Background(), relVaultDir, constants.PermDirPrivate); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("%w: %w", constants.ErrDirCreateFailed, err)
		}

		initKey := make([]byte, vault.KeySize)
		if _, err := rand.Read(initKey); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("%w: %w", constants.ErrVaultKeyGenerateFailed, err)
		}

		header, _, err := vault.NewVaultHeader(initKey)
		if err != nil {
			db.Close()
			vault.SecureZero(initKey)
			return nil, nil, fmt.Errorf("%w: %w", constants.ErrVaultHeaderCreateFailed, err)
		}

		if err := header.Save(vaultDir); err != nil {
			db.Close()
			vault.SecureZero(initKey)
			return nil, nil, fmt.Errorf("%w: %w", constants.ErrVaultHeaderSaveFailed, err)
		}

		keyData := []byte(hex.EncodeToString(initKey) + "\n")
		relVaultKeyPath, err := fileSvc.Rel(vaultKeyPath)
		if err != nil {
			db.Close()
			vault.SecureZero(initKey)
			return nil, nil, fmt.Errorf("%w: %w", constants.ErrPathValidation, err)
		}
		if err := fileSvc.WriteFile(context.Background(), relVaultKeyPath, keyData, constants.PermFilePrivate); err != nil {
			db.Close()
			vault.SecureZero(initKey)
			return nil, nil, fmt.Errorf("%w: %w", constants.ErrVaultKeyWriteFailed, err)
		}

		vault.SecureZero(initKey)
		logger.Info("Vault auto-initialized on first run", "vault_dir", vaultDir, "key_path", vaultKeyPath)
	}

	// Unlock vault. Encryption is required for secure data storage at rest —
	// the vault must always be unlocked at startup. If the key cannot be read
	// or the vault cannot be unlocked, the gateway fails to start.
	privateKey, err := vault.ReadVaultKey(vaultKeyPath)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrVaultKeyReadFailed, err)
	}
	defer vault.SecureZero(privateKey)

	if err := encryptionVault.Unlock(privateKey); err != nil {
		db.Close()
		if errors.Is(err, constants.ErrVaultNotInitialized) {
			return nil, nil, fmt.Errorf("%w: %s", constants.ErrVaultNotInitialized, vaultDir)
		}
		if errors.Is(err, constants.ErrVaultInvalidPrivateKey) {
			return nil, nil, fmt.Errorf("%w: %s", constants.ErrVaultKeyDecodeFailed, vaultKeyPath)
		}
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrVaultUnlockFailed, err)
	}
	logger.Info("Vault unlocked successfully", "vault_dir", vaultDir)

	// Initialize SQLAuditStore for transaction-native audit recording
	auditStoreConfig := storage.DefaultAuditStoreConfig()
	auditStoreConfig.EncryptionVault = encryptionVault
	auditStore, err := storage.NewSQLAuditStore(auditStoreConfig, logger, fileSvc)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrGatewayDBAuditStoreInit, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	svc := &CanonicalDBService{
		db:      db,
		logger:  logger,
		vault:   encryptionVault,
		ctx:     ctx,
		cancel:  cancel,
		running: true,
	}

	// Initialize extracted services with the same db connection
	stores := &Stores{
		DB:           db,
		DocStore:     NewDocumentStoreService(db, logger),
		StateRootSvc: NewStateRootService(db, logger),
		ReplayStore:  NewReplayStoreService(db, logger),
		KVStore:      NewKVStoreService(db, logger),
		SSEStore:     NewSSEEventService(db, logger),
		BlobStore:    NewBlobStoreService(db, logger),
		AuditStore:   auditStore,
	}
	stores.AppPolicyStore = NewAppPolicyStoreService(db, logger, stores.DocStore)
	stores.SignerStore = NewSignerStoreService(db, logger, stores.DocStore)
	stores.ConsensusStore = NewConsensusStoreService(db, logger, stores.DocStore, stores.SignerStore)
	svc.stores = stores

	if err := svc.initSchema(fileSvc, ks); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrGatewayDBSchemaInit, err)
	}

	// Initialize state root if missing
	if err := svc.initStateRoot(); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrGatewayDBStateRootInit, err)
	}

	// Start background maintenance
	svc.wg.Add(1)
	go svc.RunMaintenance(svc.ctx)

	logger.Info("Gateway database initialized", "path", dbPath)
	return svc, stores, nil
}

func (s *CanonicalDBService) initStateRoot() error {
	var count int
	err := s.db.QueryRowWithRetry("SELECT COUNT(*) FROM state_root").Scan(&count)
	if err != nil {
		return fmt.Errorf("canonicalDB: init state root: count: %w", err)
	}
	if count == 0 {
		root, err := s.stores.StateRootSvc.CalculateStateRoot()
		if err != nil {
			return fmt.Errorf("canonicalDB: init state root: calculate: %w", err)
		}
		_, err = s.db.ExecWithRetry(
			"INSERT INTO state_root (id, root, updated_at) VALUES (1, ?, ?)",
			root,
			timesvc.FormatTimestamp(time.Now().UTC()),
		)
		if err != nil {
			return fmt.Errorf("canonicalDB: init state root: insert: %w", err)
		}
	}
	return nil
}

// RunMaintenance periodically removes expired entries by delegating to extracted services.
func (s *CanonicalDBService) RunMaintenance(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.stores.KVStore.RunMaintenance(); err != nil {
				s.logger.Warn("KV store maintenance error", "error", err)
			}
			if err := s.stores.BlobStore.RunMaintenance(); err != nil {
				s.logger.Warn("Blob store maintenance error", "error", err)
			}
			if err := s.stores.ReplayStore.CleanupExpiredNonces(); err != nil {
				s.logger.Warn("Replay store maintenance error", "error", err)
			}
			if _, err := s.stores.SSEStore.SSEEventsCleanup(time.Hour); err != nil {
				s.logger.Warn("SSE event cleanup error", "error", err)
			}
		}
	}
}

func (s *CanonicalDBService) initSchema(fileSvc fs.RuntimeFileService, ks *keystore.Keystore) error {
	_, err := s.db.ExecWithRetry(gatewaySchema)
	if err != nil {
		return fmt.Errorf("canonicalDB: init schema: %w", err)
	}

	var sm *SecretManager
	if ks != nil {
		sm = &SecretManager{
			db:       s.db,
			logger:   s.logger,
			fileSvc:  fileSvc,
			keystore: ks,
		}
	} else {
		sm, err = NewSecretManager(s.db, fileSvc, s.logger)
		if err != nil {
			return fmt.Errorf("canonicalDB: init schema: secret manager: %w", err)
		}
	}
	s.sm = sm
	if err := sm.InitAppSettings(); err != nil {
		return fmt.Errorf("canonicalDB: init schema: app settings: %w", err)
	}

	return nil
}

// GetSecretManager returns the SecretManager initialized during schema init.
func (s *CanonicalDBService) GetSecretManager() *SecretManager {
	return s.sm
}

func (s *CanonicalDBService) GetVault() *vault.Vault {
	return s.vault
}

// GetDB returns the underlying SQLite database handle.
func (s *CanonicalDBService) GetDB() *sqliteutil.DB {
	return s.stores.DB
}

// GetDocStore returns the document store service.
func (s *CanonicalDBService) GetDocStore() *DocumentStoreService {
	return s.stores.DocStore
}

// GetAppPolicyStore returns the app policy store service.
func (s *CanonicalDBService) GetAppPolicyStore() *AppPolicyStoreService {
	return s.stores.AppPolicyStore
}

// GetSignerStore returns the signer store service.
func (s *CanonicalDBService) GetSignerStore() *SignerStoreService {
	return s.stores.SignerStore
}

// GetConsensusStore returns the consensus store service.
func (s *CanonicalDBService) GetConsensusStore() *ConsensusStoreService {
	return s.stores.ConsensusStore
}

// GetStateRootSvc returns the state root service.
func (s *CanonicalDBService) GetStateRootSvc() *StateRootService {
	return s.stores.StateRootSvc
}

// GetReplayStore returns the replay store service.
func (s *CanonicalDBService) GetReplayStore() *ReplayStoreService {
	return s.stores.ReplayStore
}

// GetKVStore returns the key-value store service.
func (s *CanonicalDBService) GetKVStore() *KVStoreService {
	return s.stores.KVStore
}

// GetSSEStore returns the SSE event store service.
func (s *CanonicalDBService) GetSSEStore() *SSEEventService {
	return s.stores.SSEStore
}

// GetBlobStore returns the blob store service.
func (s *CanonicalDBService) GetBlobStore() *BlobStoreService {
	return s.stores.BlobStore
}

// GetAuditStore returns the audit store service.
func (s *CanonicalDBService) GetAuditStore() *storage.SQLAuditStore {
	return s.stores.AuditStore
}

// GetStores returns the internal Stores aggregation struct for HTTPHandler construction.
func (s *CanonicalDBService) GetStores() *Stores {
	return s.stores
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

	if s.stores != nil && s.stores.AuditStore != nil {
		if err := s.stores.AuditStore.Close(); err != nil {
			s.logger.Error("AuditStore close error", "error", err)
		}
	}

	if err := s.db.Close(); err != nil {
		s.logger.Error("Database close error", "error", err)
		return err
	}
	return nil
}
