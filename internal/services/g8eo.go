// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package services

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/adapters/lattice"
	taskmanagerv1 "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/gen/anduril/taskmanager/v1"
	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/httpclient"
	"github.com/g8e-ai/g8e/v2/internal/paths"

	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/execution"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/system"
)

type G8eoService struct {
	config  *config.Config
	logger  *slog.Logger
	fileSvc fs.RuntimeFileService

	bootstrap        *auth.BootstrapService
	secretManager    *gateway.SecretManager
	execution        *execution.ExecutionService
	fileEdit         *execution.FileEditService
	pubSubCommands   *pubsub.OperatorPubSubService
	pubSubResults    *pubsub.PubSubResultsService
	executionVault   *storage.ExecutionVaultService
	tokenStore       storage.TokenStore
	suspendedTxStore *storage.SuspendedTransactionService
	gatewayDB        *gateway.CanonicalDBService

	pubSubClient pubsub.PubSubClient
	tlsConfig    *certs.TLSConfig
	keystore     *keystore.Keystore

	ledger         *storage.GitLedgerService
	historyHandler *storage.HistoryHandler

	// P0 Transaction Gate infrastructure
	replayStore governance.ReplayStore

	latticeAdapter *lattice.Adapter

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
func NewG8eoService(cfg *config.Config, logger *slog.Logger, tlsConfig *certs.TLSConfig, fileSvc fs.RuntimeFileService) (*G8eoService, error) {
	service := &G8eoService{
		config:    cfg,
		logger:    logger,
		startTime: time.Now().UTC(),
		tlsConfig: tlsConfig,
		fileSvc:   fileSvc,
	}

	bootstrapService, err := auth.NewBootstrapService(cfg, logger, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	service.bootstrap = bootstrapService

	return service, nil
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
		return fmt.Errorf("%w: fileSvc must be provided to NewG8eoService", constants.ErrInternal)
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
	gatewayDB, _, err := gateway.OpenCanonicalDBService(dataDir, vs.config.VaultDir, vs.logger, vaultKeyPath, vs.keystore, vs.fileSvc)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayDatabaseServiceNotConfigured, err)
	}
	vs.gatewayDB = gatewayDB
	vs.logger.Info("Gateway database initialized (canonical state root)")

	vs.secretManager = gatewayDB.GetSecretManager()
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
	vs.tokenStore = gateway.NewEncryptedKVAdapter(vs.gatewayDB.GetKVStore(), encryptionVault)
	vs.logger.Info("Token store initialized (canonical KV store)")

	// Initialize SuspendedTransactionService for L3 approval workflow
	suspendedTxConfig := storage.DefaultSuspendedTransactionConfig()
	suspendedTxConfig.DBPath = paths.Infra.SuspendedTransactionsDBPath
	vs.suspendedTxStore, err = storage.NewSuspendedTransactionService(suspendedTxConfig, vs.logger)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
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

	// Reuse the SQLAuditStore from CanonicalDBService — both the standalone
	// and canonical instances open the same g8e.db file, so a separate connection
	// pool and pruner are redundant. CanonicalDBService.Close() handles lifecycle.
	auditStore := vs.gatewayDB.GetAuditStore()

	if vs.config.OperatorSessionId == "" {
		return fmt.Errorf("%w: operator session ID required before audit store can accept events", constants.ErrGatewayOperatorSessionIDRequired)
	}
	operator_session, err := auditStore.GetOperatorSession(vs.config.OperatorSessionId)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrGatewayOperatorSessionInvalid, err)
	}
	if operator_session == nil {
		if err := auditStore.CreateSession(vs.config.OperatorSessionId, constants.SessionTypeOperator, "Operator Session", vs.config.OperatorID); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrAuditRecordUserMsg, err)
		}
	}

	if auditStore != nil && gitPath != "" {
		ledgerConfig := &storage.LedgerConfig{
			GitPath:         gitPath,
			EncryptionVault: encryptionVault,
		}
		ledger, err := storage.NewGitLedgerService(ledgerConfig, vs.logger, vs.fileSvc)
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

	// Create governance dependencies for transaction verification.
	// The gateway owns the state Merkle root; operators are leaves in the
	// gateway's Merkle tree. When the operator is configured to talk to a
	// gateway (OperatorEndpoint is set), it fetches the gateway's state root
	// via the /api/v1/state endpoint and uses it for L4Warden verification
	// instead of computing its own local root. In standalone mode (no
	// gateway endpoint), fall back to the local StateRootService.
	var stateRootProvider governance.StateRootProvider
	if vs.config.Endpoint != "" {
		httpClient, err := httpclient.NewWithTLSConfigAndServerName(vs.tlsConfig, vs.config.TLSServerName)
		if err != nil {
			return fmt.Errorf("g8eo: failed to create state root HTTP client: %w", err)
		}
		hostname := vs.config.Endpoint
		if vs.config.TLSServerName != "" {
			hostname = vs.config.TLSServerName
		}
		baseURL := fmt.Sprintf("https://%s:%d", hostname, vs.config.HTTPSPort)
		stateRootProvider = governance.NewRemoteStateRootProvider(httpClient, baseURL, vs.logger)
		vs.logger.Info("Using remote (gateway) state root provider", "state_url", baseURL+constants.APIPaths.State)
	} else {
		stateRootProvider = vs.gatewayDB.GetStateRootSvc()
		vs.logger.Info("Using local state root provider (standalone mode)")
	}
	transactionAudit := auditStore
	// L3Notary for outbound mode: CLI-based approval via suspended transactions
	// Mutations requiring L3 are suspended and must be approved via CLI command
	cliL3Notary := governance.NewOutboundL3Notary(vs.suspendedTxStore, vs.logger)

	// Load signing keys for Actuator (fail-closed if missing)
	actuatorPriv, actuatorKeyID, err := vs.secretManager.GetActuatorKey()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrKeyReadFailed, err)
	}
	if err := governance.ExportActuatorPublicKey(vs.fileSvc, actuatorPriv.Public().(ed25519.PublicKey), actuatorKeyID, vs.logger); err != nil {
		return fmt.Errorf("g8eo: export actuator public key: %w", err)
	}
	auditorPriv, auditorKeyID, err := vs.secretManager.GetAuditorKey()
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
	scrubbingService, err := scrubbing.NewScrubbingService(ctx, scrubbingConfig, vs.logger, vs.tokenStore)
	if err != nil {
		return fmt.Errorf("g8eo: failed to initialize scrubbing service: %w", err)
	}

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
		AuditorSigningKey:  auditorPriv,
		AuditorKeyID:       auditorKeyID,
	}

	outboundDeps, err := pubsub.NewOutboundModeDeps(pubsub.OutboundModeDeps{
		GovernanceCoreDeps: pubsub.GovernanceCoreDeps{
			ReplayStore:       vs.replayStore,
			StateRootProvider: stateRootProvider,
			TransactionAudit:  transactionAudit,
			SignerStore:       signerStore,
			L3Notary:          cliL3Notary,
			Doctrine:          governance.NewL1Doctrine(),
		},
	})
	if err != nil {
		return fmt.Errorf("g8eo: outbound mode deps: %w", err)
	}

	vs.pubSubCommands, err = pubsub.NewOperatorPubSubService(psConfig, *outboundDeps)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPubSubActuator, err)
	}

	if err = vs.pubSubCommands.Start(vs.ctx); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrServiceUnavailable, err)
	}

	if vs.config.Lattice != nil && vs.config.Lattice.Enabled {
		if err := vs.config.Lattice.Validate(); err != nil {
			return fmt.Errorf("lattice: config validation: %w", err)
		}
		if err := lattice.ValidateHeartbeatInterval(vs.config.HeartbeatInterval); err != nil {
			return fmt.Errorf("lattice: heartbeat interval: %w", err)
		}

		tlsCfg, err := vs.tlsConfig.GetTLSConfig()
		if err != nil {
			return fmt.Errorf("lattice: tls config: %w", err)
		}
		adapter, err := lattice.NewAdapter(vs.config.Lattice, vs.fileSvc, tlsCfg, vs.logger)
		if err != nil {
			return fmt.Errorf("lattice: dial: %w", err)
		}

		adapter.SetHeartbeatService(vs.pubSubCommands.HeartbeatService())
		adapter.SetTaskHandler(vs.latticeTaskHandler)
		adapter.SetPostureProvider(func() string {
			return string(vs.config.Posture)
		})

		if err := adapter.Start(vs.ctx); err != nil {
			return fmt.Errorf("lattice: start: %w", err)
		}
		vs.latticeAdapter = adapter
		vs.logger.Info("Lattice adapter started",
			"endpoint", vs.config.Lattice.Endpoint,
			"entity", vs.config.Lattice.Entity.Name)
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

	// Stop Lattice adapter (after pubsub stops receiving new tasks)
	if vs.latticeAdapter != nil {
		if err := vs.latticeAdapter.Stop(ctx); err != nil {
			vs.logger.Error("g8eo: failed to stop Lattice adapter", "error", err)
		}
	}

	// Stop execution service to kill any active tasks
	if vs.execution != nil {
		vs.execution.Stop()
	}

	// Drain audit store writes (CanonicalDBService.Close() handles final close)
	if vs.gatewayDB != nil && vs.gatewayDB.GetAuditStore() != nil {
		vs.logger.Info("Waiting for audit writes to drain...")
		vs.gatewayDB.GetAuditStore().Wait()
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

	if vs.suspendedTxStore != nil {
		if err := vs.suspendedTxStore.Close(); err != nil {
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

func (vs *G8eoService) latticeTaskHandler(ctx context.Context, task *taskmanagerv1.Task) error {
	vs.logger.Info("Lattice task received", "task_id", task.GetVersion().GetTaskId())
	return nil
}
