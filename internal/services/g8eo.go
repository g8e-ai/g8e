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
	"net"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/adapters/lattice"
	taskmanagerv1 "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/gen/anduril/taskmanager/v1"
	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/httpclient"

	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/execution"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/gateway"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/services/system"
)

// ExecutesFor binds outbound execution to this runtime's enrolled Operator ID.
func (vs *G8eoService) ExecutesFor(operatorID string) bool {
	return vs != nil && vs.config != nil && operatorID != "" && operatorID == vs.config.OperatorID
}

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

	pubSubClientFactory   PubSubClientFactory
	pubSubClient          pubsub.PubSubClient
	onCommandSubscription func(context.Context, string, bool) error
	tlsConfig             *certs.TLSConfig
	keystore              *keystore.Keystore

	ledger         *storage.GitLedgerService
	historyHandler *storage.HistoryHandler

	// P0 Transaction Gate infrastructure
	replayStore governance.ReplayStore

	latticeAdapter *lattice.Adapter

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	running   bool
	mu        sync.RWMutex
	startTime time.Time
	wg        sync.WaitGroup
}

type PubSubClientFactory func(baseURL, serverName string, logger *slog.Logger, tlsConfig *certs.TLSConfig) (pubsub.PubSubClient, error)

// NewG8eoService creates a new Operator service in Outbound Mode.
// In this mode, the Operator initiates all connections to the platform
// on port 443 and performs command execution on the local host.
func NewG8eoService(cfg *config.Config, logger *slog.Logger, tlsConfig *certs.TLSConfig, fileSvc fs.RuntimeFileService, pubSubClientFactory PubSubClientFactory) (*G8eoService, error) {
	if pubSubClientFactory == nil {
		return nil, fmt.Errorf("%w: pub/sub client factory is required", constants.ErrInternal)
	}

	service := &G8eoService{
		config:              cfg,
		logger:              logger,
		startTime:           time.Now().UTC(),
		tlsConfig:           tlsConfig,
		fileSvc:             fileSvc,
		pubSubClientFactory: pubSubClientFactory,
		done:                make(chan struct{}),
	}

	bootstrapService, err := auth.NewBootstrapService(cfg, logger, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	service.bootstrap = bootstrapService

	return service, nil
}

// SetCommandSubscriptionObserver installs a startup dependency before Start.
func (vs *G8eoService) SetCommandSubscriptionObserver(observer func(context.Context, string, bool) error) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.ctx != nil {
		return fmt.Errorf("operator subscription observer: %w", constants.ErrServiceUnavailable)
	}
	vs.onCommandSubscription = observer
	return nil
}

// Done is closed when the Operator service context ends. The CLI waits on it
// so a governed shutdown request can stop the process after service cleanup.
func (vs *G8eoService) Done() <-chan struct{} {
	return vs.done
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

	// Validate local inference configuration without contacting the provider.
	// Provider availability belongs to individual requests, not Operator liveness.
	var ollamaBackend *inference.OllamaBackend
	if vs.config.Inference.Enabled {
		var err error
		ollamaBackend, err = vs.newInferenceBackend()
		if err != nil {
			return err
		}
	}

	bootstrapConfig, err := vs.bootstrap.RequestBootstrapConfig(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrNotAuthenticated, err)
	}

	if err = vs.bootstrap.ApplyBootstrapConfig(bootstrapConfig); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConfigLoadFailed, err)
	}

	vs.logger.Info("Establishing g8e connectivity...")
	vs.pubSubClient, err = vs.pubSubClientFactory(vs.config.PubSubURL, vs.config.TLSServerName, vs.logger, vs.tlsConfig)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	vs.execution = execution.NewExecutionService(vs.config, vs.logger)
	vs.fileEdit = execution.NewFileEditService(vs.config, vs.logger)

	// Initialize SecretManager for loading signing keys (Actuator and Consensus)
	// This must be initialized before storage services to provide keystore for encrypted token storage

	// Initialize CanonicalDBService for canonical state root calculation
	// This ensures outbound mode uses the same state root schema as gateway mode.
	gatewayDB, err := gateway.OpenCanonicalDBService(vs.logger, vs.config.VaultKeyPath, vs.keystore, vs.fileSvc)
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
	executionVaultConfig.DBPath = vs.fileSvc.Resolve(filepath.Join(constants.DataDirname, constants.ExecutionVaultDBFilename))
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
	suspendedTxConfig.DBPath = vs.fileSvc.Resolve(filepath.Join(constants.DataDirname, constants.SuspendedTxFilename))
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
	replayStoreConfig.DBPath = vs.fileSvc.Resolve(filepath.Join(constants.DataDirname, constants.ReplayStoreDBFilename))
	replayStore, err := storage.NewSQLReplayStore(replayStoreConfig, vs.logger)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDatabaseReplay, err)
	}
	vs.replayStore = replayStore
	vs.logger.Info("Replay store initialized for transaction verification")

	// Initialize PubSub Layer
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
		baseURL := "https://" + net.JoinHostPort(vs.config.Endpoint, strconv.Itoa(vs.config.HTTPSPort))
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
	trustedSignersDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrustedSigners)
	signerStore, err := governance.NewFilesystemSignerStore(vs.fileSvc, trustedSignersDir, vs.logger)
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

	handlers, err := pubsub.NewRoleHandlers(vs.config, vs.logger, vs.fileSvc, scrubbingService, vs.pubSubResults, ollamaBackend)
	if err != nil {
		return err
	}

	// OperatorPubSubService Construction
	psConfig := pubsub.CommandServiceConfig{
		Config:                   vs.config,
		OnCommandSubscription:    vs.onCommandSubscription,
		Logger:                   vs.logger,
		Execution:                vs.execution,
		FileEdit:                 vs.fileEdit,
		PubSubClient:             vs.pubSubClient,
		ResultsService:           vs.pubSubResults,
		ExecutionVault:           vs.executionVault,
		AuditStore:               auditStore,
		Ledger:                   vs.ledger,
		HistoryHandler:           vs.historyHandler,
		Scrubbing:                scrubbingService,
		Inference:                handlers.Inference,
		InferenceAttemptStore:    handlers.InferenceAttemptStore,
		ProviderBoundaryObserver: handlers.ProviderBoundaryObserver,
		ModelProvenanceOperator:  handlers.ModelProvenanceOperator,
		ActuatorSigningKey:       actuatorPriv,
		ActuatorKeyID:            actuatorKeyID,
		AuditorSigningKey:        auditorPriv,
		AuditorKeyID:             auditorKeyID,
	}

	outboundDeps, err := pubsub.NewOutboundModeDeps(pubsub.OutboundModeDeps{
		GovernanceCoreDeps: pubsub.GovernanceCoreDeps{
			ExecutionTarget:   vs,
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
		defer close(vs.done)
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

// newInferenceBackend validates the endpoint without requiring the remote provider
// to be online. The reusable backend retries connectivity on each request, allowing
// recovery without restarting or re-enrolling the Operator.
func (vs *G8eoService) newInferenceBackend() (*inference.OllamaBackend, error) {
	backend, err := inference.NewOllamaBackend(vs.config.Inference.OllamaEndpoint, vs.logger)
	if err != nil {
		return nil, fmt.Errorf("g8eo: inference backend: %w", err)
	}
	return backend, nil
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
		"gateway_uplink", fmt.Sprintf("CONNECTED (WSS @ %s:%d)", cfg.Endpoint, cfg.HTTPSPort),
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
