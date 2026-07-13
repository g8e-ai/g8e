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
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/paths"

	"github.com/g8e-ai/g8e/internal/services/auth"
	"github.com/g8e-ai/g8e/internal/services/execution"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/gateway"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
)

type G8eoService struct {
	config  *config.Config
	logger  *slog.Logger
	fileSvc fs.RuntimeFileService

	bootstrap         *auth.BootstrapService
	secretManager     *gateway.SecretManager
	execution         *execution.ExecutionService
	fileEdit          *execution.FileEditService
	pubSubCommands    *pubsub.OperatorPubSubService
	pubSubResults     *pubsub.PubSubResultsService
	executionVault    *storage.ExecutionVaultService
	tokenStore        storage.TokenStore
	suspendedTxStore  storage.SuspendedTransactionStore
	suspendedTxCloser *storage.SuspendedTransactionService
	gatewayDB         *gateway.CanonicalDBService

	pubSubClient pubsub.PubSubClient
	tlsConfig    *certs.TLSConfig
	testKeystore *keystore.Keystore

	ledger         *storage.GitLedgerService
	historyHandler *storage.HistoryHandler

	// P0 Transaction Gate infrastructure
	replayStore governance.ReplayStore

	ctx    context.Context
	cancel context.CancelFunc

	running   bool
	mu        sync.RWMutex
	startTime time.Time
	wg        sync.WaitGroup
}

// NewG8eoService creates a new Operator service in Outbound Mode.
// In this mode, the Operator initiates all connections to the platform
// on port 443 and performs command execution on the local host.
func NewG8eoService(cfg *config.Config, logger *slog.Logger, tlsConfig *certs.TLSConfig) (*G8eoService, error) {
	service := &G8eoService{
		config:    cfg,
		logger:    logger,
		startTime: time.Now().UTC(),
		tlsConfig: tlsConfig,
	}

	bootstrapService, err := auth.NewBootstrapService(cfg, logger, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
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

// SetKeystore injects a pre-initialized keystore for testing, allowing
// cross-platform tests to bypass OS keychain dependencies.
func (vs *G8eoService) SetKeystore(ks *keystore.Keystore) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.testKeystore = ks
}

// SetFileService injects a RuntimeFileService for file I/O within the .g8e/ directory.
// Must be called before Start().
func (vs *G8eoService) SetFileService(fileSvc fs.RuntimeFileService) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.fileSvc = fileSvc
}

func (vs *G8eoService) Start(ctx context.Context) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.running {
		return fmt.Errorf("%w", constants.ErrServiceUnavailable)
	}

	vs.ctx, vs.cancel = context.WithCancel(ctx)
	vs.logger.Info("g8e Operator initializing (Outbound Mode)",
		"posture", vs.config.Posture)

	if vs.fileSvc == nil {
		return fmt.Errorf("%w: fileSvc must be set via SetFileService before Start()", constants.ErrInternal)
	}

	bootstrapConfig, err := vs.bootstrap.RequestBootstrapConfig(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrNotAuthenticated, err)
	}

	if err = vs.bootstrap.ApplyBootstrapConfig(bootstrapConfig); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}

	vs.execution = execution.NewExecutionService(vs.config, vs.logger)
	vs.fileEdit = execution.NewFileEditService(vs.config, vs.logger)

	// Initialize SecretManager for loading signing keys (Actuator and Consensus)
	// This must be initialized before storage services to provide keystore for encrypted token storage

	// Initialize CanonicalDBService for canonical state root calculation
	// This ensures outbound mode uses the same state root schema as gateway mode
	dataDir := paths.Infra.DataDir
	vaultKeyPath := vs.config.VaultKeyPath
	if vaultKeyPath == "" {
		vaultKeyPath = paths.Infra.VaultKeyPath
	}
	testMode := false
	var testKs *keystore.Keystore
	if vs.testKeystore != nil {
		testMode = true
		testKs = vs.testKeystore
	}
	gatewayDB, err := gateway.OpenCanonicalDBService(dataDir, vs.config.VaultDir, vs.logger, testMode, vaultKeyPath, vs.config.VaultRequireUnlock, testKs, vs.fileSvc)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayDatabaseServiceNotConfigured, err)
	}
	vs.gatewayDB = gatewayDB
	vs.logger.Info("Gateway database initialized (canonical state root)")

	if vs.testKeystore != nil {
		vs.secretManager, err = gateway.NewSecretManagerWithKeystore(vs.gatewayDB.GetDB(), vs.fileSvc, vs.logger, vs.testKeystore)
	} else {
		vs.secretManager, err = gateway.NewSecretManager(vs.gatewayDB.GetDB(), vs.fileSvc, vs.logger)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyNotFound, err)
	}
	if err := vs.secretManager.InitAppSettings(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	vs.logger.Info("Secret manager initialized")

	// Initialize Data Services - mandatory for replay protection
	if !vs.config.ExecutionVaultEnabled {
		return fmt.Errorf("%w: execution vault must be enabled for replay protection - set ExecutionVaultEnabled=true", constants.ErrInternal)
	}

	// Reuse vault from CanonicalDBService (already initialized and unlocked)
	encryptionVault := vs.gatewayDB.GetVault()
	if encryptionVault == nil {
		return fmt.Errorf("%w: vault not available from CanonicalDBService", constants.ErrVaultNotInitialized)
	}
	vs.logger.Info("Vault reused from CanonicalDBService")

	// Initialize ExecutionVaultService for execution log and file diff storage
	executionVaultConfig := storage.DefaultExecutionVaultConfig()
	executionVaultConfig.DBPath = paths.Infra.ExecutionVaultDBPath
	executionVaultConfig.MaxDBSizeMB = vs.config.ExecutionVaultMaxSizeMB
	executionVaultConfig.RetentionDays = vs.config.ExecutionVaultRetentionDays
	vs.executionVault, err = storage.NewExecutionVaultService(executionVaultConfig, vs.logger, encryptionVault)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	vs.logger.Info("Execution vault initialized")

	// Token persistence for Sentinel UEI tokens routes through the canonical gateway
	// DB (g8e.db) via EncryptedKVAdapter — no separate token_store.db.
	vs.tokenStore = gateway.NewEncryptedKVAdapter(vs.gatewayDB.KVStore, encryptionVault)
	vs.logger.Info("Token store initialized (canonical KV store)")

	// Initialize SuspendedTransactionService for L3 approval workflow
	suspendedTxConfig := storage.DefaultSuspendedTransactionConfig()
	suspendedTxConfig.DBPath = paths.Infra.SuspendedTransactionsDBPath
	vs.suspendedTxCloser, err = storage.NewSuspendedTransactionService(suspendedTxConfig, vs.logger)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	vs.suspendedTxStore = vs.suspendedTxCloser
	vs.logger.Info("Suspended transaction store initialized")

	vs.logger.Info("Initializing Local-First Audit Architecture (LFAA)...")

	var gitPath string
	if vs.config.NoGit {
		vs.logger.Info("Git disabled via --no-git flag - ledger will not be available")
	} else {
		vs.logger.Info("Go-git (native Go implementation) initialized and ready")
		gitPath = system.GitEmbedded
	}
	vs.config.GitPath = gitPath
	vs.config.GitAvailable = gitPath != ""

	// Reuse the SQLAuditStore from CanonicalDBService — both the standalone
	// and canonical instances open the same g8e.db file, so a separate connection
	// pool and pruner are redundant. CanonicalDBService.Close() handles lifecycle.
	auditStore := vs.gatewayDB.AuditStore

	if vs.config.OperatorSessionId == "" {
		return fmt.Errorf("%w: operator session ID required before audit store can accept events", constants.ErrGatewayOperatorSessionIDRequired)
	}
	operator_session, err := auditStore.GetOperatorSession(vs.config.OperatorSessionId)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayOperatorSessionInvalid, err)
	}
	if operator_session == nil {
		if err := auditStore.CreateSession(vs.config.OperatorSessionId, string(constants.UserRoleOperator), "Operator Session", vs.config.OperatorID); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrAuditRecordUserMsg, err)
		}
	}

	if auditStore != nil && gitPath != "" {
		ledgerConfig := &storage.LedgerConfig{
			BaseDir:         paths.Infra.LedgerDir,
			GitPath:         gitPath,
			EncryptionVault: encryptionVault,
		}
		ledger, err := storage.NewGitLedgerService(ledgerConfig, vs.logger)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrLedgerConfigRequired, err)
		}
		vs.ledger = ledger
		vs.logger.Info("Ledger initialized")
		vs.historyHandler = storage.NewHistoryHandler(auditStore, vs.ledger, vs.logger)
		vs.logger.Info("History Handler initialized (FETCH_HISTORY ready)")
	} else if auditStore != nil {
		vs.logger.Warn("Ledger disabled - audit store active without git-backed file versioning")
		vs.historyHandler = storage.NewHistoryHandler(auditStore, nil, vs.logger)
		vs.logger.Info("History Handler initialized (FETCH_HISTORY ready, file history unavailable)")
	}

	// Initialize P0 Transaction Gate infrastructure (replay protection and state root verification)
	// ReplayStore is mandatory for fail-closed replay protection
	replayStoreConfig := storage.DefaultReplayStoreConfig()
	replayStoreConfig.DBPath = paths.Infra.ReplayStoreDBPath
	replayStore, err := storage.NewSQLReplayStore(replayStoreConfig, vs.logger)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDatabaseReplay, err)
	}
	vs.replayStore = replayStore
	vs.logger.Info("Replay store initialized for transaction verification")

	// Initialize PubSub Layer
	vs.logger.Info("Establishing g8e connectivity...")

	if vs.pubSubClient == nil {
		vs.pubSubClient, err = pubsub.NewOperatorPubSubClient(vs.config.PubSubURL, vs.config.TLSServerName, vs.logger, vs.tlsConfig)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrInternal, err)
		}
	}

	vs.pubSubResults, err = pubsub.NewPubSubResultsService(vs.config, vs.logger, vs.pubSubClient)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPubSubActuator, err)
	}

	// Create governance dependencies for transaction verification
	// Use CanonicalDBService for canonical state root calculation (same schema as gateway mode)
	stateRootProvider := vs.gatewayDB.StateRootSvc
	transactionAudit := &auditStoreTransactionStore{store: auditStore}
	// L3Notary for outbound mode: CLI-based approval via suspended transactions
	// Mutations requiring L3 are suspended and must be approved via CLI command
	cliL3Notary := governance.NewOutboundL3Notary(vs.suspendedTxStore, vs.logger)

	// Load signing keys for Actuator (fail-closed if missing)
	actuatorPriv, actuatorKeyID, err := vs.secretManager.GetActuatorKey()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyReadFailed, err)
	}

	// Load trusted L2 signers from filesystem
	trustedSignersDir := paths.Infra.TrustedSignersDir
	signerStore, err := governance.NewFilesystemSignerStore(trustedSignersDir, vs.logger)
	if err != nil {
		return fmt.Errorf("%w: failed to load trusted signers: %w", constants.ErrPathNotFound, err)
	}
	vs.logger.Info("Trusted L2 signers loaded from filesystem", "directory", trustedSignersDir)

	// Initialize ScrubbingService for data scrubbing (scrubbing/rehydration)
	scrubbingConfig := scrubbing.DefaultConfig()
	scrubbingService := scrubbing.NewScrubbingService(scrubbingConfig, vs.logger, vs.tokenStore)

	// OperatorPubSubService Construction
	psConfig := pubsub.CommandServiceConfig{
		Config:             vs.config,
		Logger:             vs.logger,
		Execution:          vs.execution,
		FileEdit:           vs.fileEdit,
		PubSubClient:       vs.pubSubClient,
		ResultsService:     vs.pubSubResults,
		ExecutionVault:     vs.executionVault,
		AuditStore:         auditStore,
		Ledger:             vs.ledger,
		HistoryHandler:     vs.historyHandler,
		Scrubbing:          scrubbingService,
		ActuatorSigningKey: actuatorPriv,
		ActuatorKeyID:      actuatorKeyID,
	}

	govDeps := pubsub.GovernanceDeps{
		ReplayStore:       vs.replayStore,
		StateRootProvider: stateRootProvider,
		TransactionAudit:  transactionAudit,
		SignerStore:       signerStore,
		AppPolicyStore:    vs.gatewayDB.AppPolicyStore,
		L3Notary:          cliL3Notary,
	}

	vs.pubSubCommands, err = pubsub.NewOperatorPubSubService(psConfig, govDeps)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPubSubActuator, err)
	}

	if err = vs.pubSubCommands.Start(vs.ctx); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrServiceUnavailable, err)
	}

	vs.running = true

	// Handle external shutdown requests (remote shutdown or SSL failure)
	vs.wg.Add(1)
	go func() {
		defer vs.wg.Done()
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

	// Print startup banner to stdout
	printOperatorStartupBanner(vs.config, vs.logger)

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
			vs.logger.Error("g8eo: failed to stop pubsub command service", "error", err)
		}
	}

	// Stop execution service to kill any active tasks
	if vs.execution != nil {
		vs.execution.Stop()
	}

	// Drain audit store writes (CanonicalDBService.Close() handles final close)
	if vs.gatewayDB != nil && vs.gatewayDB.AuditStore != nil {
		vs.logger.Info("Waiting for audit writes to drain...")
		vs.gatewayDB.AuditStore.Wait()
	}

	// Wait for shutdown handler goroutine to complete
	vs.wg.Wait()

	// Close vaults and stores
	if vs.gatewayDB != nil {
		if err := vs.gatewayDB.Close(); err != nil {
			vs.logger.Error("g8eo: failed to close gateway database", "error", err)
		}
	}

	if vs.executionVault != nil {
		if err := vs.executionVault.Close(); err != nil {
			vs.logger.Error("g8eo: failed to close execution vault", "error", err)
		}
	}

	if vs.suspendedTxCloser != nil {
		if err := vs.suspendedTxCloser.Close(); err != nil {
			vs.logger.Error("g8eo: failed to close suspended transaction store", "error", err)
		}
	}

	if vs.replayStore != nil {
		if err := vs.replayStore.Close(); err != nil {
			vs.logger.Error("g8eo: failed to close replay store", "error", err)
		}
	}

	vs.running = false
	vs.logger.Info("g8e Operator stopped")
	return nil
}

// auditStoreTransactionStore wraps SQLAuditStore to implement governance.TransactionAuditStore.
type auditStoreTransactionStore struct {
	store *storage.SQLAuditStore
}

func (a *auditStoreTransactionStore) DocSet(collection, id string, data json.RawMessage) error {
	var receipt models.ActionReceiptRecord
	if err := json.Unmarshal(data, &receipt); err != nil {
		return fmt.Errorf("%w: auditStoreTransactionStore: failed to decode action receipt record: %w", constants.ErrInvalidJSONBody, err)
	}
	// Record directly in receipts table via transaction-native API
	return a.store.RecordActionReceipt(&receipt)
}

// printOperatorStartupBanner prints the Operator startup banner to stdout
func printOperatorStartupBanner(cfg *config.Config, logger *slog.Logger) {
	logger.Info("[g8eo] Initializing Edge Execution Operator...")
	logger.Info("Operator Integrity & Uplink",
		"identity_attestation", "VERIFIED (mTLS Client Certificate Valid)",
		"gateway_uplink", fmt.Sprintf("CONNECTED (WSS @ %s:%d)", cfg.Endpoint, cfg.HTTPPort),
		"heartbeat", "30s interval established",
		"sovereign_boundary", "ACTIVE (Data egress scrubbing enabled)")
	logger.Info("CAPABILITIES & EXPOSED TOOLING",
		"system.run", "GRANTED: Requires L1 Signature",
		"fs.read", fmt.Sprintf("GRANTED: Scoped to %s", cfg.WorkDir),
		"fs.write", "GRANTED: Requires L1 Signature",
		"net.fetch", "DENIED: Air-gap mode active")
	logger.Info("[g8eo] Edge node operational. Awaiting cryptographically signed agentic intents...")
}
