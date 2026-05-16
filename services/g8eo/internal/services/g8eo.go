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

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/auth"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/execution"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/pubsub"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sentinel"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/storage"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/system"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/vault"
)

type G8eoService struct {
	config *config.Config
	logger *slog.Logger

	bootstrap      *auth.BootstrapService
	execution      *execution.ExecutionService
	fileEdit       *execution.FileEditService
	pubSubCommands *pubsub.PubSubCommandService
	pubSubResults  *pubsub.PubSubResultsService
	localStore     *storage.LocalStoreService
	rawVault       *storage.RawVaultService

	pubSubClient pubsub.PubSubClient

	auditVault      *storage.AuditVaultService
	encryptionVault *vault.Vault
	ledger          *storage.LedgerService
	historyHandler  *storage.HistoryHandler

	sentinel *sentinel.Sentinel

	// P0 Transaction Gate infrastructure
	replayStore governance.ReplayStore

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	running   bool
	mu        sync.RWMutex
	startTime time.Time
}

func ProductionSentinelConfig() *sentinel.SentinelConfig {
	return &sentinel.SentinelConfig{
		Enabled:                true,
		StrictMode:             true,
		ThreatDetectionEnabled: true,
		MaxOutputLength:        4096,
	}
}

// NewG8eoService creates a new Operator service in Outbound Mode.
// In this mode, the Operator initiates all connections to the platform
// on port 443 and performs command execution on the local host.
func NewG8eoService(cfg *config.Config, logger *slog.Logger) (*G8eoService, error) {
	service := &G8eoService{
		config:    cfg,
		logger:    logger,
		startTime: time.Now().UTC(),
	}

	bootstrapService, err := auth.NewBootstrapService(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create bootstrap service: %w", err)
	}
	service.bootstrap = bootstrapService

	return service, nil
}

// SetPubSubClient injects a custom PubSub client for testing.
func (vs *G8eoService) SetPubSubClient(client pubsub.PubSubClient) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.pubSubClient = client
}

func (vs *G8eoService) Start(ctx context.Context) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.running {
		return fmt.Errorf("operator service is already running")
	}

	vs.ctx, vs.cancel = context.WithCancel(ctx)
	vs.logger.Info("g8e Operator initializing (Outbound Mode)...")

	bootstrapConfig, err := vs.bootstrap.RequestBootstrapConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to authenticate: %w", err)
	}

	if err = vs.bootstrap.ApplyBootstrapConfig(bootstrapConfig); err != nil {
		return fmt.Errorf("failed to apply bootstrap configuration: %w", err)
	}

	vs.execution = execution.NewExecutionService(vs.config, vs.logger)
	vs.fileEdit = execution.NewFileEditService(vs.config, vs.logger)

	// Initialize Data Services
	if vs.config.LocalStoreEnabled {
		localStoreConfig := &storage.LocalStoreConfig{
			DBPath:               vs.config.LocalStoreDBPath,
			MaxDBSizeMB:          vs.config.LocalStoreMaxSizeMB,
			RetentionDays:        vs.config.LocalStoreRetentionDays,
			PruneIntervalMinutes: 60,
			Enabled:              true,
		}
		vs.localStore, err = storage.NewLocalStoreService(localStoreConfig, vs.logger)
		if err != nil {
			vs.logger.Warn("Failed to initialize scrubbed vault - continuing without it", "error", err)
		} else if vs.localStore != nil {
			vs.logger.Info("Scrubbed vault initialized (AI-accessible)")
		}

		rawVaultConfig := &storage.RawVaultConfig{
			DBPath:               filepath.Join(vs.config.WorkDir, ".g8e", "raw_vault.db"),
			MaxDBSizeMB:          2048,
			RetentionDays:        30,
			PruneIntervalMinutes: 60,
			Enabled:              true,
		}
		vs.rawVault, err = storage.NewRawVaultService(rawVaultConfig, vs.logger)
		if err != nil {
			vs.logger.Warn("Failed to initialize raw vault - continuing without it", "error", err)
		} else if vs.rawVault != nil {
			vs.logger.Info("Raw vault initialized (customer data store)")
		}
	}

	vs.logger.Info("Initializing Local-First Audit Architecture (LFAA)...")

	var gitPath string
	if vs.config.NoGit {
		vs.logger.Info("Git disabled via --no-git flag — ledger will not be available")
	} else {
		gitPath = system.ResolveGitBinary(vs.logger)
		if gitPath != "" {
			if gitVersion, err := system.ValidateGitBinary(gitPath); err != nil {
				vs.logger.Warn("Git binary found but not functional — ledger will not be available", "path", gitPath, "error", err)
				gitPath = ""
			} else {
				vs.logger.Info("Git binary validated", "version", gitVersion)
			}
		}
	}
	vs.config.GitPath = gitPath
	vs.config.GitAvailable = gitPath != ""

	auditVaultConfig := storage.DefaultAuditVaultConfig()
	auditVaultConfig.DataDir = filepath.Join(vs.config.WorkDir, ".g8e", "data")
	auditVaultConfig.GitPath = gitPath
	vs.auditVault, err = storage.NewAuditVaultService(auditVaultConfig, vs.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize audit vault: %w", err)
	}
	if vs.config.OperatorSessionId == "" {
		return fmt.Errorf("operator session ID required before audit vault can accept events")
	}
	session, err := vs.auditVault.GetSession(vs.config.OperatorSessionId)
	if err != nil {
		return fmt.Errorf("failed to verify audit session: %w", err)
	}
	if session == nil {
		if err := vs.auditVault.CreateSession(vs.config.OperatorSessionId, "Operator Session", vs.config.OperatorID); err != nil {
			return fmt.Errorf("failed to create audit session: %w", err)
		}
	}

	if vs.auditVault != nil && vs.auditVault.IsEnabled() && vs.auditVault.IsGitAvailable() {
		vs.ledger = storage.NewLedgerService(vs.auditVault, vs.auditVault.GetEncryptionVault(), vs.logger)
		vs.logger.Info("Ledger initialized")
		vs.historyHandler = storage.NewHistoryHandler(vs.auditVault, vs.ledger, vs.logger)
		vs.logger.Info("History Handler initialized (FETCH_HISTORY ready)")
	} else if vs.auditVault != nil && vs.auditVault.IsEnabled() {
		vs.logger.Warn("Ledger disabled — audit vault active without git-backed file versioning")
		vs.historyHandler = storage.NewHistoryHandler(vs.auditVault, nil, vs.logger)
		vs.logger.Info("History Handler initialized (FETCH_HISTORY ready, file history unavailable)")
	}

	// Initialize P0 Transaction Gate infrastructure (replay protection and state root verification)
	if vs.localStore != nil {
		replayStore, err := storage.NewSQLReplayStore(vs.localStore.GetDB().DB, vs.logger)
		if err != nil {
			vs.logger.Warn("Failed to initialize replay store - transaction verification will not enforce replay protection", "error", err)
		} else {
			vs.replayStore = replayStore
			vs.logger.Info("Replay store initialized for transaction verification")
		}
	}

	// Initialize PubSub Layer
	vs.logger.Info("Establishing g8e connectivity...")

	if vs.pubSubClient == nil {
		vs.pubSubClient, err = pubsub.NewOperatorPubSubClient(vs.config.PubSubURL, vs.config.TLSServerName, vs.logger)
		if err != nil {
			return fmt.Errorf("failed to create operator pub/sub client: %w", err)
		}
	}

	vs.pubSubResults, err = pubsub.NewPubSubResultsService(vs.config, vs.logger, vs.pubSubClient, vs.localStore)
	if err != nil {
		return fmt.Errorf("failed to initialize results service: %w", err)
	}

	// Create governance dependencies for transaction verification
	// Outbound mode does not configure L3Verifier - mutations will fail-closed at verification layer
	stateRootProvider := &governance.SimpleStateRootProvider{Root: vs.config.SystemFingerprint}
	transactionAudit := &auditVaultTransactionStore{vault: vs.auditVault}
	// NoOpL3Verifier removed - outbound mode now fails-closed when mutations require L3 verification
	// This is intentional: outbound operators must connect to a platform that provides L3 verification

	// PubSubCommandService Construction
	psConfig := pubsub.CommandServiceConfig{
		Config:            vs.config,
		Logger:            vs.logger,
		Execution:         vs.execution,
		FileEdit:          vs.fileEdit,
		PubSubClient:      vs.pubSubClient,
		ResultsService:    vs.pubSubResults,
		LocalStore:        vs.localStore,
		RawVault:          vs.rawVault,
		AuditVault:        vs.auditVault,
		Ledger:            vs.ledger,
		HistoryHandler:    vs.historyHandler,
		Sentinel:          sentinel.NewSentinel(ProductionSentinelConfig(), vs.logger),
		ReplayStore:       vs.replayStore,
		StateRootProvider: stateRootProvider,
		TransactionAudit:  transactionAudit,
		// L3Verifier intentionally nil - mutations will fail-closed at TransactionVerifier
	}

	vs.pubSubCommands, err = pubsub.NewPubSubCommandService(psConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize command service: %w", err)
	}
	vs.sentinel = psConfig.Sentinel

	if err = vs.pubSubCommands.Start(vs.ctx); err != nil {
		return fmt.Errorf("failed to start command service: %w", err)
	}

	vs.running = true

	// Handle external shutdown requests (remote shutdown or SSL failure)
	go func() {
		select {
		case reason := <-vs.pubSubCommands.ShutdownChan:
			vs.logger.Info("g8eo Service received external shutdown request", "reason", reason)
			// We can't call vs.Stop() here because it would deadlock on vs.mu
			// Instead, we signal the main loop via the context or a dedicated channel if needed.
			// However, in our current architecture, the main.go's context is what we should cancel.
			if vs.cancel != nil {
				vs.cancel()
			}
		case <-vs.ctx.Done():
			return
		}
	}()

	vs.logger.Info("g8e Operator started successfully!",
		"max_concurrent_tasks", vs.config.MaxConcurrentTasks,
		"startup_duration", time.Since(vs.startTime))

	vs.logger.Info("Standing by")
	return nil
}

// Stop gracefully shuts down all g8eo sub-services.
func (vs *G8eoService) Stop(ctx context.Context) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if !vs.running {
		return nil
	}

	vs.logger.Info("g8e Operator shutting down...")

	if vs.cancel != nil {
		vs.cancel()
	}

	// Stop pubsub command service first to stop receiving new commands
	if vs.pubSubCommands != nil {
		if err := vs.pubSubCommands.Stop(); err != nil {
			vs.logger.Error("Failed to stop pubsub command service", "error", err)
		}
	}

	// Stop execution service to kill any active tasks
	if vs.execution != nil {
		vs.execution.Stop()
	}

	// Close vaults and stores
	if vs.localStore != nil {
		if err := vs.localStore.Close(); err != nil {
			vs.logger.Error("Failed to close local store", "error", err)
		}
	}

	if vs.rawVault != nil {
		if err := vs.rawVault.Close(); err != nil {
			vs.logger.Error("Failed to close raw vault", "error", err)
		}
	}

	if vs.auditVault != nil {
		if err := vs.auditVault.Close(); err != nil {
			vs.logger.Error("Failed to close audit vault", "error", err)
		}
	}

	vs.running = false
	vs.logger.Info("g8e Operator stopped")
	return nil
}

// auditVaultTransactionStore wraps AuditVaultService to implement governance.TransactionAuditStore.
type auditVaultTransactionStore struct {
	vault *storage.AuditVaultService
}

func (a *auditVaultTransactionStore) DocSet(collection, id string, data json.RawMessage) error {
	if a.vault == nil {
		return nil
	}
	var receipt models.ActionReceiptRecord
	if err := json.Unmarshal(data, &receipt); err != nil {
		return fmt.Errorf("failed to decode action receipt record: %w", err)
	}
	// Record directly in receipts table via transaction-native API
	return a.vault.RecordActionReceipt(&receipt)
}
