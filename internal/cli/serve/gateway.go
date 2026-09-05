// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/services/consensus"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	gateway "github.com/g8e-ai/g8e/v2/internal/services/gateway"
	govsvc "github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/logging"
	"github.com/g8e-ai/g8e/v2/internal/services/system"
)

// GatewayConfig holds configuration for starting the gateway in gateway mode.
type GatewayConfig struct {
	Posture             config.GatewayPosture
	HTTPPort            int
	HTTPSPort           int
	DataDir             string
	PKIDir              string
	SecretsDir          string
	VaultDir            string
	VaultKeyPath        string
	PasskeyRpID         string
	PasskeyRpName       string
	PasskeyRpOrigins    []string
	RateLimitRPS        float64
	RateLimitBurst      int
	LogLevel            string
	CertIdentityMode    string
	NetworkIdentityFile string
	ConsensusID         string
	ConsensusURL        string
	ConsensusBootstrap  string
	MCPDownstreamURL    string
	A2ADownstreamURL    string
	PublicBaseURL       string
	AllowedOrigins      []string
	DoctrineDir         string
}

// RunGateway starts the Operator in gateway mode - the platform's central
// persistence (operator) and pub/sub broker. In this mode, the Operator also
// runs an in-process command service to act as the sovereign execution Gateway.
func RunGateway(cfg GatewayConfig, vi VersionInfo) error {
	// Initialize paths relative to current working directory
	if err := paths.Init(); err != nil {
		return fmt.Errorf("gateway: initialize paths: %w", err)
	}

	// Construct RuntimeFileService early so all .g8e/ I/O goes through it
	initLogger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	fileSvc, err := fs.NewRuntimeFileService("", initLogger)
	if err != nil {
		return fmt.Errorf("gateway: create file service: %w", err)
	}
	if err := fileSvc.CreateRuntimeTree(context.Background()); err != nil {
		return fmt.Errorf("gateway: create runtime tree: %w", err)
	}

	logSvc := logging.NewLogService(fileSvc)
	logger, logHandle, err := logSvc.ConfigureFileLogger(context.Background(), cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("gateway: configure logger: %w", err)
	}
	defer func() {
		if closeErr := logHandle.Close(); closeErr != nil {
			logger.Error("gateway: close log handle", "error", closeErr)
		}
	}()

	// Apply defaults for empty directory flags
	if cfg.DataDir == "" {
		cfg.DataDir = fileSvc.Resolve(constants.DataDirname)
	}
	if cfg.PKIDir == "" {
		cfg.PKIDir = fileSvc.Resolve(constants.PkiDirname)
	}
	if cfg.SecretsDir == "" {
		cfg.SecretsDir = fileSvc.Resolve(constants.SecretsDirname)
	}

	logger.Info("Gateway paths configured", "data_dir", cfg.DataDir, "pki_dir", cfg.PKIDir, "secrets_dir", cfg.SecretsDir)

	logger.Info("g8e - Gateway Mode",
		"posture", cfg.Posture,
		"version", vi.Version,
		"build", vi.BuildID)

	gatewayCfg, err := config.LoadGateway(config.GatewayOptions{
		Posture:             cfg.Posture,
		HTTPPort:            cfg.HTTPPort,
		HTTPSPort:           cfg.HTTPSPort,
		DataDir:             cfg.DataDir,
		PKIDir:              cfg.PKIDir,
		SecretsDir:          cfg.SecretsDir,
		VaultDir:            cfg.VaultDir,
		VaultKeyPath:        cfg.VaultKeyPath,
		PasskeyRpID:         cfg.PasskeyRpID,
		PasskeyRpName:       cfg.PasskeyRpName,
		PasskeyRpOrigins:    cfg.PasskeyRpOrigins,
		RateLimitRPS:        cfg.RateLimitRPS,
		RateLimitBurst:      cfg.RateLimitBurst,
		CertMode:            cfg.CertIdentityMode,
		NetworkIdentityFile: cfg.NetworkIdentityFile,
		MCPDownstreamURL:    cfg.MCPDownstreamURL, // empty by default — no downstream proxy
		A2ADownstreamURL:    cfg.A2ADownstreamURL, // empty by default — no downstream proxy
		PublicBaseURL:       cfg.PublicBaseURL,
		AllowedOrigins:      cfg.AllowedOrigins,
		ConsensusID:         cfg.ConsensusID,
		ConsensusURL:        cfg.ConsensusURL,
		DoctrineDir:         cfg.DoctrineDir,
		AllowTestPortZero:   false,
	})
	if err != nil {
		return fmt.Errorf("gateway: load configuration: %w", err)
	}
	gatewayCfg.Version = vi.Version

	// Git for ledger (embedded go-git)
	gatewayCfg.GitPath = system.GitEmbedded
	gatewayCfg.GitAvailable = true

	var svc *gateway.GatewayModeService
	if cfg.ConsensusBootstrap != "" {
		db, err := gateway.OpenCanonicalDBService(gatewayCfg.Gateway.DataDir, gatewayCfg.Gateway.VaultDir, logger, gatewayCfg.Gateway.VaultKeyPath, nil, fileSvc)
		if err != nil {
			return fmt.Errorf("gateway: failed to initialize database: %w", err)
		}
		if err := consensusPolicyBootstrap(db.GetConsensusStore(), db.GetSignerStore(), cfg.ConsensusBootstrap, cfg.SecretsDir, logger); err != nil {
			return fmt.Errorf("gateway: consensus bootstrap: %w", err)
		}
		svc, err = gateway.NewGatewayModeServiceWithDB(gatewayCfg, fileSvc, logger, db, nil, nil)
		if err != nil {
			return fmt.Errorf("gateway: create service: %w", err)
		}
	} else {
		svc, err = gateway.NewGatewayModeService(gatewayCfg, fileSvc, logger)
		if err != nil {
			return fmt.Errorf("gateway: create service: %w", err)
		}
	}

	// L2 posture advisory check:
	// The gateway starts regardless of consensus configuration — L2 enforcement
	// happens at transaction time via L4Warden. If no consensus is configured yet,
	// log a warning so the operator knows L2-gated transactions will be rejected
	// until a consensus policy is enrolled.
	if cfg.Posture.RequiresL2() {
		if cfg.ConsensusID == "" {
			logger.Warn("L2 posture requires consensus but no --consensus-id set; L2-gated transactions will be rejected until a consensus is configured",
				"posture", cfg.Posture)
		} else {
			policy, err := svc.GetConsensusStore().GetConsensus(cfg.ConsensusID)
			if err != nil || policy == nil || !policy.Enabled {
				logger.Warn("L2 posture requires consensus but policy not found or disabled; L2-gated transactions will be rejected until consensus is enrolled",
					"posture", cfg.Posture, "consensus_id", cfg.ConsensusID)
			} else {
				logger.Info("Consensus policy validated", "consensus_id", cfg.ConsensusID, "members", len(policy.MemberAppIDs), "quorum", policy.Quorum)
			}
		}
	}

	// Export Actuator public key for receipt verification by evals harness
	sm, err := svc.GetSecretManager()
	if err != nil {
		return fmt.Errorf("gateway: get secret manager: %w", err)
	}

	actuatorPriv, actuatorKeyID, err := sm.GetActuatorKey()
	if err != nil {
		return fmt.Errorf("gateway: load actuator signing key: %w", err)
	}

	actuatorPub := actuatorPriv.Public().(ed25519.PublicKey)
	logger.Info("Exporting Actuator public key", "pki_dir", cfg.PKIDir, "key_id", actuatorKeyID)
	if err := govsvc.ExportActuatorPublicKey(fileSvc, actuatorPub, actuatorKeyID, logger); err != nil {
		logger.Warn("Failed to export Actuator public key for evals harness receipt verification", "error", err)
	}

	cmdSvc := svc.GetCommandService()
	if cmdSvc == nil {
		return fmt.Errorf("gateway: command service not initialized")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := svc.Start(ctx); err != nil {
			logger.Error("Gateway service failed", string(constants.ConnectionStateError), err)
			errChan <- err
		}
	}()

	// Start the command service once the gateway service is ready
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !svc.IsReady() {
			time.Sleep(100 * time.Millisecond)
			if ctx.Err() != nil {
				return
			}
		}
		logger.Info("Gateway service ready, starting in-process command service")
		if err := cmdSvc.Start(ctx); err != nil {
			logger.Error("In-process command service failed to start", string(constants.ConnectionStateError), err)
			errChan <- err
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Received signal, shutting down", "signal", sig.String())
	case err := <-errChan:
		logger.Error("Gateway service failed during operation", string(constants.ConnectionStateError), err)
	}
	cancel()
	wg.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	var shutdownErr error
	if cmdSvc.Actuator() != nil {
		logger.Info("Waiting for in-flight transactions to drain...")
		cmdSvc.Actuator().Wait()
	}
	if err := cmdSvc.Stop(); err != nil {
		logger.Error("Command service stop error", string(constants.ConnectionStateError), err)
		shutdownErr = fmt.Errorf("gateway: command service stop: %w", err)
	}

	if err := svc.Stop(shutdownCtx); err != nil {
		logger.Error("Gateway shutdown error", string(constants.ConnectionStateError), err)
		shutdownErr = fmt.Errorf("gateway: service stop: %w", err)
	}
	logger.Info("Gateway mode stopped")

	select {
	case err := <-errChan:
		return err
	default:
		return shutdownErr
	}
}

// consensusBootstrapConfig is the typed JSON config for declarative consensus
// seeding at gateway startup.
type consensusBootstrapConfig struct {
	ConsensusID  string            `json:"consensus_id"`
	MemberAppIDs []string          `json:"member_app_ids"`
	Quorum       int               `json:"quorum"`
	SeedHex      string            `json:"seed_hex"`
	MemberSeeds  map[string]string `json:"member_seeds,omitempty"`
}

// parseConsensusBootstrapConfig parses and validates a consensus bootstrap JSON
// config. Returns an error if the JSON is malformed or required fields are
// missing/invalid.
func parseConsensusBootstrapConfig(data []byte) (consensusBootstrapConfig, error) {
	var boot consensusBootstrapConfig
	if err := json.Unmarshal(data, &boot); err != nil {
		return boot, fmt.Errorf("%w: %w", constants.ErrConsensusBootstrapParseConfig, err)
	}
	if boot.ConsensusID == "" || len(boot.MemberAppIDs) == 0 || boot.Quorum < 1 {
		return boot, constants.ErrConsensusBootstrapMissingFields
	}
	return boot, nil
}

// deriveSeedPublicKey decodes a hex-encoded Ed25519 seed and returns the
// corresponding public key as a hex string. The seed must be exactly
// ed25519.SeedSize bytes.
func deriveSeedPublicKey(seedHex string) (string, error) {
	seed, err := hex.DecodeString(strings.TrimSpace(seedHex))
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrConsensusBootstrapDecodeSeed, err)
	}
	if len(seed) != ed25519.SeedSize {
		return "", fmt.Errorf("consensus bootstrap: %w: got %d, expected %d", constants.ErrInvalidSeedLength, len(seed), ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)
	return hex.EncodeToString(pub), nil
}

// consensusPolicyBootstrap seeds trusted signers and a ConsensusPolicy from a
// JSON config file. The file format is:
//
//	{
//	  "consensus_id": "fedramp-consensus",
//	  "member_app_ids": ["fedramp-csp-auditor", "fedramp-3pao", "fedramp-jab"],
//	  "quorum": 2,
//	  "member_seeds": {                          // optional, per-member keys
//	    "fedramp-csp-auditor": "<hex seed>",
//	    "fedramp-3pao": "<hex seed>",
//	    "fedramp-jab": "<hex seed>"
//	  },
//	  "seed_hex": "<hex-encoded Ed25519 seed>"    // optional, single-key fallback
//	}
//
// If member_seeds is provided, each member gets its own derived Ed25519 key
// pair: the public key is registered as a trusted signer for that member, and
// the private key is saved to secretsDir so the in-process LocalDeliberator
// signs L2 votes with distinct per-member keys via FileKeyProvider. This makes
// RequireDistinct and quorum cryptographically meaningful.
//
// If member_seeds is omitted but seed_hex is provided, the same key is
// registered for every member (single-key ensemble pattern). If both are
// omitted, a fresh key pair is generated and shared across members. The
// ConsensusPolicy is then created in the database. This is idempotent: if the
// consensus already exists, the bootstrap is skipped.
//
// Under the C2 inverted construction order, this is called BEFORE
// NewGatewayModeServiceWithDB so that build() reads the seeded policy from
// the DB and constructs the ConsensusService internally. Member private keys
// are saved to secretsDir so build()'s FileKeyProvider can load them.
func consensusPolicyBootstrap(consensusStore *gateway.ConsensusStoreService, signerStore *gateway.SignerStoreService, bootstrapPath string, secretsDir string, logger *slog.Logger) error {
	data, err := os.ReadFile(bootstrapPath)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConsensusBootstrapReadConfig, err)
	}

	boot, err := parseConsensusBootstrapConfig(data)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrConsensusBootstrapParseConfig, err)
	}

	if consensusStore == nil || signerStore == nil {
		return constants.ErrGatewayStoresNil
	}

	// Check if consensus already exists (idempotent)
	existing, err := consensusStore.GetConsensus(boot.ConsensusID)
	if err != nil {
		return fmt.Errorf("consensus bootstrap: check existing: %w", err)
	}
	if existing != nil {
		logger.Info("Consensus already exists, skipping bootstrap", "consensus_id", boot.ConsensusID)
		return nil
	}

	// Determine the key mode: per-member seeds take precedence over the
	// shared seed_hex fallback.
	usePerMemberKeys := len(boot.MemberSeeds) > 0

	if usePerMemberKeys {
		// Validate that every member has a seed.
		for _, appID := range boot.MemberAppIDs {
			if _, ok := boot.MemberSeeds[appID]; !ok {
				return fmt.Errorf("consensus bootstrap: %w: member %s has no seed in member_seeds", constants.ErrConsensusBootstrapMissingFields, appID)
			}
		}
	}

	// Derive the shared public/private key pair for the single-key fallback.
	var sharedPubHex string
	var sharedPrivKey ed25519.PrivateKey
	if !usePerMemberKeys && boot.SeedHex != "" {
		sharedPubHex, err = deriveSeedPublicKey(boot.SeedHex)
		if err != nil {
			return fmt.Errorf("consensus bootstrap: derive seed public key: %w", err)
		}
		seedBytes, err := hex.DecodeString(strings.TrimSpace(boot.SeedHex))
		if err != nil {
			return fmt.Errorf("consensus bootstrap: decode seed: %w", err)
		}
		sharedPrivKey = ed25519.NewKeyFromSeed(seedBytes)
	} else if !usePerMemberKeys {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			return fmt.Errorf("consensus bootstrap: generate key: %w", err)
		}
		sharedPubHex = hex.EncodeToString(pub)
		sharedPrivKey = priv
	}

	// Register each member as a trusted signer and save the private key to disk
	// so the in-process LocalDeliberator can sign L2 votes via FileKeyProvider.
	for _, appID := range boot.MemberAppIDs {
		var pubHex string
		var privKey ed25519.PrivateKey

		if usePerMemberKeys {
			memberSeedHex := boot.MemberSeeds[appID]
			pubHex, err = deriveSeedPublicKey(memberSeedHex)
			if err != nil {
				return fmt.Errorf("consensus bootstrap: derive member seed public key for %s: %w", appID, err)
			}
			seedBytes, decodeErr := hex.DecodeString(strings.TrimSpace(memberSeedHex))
			if decodeErr != nil {
				return fmt.Errorf("consensus bootstrap: decode member seed for %s: %w", appID, decodeErr)
			}
			privKey = ed25519.NewKeyFromSeed(seedBytes)
		} else {
			pubHex = sharedPubHex
			privKey = sharedPrivKey
		}

		signer := models.TrustedSigner{
			ID:        appID,
			PublicKey: pubHex,
			AddedAt:   time.Now().UTC(),
			Enabled:   true,
		}
		if err := signerStore.AddTrustedSigner(signer); err != nil {
			return fmt.Errorf("consensus bootstrap: register signer %s: %w", appID, err)
		}
		if err := consensus.SaveMemberKey(secretsDir, boot.ConsensusID, appID, privKey); err != nil {
			return fmt.Errorf("consensus bootstrap: save member key %s: %w", appID, err)
		}
		logger.Info("Trusted signer registered and key saved", "app_id", appID)
	}

	// Create the ConsensusPolicy
	policy := models.ConsensusPolicy{
		ID:              boot.ConsensusID,
		MemberAppIDs:    boot.MemberAppIDs,
		Quorum:          boot.Quorum,
		RequireDistinct: true,
		Enabled:         true,
	}
	if err := consensusStore.AddConsensus(policy); err != nil {
		return fmt.Errorf("consensus bootstrap: create policy: %w", err)
	}
	logger.Info("Consensus policy created", "consensus_id", boot.ConsensusID, "members", len(boot.MemberAppIDs), "quorum", boot.Quorum)
	return nil
}
