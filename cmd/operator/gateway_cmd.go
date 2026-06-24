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

package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/exitcode"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/execution"
	gateway "github.com/g8e-ai/g8e/internal/services/gateway"
	govsvc "github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/system"
	"github.com/g8e-ai/g8e/internal/services/tribunal"
)

// GatewayStartConfig holds configuration for starting the gateway in gateway mode.
type GatewayStartConfig struct {
	Posture             config.GatewayPosture
	HTTPPort            int
	HTTPSPort           int
	DataDir             string
	PKIDir              string
	SecretsDir          string
	VaultDir            string
	VaultKeyPath        string
	VaultRequireUnlock  bool
	PasskeyRpID         string
	PasskeyRpName       string
	RateLimitRPS        float64
	RateLimitBurst      int
	LogLevel            string
	CertIdentityMode    string
	NetworkIdentityFile string
	TribunalID          string
	TribunalURL         string
}

// runGatewayMode starts the Operator in gateway mode - the platform's central
// persistence (operator) and pub/sub broker. In this mode, the Operator also
// runs an in-process command service to act as the sovereign execution Gateway.
func runGatewayMode(cfg GatewayStartConfig) {
	// Initialize paths relative to current working directory
	if err := paths.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize paths: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	// Create log directory and file
	if err := os.MkdirAll(paths.Infra.LogDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}

	logHandle, err := os.OpenFile(paths.Infra.OperatorLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(constants.ExitConfigError)
	}
	defer logHandle.Close()

	logger, err := configureLoggerWithOutput(cfg.LogLevel, logHandle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", cfg.LogLevel, err)
		os.Exit(constants.ExitConfigError)
	}

	// Apply defaults for empty directory flags (constants are now absolute)
	if cfg.DataDir == "" {
		cfg.DataDir = paths.Infra.DataDir
	}
	if cfg.PKIDir == "" {
		cfg.PKIDir = paths.Infra.PkiDir
	}
	if cfg.SecretsDir == "" {
		cfg.SecretsDir = paths.Infra.SecretsDir
	}

	logger.Info("Gateway paths configured", "data_dir", cfg.DataDir, "pki_dir", cfg.PKIDir, "secrets_dir", cfg.SecretsDir)

	logger.Info("g8e - Gateway Mode",
		"posture", cfg.Posture,
		"version", version,
		"build", buildID)

	gatewayCfg, err := config.LoadGateway(config.GatewayOptions{
		Posture:             cfg.Posture,
		HTTPPort:            cfg.HTTPPort,
		HTTPSPort:           cfg.HTTPSPort,
		DataDir:             cfg.DataDir,
		PKIDir:              cfg.PKIDir,
		SecretsDir:          cfg.SecretsDir,
		PasskeyRpID:         cfg.PasskeyRpID,
		PasskeyRpName:       cfg.PasskeyRpName,
		RateLimitRPS:        cfg.RateLimitRPS,
		RateLimitBurst:      cfg.RateLimitBurst,
		CertMode:            cfg.CertIdentityMode,
		NetworkIdentityFile: cfg.NetworkIdentityFile,
		MCPDownstreamURL:    "",
		A2ADownstreamURL:    "",
		TribunalID:          cfg.TribunalID,
		TribunalURL:         cfg.TribunalURL,
		AllowTestPortZero:   false,
	})
	if err != nil {
		logger.Error("Failed to load gateway configuration", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitConfigError)
	}
	gatewayCfg.Version = version

	svc, err := gateway.NewGatewayModeService(gatewayCfg, logger)
	if err != nil {
		logger.Error("Failed to create gateway service", string(constants.ConnectionStateError), err)
		os.Exit(exitcode.FromError(err))
	}

	// Startup validation for consensus posture (Phase 5.4):
	// If posture is consensus, TribunalID must be set and the TribunalPolicy
	// must exist and be enabled in the database. Fail fast before starting
	// any services.
	if err := config.ValidateConsensusStartup(string(cfg.Posture), cfg.TribunalID, 0); err != nil {
		logger.Error("Startup validation failed", string(constants.ConnectionStateError), err)
		os.Exit(constants.ExitConfigError)
	}
	if cfg.Posture == config.PostureConsensus {
		policy, err := svc.GetDB().TribunalStore.GetTribunal(cfg.TribunalID)
		if err != nil {
			logger.Error("Failed to load Tribunal policy", "tribunal_id", cfg.TribunalID, string(constants.ConnectionStateError), err)
			os.Exit(exitcode.FromError(err))
		}
		if policy == nil || !policy.Enabled {
			logger.Error("Tribunal policy not found or disabled", "tribunal_id", cfg.TribunalID, string(constants.ConnectionStateError), constants.ErrTxL2TribunalNotConfigured)
			os.Exit(constants.ExitConfigError)
		}
		if err := config.ValidateConsensusStartup(string(cfg.Posture), cfg.TribunalID, policy.Quorum); err != nil {
			logger.Error("Startup validation failed", "tribunal_id", cfg.TribunalID, "quorum", policy.Quorum, string(constants.ConnectionStateError), err)
			os.Exit(constants.ExitConfigError)
		}
		logger.Info("Tribunal policy validated", "tribunal_id", cfg.TribunalID, "members", len(policy.MemberAppIDs), "quorum", policy.Quorum)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize In-Process Execution Gateway
	logger.Info("Initializing in-process execution Gateway...")
	execSvc := execution.NewExecutionService(gatewayCfg, logger)
	fileSvc := execution.NewFileEditService(gatewayCfg, logger)

	// Resolve Git for ledger
	gitPath := system.ResolveGitBinary(logger)
	gatewayCfg.GitPath = gitPath
	gatewayCfg.GitAvailable = gitPath != ""

	// Use the gateway-mode database for everything
	govDeps := svc.GetGovernanceDeps()
	sm, err := svc.GetSecretManager()
	if err != nil {
		logger.Error("Failed to get secret manager", string(constants.ConnectionStateError), err)
		cancel()
		os.Exit(constants.ExitConfigError)
	}

	ActuatorPriv, ActuatorKeyID, err := sm.GetActuatorKey()
	if err != nil {
		logger.Error("Failed to load Actuator signing key - mutations will fail", string(constants.ConnectionStateError), err)
		cancel()
		os.Exit(constants.ExitConfigError)
	}

	// Export Actuator public key for receipt verification by evals harness
	ActuatorPub := ActuatorPriv.Public().(ed25519.PublicKey)
	logger.Info("Exporting Actuator public key", "pki_dir", cfg.PKIDir, "key_id", ActuatorKeyID)
	if err := exportActuatorPublicKey(cfg.PKIDir, ActuatorPub, ActuatorKeyID, logger); err != nil {
		logger.Warn("Failed to export Actuator public key for evals harness receipt verification", "error", err)
	}

	// Loopback Pub/Sub for in-process command dispatch
	loopbackClient := pubsub.NewInProcessPubSubClient(svc.GetHTTPHandler().GetGatewayWebSocketHandler())

	// Resolve the MCP gateway up-front so the pubsub command service can
	// reach it for Actuator egress dispatch on verified MCP_CALL transactions.
	mcpSvc := svc.GetHTTPHandler().GetMCPGateway()

	// Get the GatewayDBService's AuditStore for full audit storage
	// This ensures ActionReceipts are persisted in the receipts table
	var auditStore *storage.SQLAuditStore
	if svc.GetDB() != nil && svc.GetDB().AuditStore != nil {
		auditStore = svc.GetDB().AuditStore
		logger.Info("Gateway AuditStore enabled for full audit storage")
	} else {
		logger.Warn("Gateway AuditStore not available - ActionReceipts will not be stored in audit store")
	}

	psConfig := pubsub.CommandServiceConfig{
		Config:             gatewayCfg,
		Logger:             logger,
		Execution:          execSvc,
		FileEdit:           fileSvc,
		PubSubClient:       loopbackClient,
		ResultsService:     nil, // Results handled via direct loopback publish if needed
		ExecutionVault:     nil, // Not used in gateway mode
		AuditStore:         auditStore,
		Ledger:             nil, // P1: Ledger in gateway mode
		HistoryHandler:     nil, // P1: History in gateway mode
		Scrubbing:          scrubbing.NewScrubbingService(scrubbing.DefaultConfig(), logger, nil),
		ReplayStore:        govDeps.ReplayStore,
		StateRootProvider:  govDeps.StateRootProvider,
		TransactionAudit:   govDeps.TransactionAudit,
		FieldReader:        govDeps.FieldReader,
		SignerStore:        govDeps.SignerStore,
		AppPolicyStore:     govDeps.AppPolicyStore,
		TribunalStore:      govDeps.TribunalStore,
		L3Notary:           govDeps.L3Notary,
		ActuatorSigningKey: ActuatorPriv,
		ActuatorKeyID:      ActuatorKeyID,
		MCPGateway:         mcpSvc,
	}

	cmdSvc, err := pubsub.NewOperatorPubSubService(psConfig)
	if err != nil {
		logger.Error("Failed to initialize in-process command service", string(constants.ConnectionStateError), err)
		os.Exit(exitcode.FromError(err))
	}

	// Wire the synchronous fail-closed mutation gate into the gateway HTTP
	// surface. Once set, BYO clients can POST GovernanceEnvelope envelopes to
	// /api/v1/governance/envelopes and receive a signed ActionReceipt.
	svc.SetEnvelopeProcessor(cmdSvc)

	// The MCP gateway's runtime governance dependencies (gateway processor,
	// signing identity, audit logger, etc.) are wired by NewOperatorPubSubService
	// via initializeGovernance, which received mcpSvc through psConfig.MCPGateway.
	// No additional gateway wiring is needed here.

	// Bootstrap Tribunal service for consensus posture (Phase 5.2):
	// Construct the TribunalService in-process and wire it both as the mTLS
	// HTTP handler (for remote deliberation calls) and as the local deliberator
	// (for in-process envelope processing). Under doctrine/notary posture,
	// the Tribunal is not constructed.
	if cfg.Posture == config.PostureConsensus && cfg.TribunalID != "" {
		tribunalSvc, err := bootstrapTribunal(svc, cfg.TribunalID, ActuatorPriv, ActuatorKeyID, logger)
		if err != nil {
			logger.Error("Failed to bootstrap Tribunal service", string(constants.ConnectionStateError), err)
			cancel()
			os.Exit(exitcode.FromError(err))
		}
		svc.SetTribunal(tribunalSvc)
		mcpSvc.SetTribunalDeliberator(tribunal.NewLocalDeliberator(tribunalSvc))
		logger.Info("Tribunal service bootstrapped", "tribunal_id", cfg.TribunalID)
	}

	go func() {
		if err := svc.Start(ctx); err != nil {
			logger.Error("Gateway service failed", string(constants.ConnectionStateError), err)
			os.Exit(exitcode.FromError(err))
		}
	}()

	// Start the command service once the gateway service is ready
	go func() {
		for !svc.IsReady() {
			time.Sleep(100 * time.Millisecond)
			if ctx.Err() != nil {
				return
			}
		}
		logger.Info("Gateway service ready, starting in-process command service")
		if err := cmdSvc.Start(ctx); err != nil {
			logger.Error("In-process command service failed to start", string(constants.ConnectionStateError), err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if cmdSvc != nil {
		if cmdSvc.Actuator() != nil {
			logger.Info("Waiting for in-flight transactions to drain...")
			cmdSvc.Actuator().Wait()
		}
		if err := cmdSvc.Stop(); err != nil {
			logger.Error("Command service stop error", string(constants.ConnectionStateError), err)
		}
	}

	if err := svc.Stop(shutdownCtx); err != nil {
		logger.Error("Gateway shutdown error", string(constants.ConnectionStateError), err)
	}
	logger.Info("Gateway mode stopped")
}

// bootstrapTribunal constructs a TribunalService from the TribunalPolicy stored
// in the database. For single-member tribunals, the gateway's actuator signing
// key is used as the member private key (Option C from the design doc). Multi-
// member tribunals require a separate key provisioning flow (not yet implemented).
func bootstrapTribunal(svc *gateway.GatewayModeService, tribunalID string, actuatorPriv ed25519.PrivateKey, actuatorKeyID string, logger *slog.Logger) (*tribunal.TribunalService, error) {
	policy, err := svc.GetDB().TribunalStore.GetTribunal(tribunalID)
	if err != nil {
		return nil, fmt.Errorf("bootstrap tribunal: load policy: %w", err)
	}
	if policy == nil {
		return nil, fmt.Errorf("bootstrap tribunal: %w: %s", constants.ErrTxL2TribunalNotConfigured, tribunalID)
	}

	// Option C: For single-member tribunal, use the gateway's actuator key.
	// The actuator's public key must already be registered as a TrustedSigner
	// with keyID = policy.MemberAppIDs[0].
	var members []tribunal.TribunalMember
	for _, appID := range policy.MemberAppIDs {
		if appID == actuatorKeyID {
			members = append(members, tribunal.TribunalMember{
				AppID:      appID,
				PrivateKey: actuatorPriv,
			})
		} else {
			logger.Warn("Tribunal member lacks private key (multi-member not yet supported)",
				"member_app_id", appID,
				"actuator_key_id", actuatorKeyID)
			members = append(members, tribunal.TribunalMember{
				AppID: appID,
			})
		}
	}

	doctrine := govsvc.NewL1Doctrine()
	responder := response.NewWriter(logger)

	return tribunal.NewTribunalService(tribunalID, members, doctrine, logger, responder), nil
}
