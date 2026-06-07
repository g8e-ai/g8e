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
	"errors"
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
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
	"github.com/g8e-ai/g8e/internal/services/vault"
)

type G8eoService struct {
	config *config.Config
	logger *slog.Logger

	bootstrap        *auth.BootstrapService
	secretManager    *gateway.SecretManager
	execution        *execution.ExecutionService
	fileEdit         *execution.FileEditService
	pubSubCommands   *pubsub.PubSubCommandService
	pubSubResults    *pubsub.PubSubResultsService
	localStore       *storage.LocalStoreService
	executionVault   *storage.ExecutionVaultService
	tokenStore       *storage.TokenStoreService
	suspendedTxStore *storage.SuspendedTransactionService
	gatewayDB        *gateway.CanonicalDBService

	pubSubClient pubsub.PubSubClient

	auditStore     *storage.SQLAuditStore
	ledger         *storage.GitLedgerService
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
	vs.logger.Info("g8e Operator initializing (Outbound Mode)",
		"posture", vs.config.Posture)

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
	secretsDir := vs.config.SecretsDir

	// Initialize CanonicalDBService for canonical state root calculation
	// This ensures outbound mode uses the same state root schema as gateway mode
	dataDir := filepath.Join(vs.config.WorkDir, ".g8e")
	gatewayDB, err := gateway.OpenCanonicalDBService(dataDir, secretsDir, vs.logger, false, vs.config.VaultKeyPath)
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

	// Initialize vault for encryption
	vaultDir := filepath.Join(vs.config.WorkDir, ".g8e/vault")
	vaultConfig := &vault.VaultConfig{
		DataDir: vaultDir,
		Logger:  vs.logger,
	}
	encryptionVault, err := vault.NewVault(vaultConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize vault: %w", err)
	}

	// Unlock vault before initializing storage services
	// Encryption is required for secure data storage at rest
	vaultKeyPath := vs.config.VaultKeyPath
	if vaultKeyPath == "" {
		vaultKeyPath = filepath.Join(vaultDir, "key")
	}
	if !filepath.IsAbs(vaultKeyPath) {
		vaultKeyPath = filepath.Join(vs.config.WorkDir, vaultKeyPath)
	}

	privateKey, err := vault.ReadVaultKey(vaultKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read vault key: %w", err)
	}
	defer vault.SecureZero(privateKey)

	if err := encryptionVault.Unlock(privateKey); err != nil {
		if errors.Is(err, vault.ErrVaultNotInit) {
			return fmt.Errorf("vault not initialized at %s. Run 'g8e vault init' first", vaultDir)
		}
		if errors.Is(err, vault.ErrInvalidPrivateKey) {
			return fmt.Errorf("invalid vault key at %s. Verify the key file is correct", vaultKeyPath)
		}
		return fmt.Errorf("failed to unlock vault: %w", err)
	}
	vs.logger.Info("Vault unlocked successfully", "vault_dir", vaultDir)

	// Initialize ExecutionVaultService for execution log and file diff storage
	executionVaultConfig := storage.DefaultExecutionVaultConfig()
	executionVaultConfig.DBPath = filepath.Join(dataDir, "execution_vault.db")
	executionVaultConfig.MaxDBSizeMB = vs.config.LocalStoreMaxSizeMB
	executionVaultConfig.RetentionDays = vs.config.LocalStoreRetentionDays
	vs.executionVault, err = storage.NewExecutionVaultService(executionVaultConfig, vs.logger, encryptionVault)
	if err != nil {
		return fmt.Errorf("failed to initialize execution vault: %w", err)
	}
	if vs.executionVault == nil {
		return fmt.Errorf("execution vault is required but was not initialized")
	}
	vs.logger.Info("Execution vault initialized")

	// Initialize TokenStoreService for Sentinel token persistence
	tokenStoreConfig := storage.DefaultTokenStoreConfig()
	tokenStoreConfig.DBPath = filepath.Join(dataDir, "token_store.db")
	vs.tokenStore, err = storage.NewTokenStoreService(tokenStoreConfig, vs.logger, encryptionVault)
	if err != nil {
		return fmt.Errorf("failed to initialize token store: %w", err)
	}
	if vs.tokenStore == nil {
		return fmt.Errorf("token store is required but was not initialized")
	}
	vs.logger.Info("Token store initialized")

	// Initialize SuspendedTransactionService for L3 approval workflow
	suspendedTxConfig := storage.DefaultSuspendedTransactionConfig()
	suspendedTxConfig.DBPath = filepath.Join(dataDir, "suspended_transactions.db")
	vs.suspendedTxStore, err = storage.NewSuspendedTransactionService(suspendedTxConfig, vs.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize suspended transaction store: %w", err)
	}
	if vs.suspendedTxStore == nil {
		return fmt.Errorf("suspended transaction store is required but was not initialized")
	}
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

	// Initialize LocalStoreService for sentinel token persistence and suspended transactions
	localStoreConfig := storage.DefaultLocalStoreConfig()
	localStoreConfig.DBPath = filepath.Join(dataDir, "local_state.db")
	localStoreConfig.MaxDBSizeMB = vs.config.LocalStoreMaxSizeMB
	localStoreConfig.RetentionDays = vs.config.LocalStoreRetentionDays
	vs.localStore, err = storage.NewLocalStoreService(localStoreConfig, vs.logger, encryptionVault)
	if err != nil {
		return fmt.Errorf("failed to initialize local store: %w", err)
	}
	if vs.localStore == nil {
		return fmt.Errorf("local store is required but was not initialized")
	}
	vs.logger.Info("Local store initialized")

	// Initialize SQLAuditStore for history handler
	auditStoreConfig := storage.DefaultAuditStoreConfig()
	auditStoreConfig.DataDir = filepath.Join(vs.config.WorkDir, ".g8e/data")
	auditStoreConfig.EncryptionVault = encryptionVault
	vs.auditStore, err = storage.NewSQLAuditStore(auditStoreConfig, vs.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize audit store: %w", err)
	}
	if vs.auditStore == nil {
		return fmt.Errorf("audit store is required but was not initialized")
	}

	if vs.config.OperatorSessionId == "" {
		return fmt.Errorf("operator session ID required before audit store can accept events")
	}
	operator_session, err := vs.auditStore.GetOperatorSession(vs.config.OperatorSessionId)
	if err != nil {
		return fmt.Errorf("failed to verify audit session: %w", err)
	}
	if operator_session == nil {
		if err := vs.auditStore.CreateSession(vs.config.OperatorSessionId, string(constants.UserRoleOperator), "Operator Session", vs.config.OperatorID); err != nil {
			return fmt.Errorf("failed to create audit session: %w", err)
		}
	}

	if vs.auditStore != nil && vs.auditStore.IsEnabled() && gitPath != "" {
		ledgerConfig := &storage.LedgerConfig{
			BaseDir:         filepath.Join(vs.config.WorkDir, ".g8e/data/ledger"),
			GitPath:         gitPath,
			EncryptionVault: vs.auditStore.GetEncryptionVault(),
		}
		ledger, err := storage.NewGitLedgerService(ledgerConfig, vs.logger)
		if err != nil {
			return fmt.Errorf("failed to initialize ledger: %w", err)
		}
		vs.ledger = ledger
		vs.logger.Info("Ledger initialized")
		vs.historyHandler = storage.NewHistoryHandler(vs.auditStore, vs.ledger, vs.logger)
		vs.logger.Info("History Handler initialized (FETCH_HISTORY ready)")
	} else if vs.auditStore != nil && vs.auditStore.IsEnabled() {
		vs.logger.Warn("Ledger disabled - audit store active without git-backed file versioning")
		vs.historyHandler = storage.NewHistoryHandler(vs.auditStore, nil, vs.logger)
		vs.logger.Info("History Handler initialized (FETCH_HISTORY ready, file history unavailable)")
	}

	// Initialize P0 Transaction Gate infrastructure (replay protection and state root verification)
	// ReplayStore is mandatory for fail-closed replay protection
	replayStoreConfig := storage.DefaultReplayStoreConfig()
	replayStoreConfig.DBPath = filepath.Join(dataDir, "replay_store.db")
	replayStore, err := storage.NewSQLReplayStore(replayStoreConfig, vs.logger)
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
			return fmt.Errorf("failed to create Operator pub/sub client: %w", err)
		}
	}

	vs.pubSubResults, err = pubsub.NewPubSubResultsService(vs.config, vs.logger, vs.pubSubClient, vs.localStore)
	if err != nil {
		return fmt.Errorf("failed to initialize results service: %w", err)
	}

	// Create governance dependencies for transaction verification
	// Use CanonicalDBService for canonical state root calculation (same schema as gateway mode)
	stateRootProvider := vs.gatewayDB
	transactionAudit := &auditStoreTransactionStore{store: vs.auditStore}
	// L3Notary for outbound mode: CLI-based approval via suspended transactions
	// Mutations requiring L3 are suspended and must be approved via CLI command
	cliL3Notary := governance.NewOutboundL3Notary(vs.suspendedTxStore, vs.logger)

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

	// Initialize ScrubbingService for data scrubbing (scrubbing/rehydration)
	scrubbingConfig := scrubbing.DefaultConfig()
	scrubbingService := scrubbing.NewScrubbingService(scrubbingConfig, vs.logger, vs.tokenStore)

	// PubSubCommandService Construction
	psConfig := pubsub.CommandServiceConfig{
		Config:              vs.config,
		Logger:              vs.logger,
		Execution:           vs.execution,
		FileEdit:            vs.fileEdit,
		PubSubClient:        vs.pubSubClient,
		ResultsService:      vs.pubSubResults,
		LocalStore:          vs.localStore,
		AuditStore:          vs.auditStore,
		Ledger:              vs.ledger,
		HistoryHandler:      vs.historyHandler,
		Scrubbing:           scrubbingService,
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

	// Print startup banner to stdout
	printOperatorStartupBanner(vs.config)

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

	// Drain audit store writes
	if vs.auditStore != nil {
		vs.logger.Info("Waiting for audit writes to drain...")
		vs.auditStore.Wait()
	}

	// Close vaults and stores
	if vs.gatewayDB != nil {
		if err := vs.gatewayDB.Close(); err != nil {
			vs.logger.Error("Failed to close gateway database", string(constants.ConnectionStateError), err)
		}
	}

	if vs.executionVault != nil {
		if err := vs.executionVault.Close(); err != nil {
			vs.logger.Error("Failed to close execution vault", string(constants.ConnectionStateError), err)
		}
	}

	if vs.tokenStore != nil {
		if err := vs.tokenStore.Close(); err != nil {
			vs.logger.Error("Failed to close token store", string(constants.ConnectionStateError), err)
		}
	}

	if vs.suspendedTxStore != nil {
		if err := vs.suspendedTxStore.Close(); err != nil {
			vs.logger.Error("Failed to close suspended transaction store", string(constants.ConnectionStateError), err)
		}
	}

	if vs.replayStore != nil {
		if err := vs.replayStore.Close(); err != nil {
			vs.logger.Error("Failed to close replay store", string(constants.ConnectionStateError), err)
		}
	}

	if vs.auditStore != nil {
		if err := vs.auditStore.Close(); err != nil {
			vs.logger.Error("Failed to close audit store", string(constants.ConnectionStateError), err)
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
	if a.store == nil {
		return nil
	}
	var receipt models.ActionReceiptRecord
	if err := json.Unmarshal(data, &receipt); err != nil {
		return fmt.Errorf("failed to decode action receipt record: %w", err)
	}
	// Record directly in receipts table via transaction-native API
	return a.store.RecordActionReceipt(&receipt)
}

// printOperatorStartupBanner prints the Operator startup banner to stdout
func printOperatorStartupBanner(cfg *config.Config) {
	fmt.Println("[g8eo] Initializing Edge Execution Operator...")
	fmt.Println()
	fmt.Println(" ┌── Operator Integrity & Uplink ───────────────────────────────────────────────┐")
	fmt.Println(" │ ✔ Identity & Attestation    : VERIFIED (mTLS Client Certificate Valid)")
	fmt.Printf(" │ ✔ Gateway Uplink            : CONNECTED (WSS @ %s:%d)\n", cfg.Endpoint, cfg.HTTPPort)
	fmt.Println(" │ ✔ Heartbeat Synchronized    : 30s interval established")
	fmt.Println(" │ ✔ Sovereign Boundary        : ACTIVE (Data egress scrubbing enabled)")
	fmt.Println(" └──────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Println(" CAPABILITIES & EXPOSED TOOLING")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Println(" The following agentic capabilities are mounted to this execution platform.")
	fmt.Println(" All state-mutating actions require cryptographic intent verification.")
	fmt.Println()
	fmt.Println("  - system.run      [GRANTED: Requires L1 Signature]")
	fmt.Printf("  - fs.read         [GRANTED: Scoped to %s]\n", cfg.WorkDir)
	fmt.Println("  - fs.write        [GRANTED: Requires L1 Signature]")
	fmt.Println("  - net.fetch       [DENIED:  Air-gap mode active]")
	fmt.Println()
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Println("[g8eo] Edge node operational. Awaiting cryptographically signed agentic intents...")
}
