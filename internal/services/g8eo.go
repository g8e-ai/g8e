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

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/auth"
	"github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/gateway"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/sovereignty"
	"github.com/g8e-ai/g8e/internal/services/storage"
)

type G8eoService struct {
	config *config.Config
	logger *slog.Logger

	bootstrap      *auth.BootstrapService
	secretManager  *gateway.SecretManager
	execution      *execution.ExecutionService
	fileEdit       *execution.FileEditService
	pubSubCommands *pubsub.PubSubCommandService
	pubSubResults  *pubsub.PubSubResultsService
	localStore     *storage.LocalStoreService
	gatewayDB      *gateway.GatewayDBService

	pubSubClient pubsub.PubSubClient

	auditVault     *storage.AuditVaultService
	ledger         *storage.LedgerService
	historyHandler *storage.HistoryHandler

	// P0 Transaction Gate infrastructure
	replayStore governance.ReplayStore

	ctx    context.Context
	cancel context.CancelFunc

	running   bool
	mu        sync.RWMutex
	startTime time.Time
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

	// Initialize SecretManager for loading signing keys (Actuator and Consensus)
	// This must be initialized before LocalStore to provide keystore for encrypted token storage
	secretsDir := filepath.Join(vs.config.WorkDir, ".g8e", "secrets")

	// Initialize GatewayDBService for canonical state root calculation
	// This ensures outbound mode uses the same state root schema as gateway mode
	dataDir := filepath.Join(vs.config.WorkDir, ".g8e")
	gatewayDB, err := gateway.OpenGatewayDBService(dataDir, secretsDir, vs.logger, false)
	if err != nil {
		return fmt.Errorf("failed to initialize gateway database (required for state root calculation): %w", err)
	}
	vs.gatewayDB = gatewayDB
	vs.logger.Info("Gateway database initialized (canonical state root)")

	vs.secretManager, err = gateway.NewSecretManager(vs.gatewayDB.GetDB(), secretsDir, vs.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize secret manager: %w", err)
	}
	if err := vs.secretManager.InitAppSettings(); err != nil {
		return fmt.Errorf("failed to initialize app settings: %w", err)
	}
	vs.logger.Info("Secret manager initialized")

	// Initialize Data Services - mandatory for replay protection
	if !vs.config.LocalStoreEnabled {
		return fmt.Errorf("local storage must be enabled for replay protection - set LocalStorageEnabled=true")
	}

	localStoreConfig := &storage.LocalStoreConfig{
		DBPath:               vs.config.LocalStoreDBPath,
		MaxDBSizeMB:          vs.config.LocalStoreMaxSizeMB,
		RetentionDays:        vs.config.LocalStoreRetentionDays,
		PruneIntervalMinutes: 60,
		Enabled:              true,
	}
	vs.localStore, err = storage.NewLocalStoreService(localStoreConfig, vs.logger, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to initialize local store (required for replay protection): %w", err)
	}
	if vs.localStore == nil {
		return fmt.Errorf("local store is required but was not initialized")
	}
	vs.logger.Info("Local store initialized (consolidated execution vault, encryption enabled)")

	vs.logger.Info("Initializing Local-First Audit Architecture (LFAA)...")

	var gitPath string
	if vs.config.NoGit {
		vs.logger.Info("Git disabled via --no-git flag - ledger will not be available")
	} else {
		vs.logger.Info("Go-git (native Go implementation) initialized and ready")
		gitPath = "embedded"
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
	if vs.auditVault == nil {
		return fmt.Errorf("audit vault is required but was not initialized")
	}
	if vs.config.OperatorSessionId == "" {
		return fmt.Errorf("operator session ID required before audit vault can accept events")
	}
	session, err := vs.auditVault.GetSession(vs.config.OperatorSessionId)
	if err != nil {
		return fmt.Errorf("failed to verify audit session: %w", err)
	}
	if session == nil {
		if err := vs.auditVault.CreateSession(vs.config.OperatorSessionId, "operator", "Operator Session", vs.config.OperatorID); err != nil {
			return fmt.Errorf("failed to create audit session: %w", err)
		}
	}

	if vs.auditVault != nil && vs.auditVault.IsEnabled() && vs.auditVault.IsGitAvailable() {
		vs.ledger = storage.NewLedgerService(vs.auditVault, vs.auditVault.GetEncryptionVault(), vs.logger)
		vs.logger.Info("Ledger initialized")
		vs.historyHandler = storage.NewHistoryHandler(vs.auditVault, vs.ledger, vs.logger)
		vs.logger.Info("History Handler initialized (FETCH_HISTORY ready)")
	} else if vs.auditVault != nil && vs.auditVault.IsEnabled() {
		vs.logger.Warn("Ledger disabled - audit vault active without git-backed file versioning")
		vs.historyHandler = storage.NewHistoryHandler(vs.auditVault, nil, vs.logger)
		vs.logger.Info("History Handler initialized (FETCH_HISTORY ready, file history unavailable)")
	}

	// Initialize P0 Transaction Gate infrastructure (replay protection and state root verification)
	// ReplayStore is mandatory for fail-closed replay protection
	if vs.localStore == nil {
		return fmt.Errorf("local store is required for replay protection initialization")
	}
	replayStore, err := storage.NewSQLReplayStore(vs.localStore.GetDB(), vs.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize replay store (required for transaction verification): %w", err)
	}
	vs.replayStore = replayStore
	vs.logger.Info("Replay store initialized for transaction verification")

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
	// Use GatewayDBService for canonical state root calculation (same schema as gateway mode)
	stateRootProvider := vs.gatewayDB
	transactionAudit := &auditVaultTransactionStore{vault: vs.auditVault}
	// L3Notary for outbound mode: CLI-based approval via suspended transactions
	// Mutations requiring L3 are suspended and must be approved via CLI command
	cliL3Notary := governance.NewOutboundL3Notary(vs.localStore, vs.logger)

	// Load signing keys for Actuator and Consensus (fail-closed if missing)
	actuatorPriv, actuatorKeyID, err := vs.secretManager.GetActuatorKey()
	if err != nil {
		return fmt.Errorf("failed to load Actuator signing key: %w", err)
	}
	consensusPriv, err := vs.secretManager.GetConsensusKey()
	if err != nil {
		return fmt.Errorf("failed to load Consensus signing key: %w", err)
	}
	vs.logger.Info("Consensus signing key loaded successfully")

	// Load trusted L2 signers from filesystem (fail-closed if directory doesn't exist)
	trustedSignersDir := filepath.Join(vs.config.PKIDir, "trusted_signers")
	signerStore, err := governance.NewFilesystemSignerStore(trustedSignersDir, vs.logger)
	if err != nil {
		return fmt.Errorf("failed to load trusted signers from %s: %w", trustedSignersDir, err)
	}
	vs.logger.Info("Trusted L2 signers loaded from filesystem", "directory", trustedSignersDir)

	// Initialize SovereigntyService for data sovereignty (scrubbing/rehydration)
	sovereigntyConfig := sovereignty.DefaultConfig()
	sovereigntyService := sovereignty.NewSovereigntyService(sovereigntyConfig, vs.logger, vs.localStore)

	// PubSubCommandService Construction
	psConfig := pubsub.CommandServiceConfig{
		Config:              vs.config,
		Logger:              vs.logger,
		Execution:           vs.execution,
		FileEdit:            vs.fileEdit,
		PubSubClient:        vs.pubSubClient,
		ResultsService:      vs.pubSubResults,
		LocalStore:          vs.localStore,
		AuditVault:          vs.auditVault,
		Ledger:              vs.ledger,
		HistoryHandler:      vs.historyHandler,
		Sovereignty:         sovereigntyService,
		ReplayStore:         vs.replayStore,
		StateRootProvider:   stateRootProvider,
		TransactionAudit:    transactionAudit,
		SignerStore:         signerStore,
		AppPolicyStore:      vs.gatewayDB,
		ActuatorSigningKey:  actuatorPriv,
		ActuatorKeyID:       actuatorKeyID,
		ConsensusSigningKey: consensusPriv,
		L3Notary:            cliL3Notary,
	}

	vs.pubSubCommands, err = pubsub.NewPubSubCommandService(psConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize command service: %w", err)
	}

	// Wire SovereigntyService into LocalStoreService for AI data sovereignty scrubbing
	// This must happen after SovereigntyService is created to break circular dependency
	vs.localStore.SetScrubber(sovereigntyService)
	vs.logger.Info("SovereigntyService wired to LocalStoreService for AI data sovereignty")

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
		if vs.pubSubCommands.Actuator() != nil {
			vs.logger.Info("Waiting for in-flight transactions to drain...")
			vs.pubSubCommands.Actuator().Wait()
		}
		if err := vs.pubSubCommands.Stop(); err != nil {
			vs.logger.Error("Failed to stop pubsub command service", string(constants.ConnectionStateError), err)
		}
	}

	// Stop execution service to kill any active tasks
	if vs.execution != nil {
		vs.execution.Stop()
	}

	// Drain audit vault writes
	if vs.auditVault != nil {
		vs.logger.Info("Waiting for audit writes to drain...")
		vs.auditVault.Wait()
	}

	// Close vaults and stores
	if vs.gatewayDB != nil {
		if err := vs.gatewayDB.Close(); err != nil {
			vs.logger.Error("Failed to close gateway database", string(constants.ConnectionStateError), err)
		}
	}

	if vs.localStore != nil {
		if err := vs.localStore.Close(); err != nil {
			vs.logger.Error("Failed to close local store", string(constants.ConnectionStateError), err)
		}
	}

	if vs.auditVault != nil {
		if err := vs.auditVault.Close(); err != nil {
			vs.logger.Error("Failed to close audit vault", string(constants.ConnectionStateError), err)
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
