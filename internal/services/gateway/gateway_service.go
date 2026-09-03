// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/response"
	"github.com/g8e-ai/g8e/v2/internal/services/consensus"
	"github.com/g8e-ai/g8e/v2/internal/services/execution"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
)

// GatewayModeService is the top-level orchestrator for gateway mode (operator).
// It acts as the platform's central persistence and messaging backbone.
// In this mode, the Operator does NOT execute commands or initiate outbound
// connections. It strictly serves inbound requests from platform components.
type GatewayModeService struct {
	cfg      *config.Config
	logger   *slog.Logger
	fileSvc  fs.RuntimeFileService
	doctrine *governance.L1Doctrine

	db                    *CanonicalDBService
	docStore              *DocumentStoreService
	consensusStore        *ConsensusStoreService
	signerStore           *SignerStoreService
	auditStore            *storage.SQLAuditStore
	stateRootSvc          *StateRootService
	kvStore               *KVStoreService
	replayStore           *ReplayStoreService
	pubsub                *GatewayWebSocketHandler
	auth                  *AuthService
	pki                   *PKIAuthority
	reg                   *RegistrationService
	passkey               *PasskeyHandler
	enrollmentTokenSvc    *EnrollmentTokenService
	userSvc               *UserService
	cliSessionSvc         *CLISessionService
	operatorSessionSvc    *OperatorSessionService
	webSessionSvc         *WebSessionService
	suspendedTxService    *storage.SuspendedTransactionService
	mcpGateway            *mcp.GatewayService
	cmdSvc                *pubsub.OperatorPubSubService
	envProc               governance.EnvelopeProcessor
	platformEnrollmentSvc *PlatformEnrollmentService
	consensusSvc          *consensus.ConsensusService
	dispatchSvc           *DispatchService
	responder             *response.Writer
	server                *http.Server
	publicServer          *http.Server

	handler *HTTPHandler

	extraIPs []net.IP

	mu      sync.Mutex
	running bool
	ready   bool
}

// gatewayServiceBuilder constructs a GatewayModeService from configuration.
type gatewayServiceBuilder struct {
	cfg          *config.Config
	logger       *slog.Logger
	fileSvc      fs.RuntimeFileService
	db           *CanonicalDBService
	consensusSvc *consensus.ConsensusService
	deliberator  *consensus.LocalDeliberator
	cmdSvc       *pubsub.OperatorPubSubService
}

// newGatewayServiceBuilder creates a builder for production use.
func newGatewayServiceBuilder(cfg *config.Config, fileSvc fs.RuntimeFileService, logger *slog.Logger) *gatewayServiceBuilder {
	return &gatewayServiceBuilder{cfg: cfg, logger: logger, fileSvc: fileSvc}
}

// withDB configures an existing database.
func (b *gatewayServiceBuilder) withDB(db *CanonicalDBService) *gatewayServiceBuilder {
	b.db = db
	return b
}

// withConsensus configures pre-bootstrapped consensus service and deliberator.
func (b *gatewayServiceBuilder) withConsensus(cs *consensus.ConsensusService, delib *consensus.LocalDeliberator) *gatewayServiceBuilder {
	b.consensusSvc = cs
	b.deliberator = delib
	return b
}

// build assembles the GatewayModeService from the builder's configuration.
// SecretManager lifecycle: sm is obtained from CanonicalDBService via
// GetSecretManager() and passed to PKIAuthority, but is not retained on
// GatewayModeService. CanonicalDBService owns the SecretManager lifecycle
// (initialized in initSchema, closed in CanonicalDBService.Close).
func (b *gatewayServiceBuilder) build() (*GatewayModeService, error) {
	cfg := b.cfg
	logger := b.logger

	// --- DB and pubsub ---
	var db *CanonicalDBService
	var stores *Stores
	var err error
	if b.db != nil {
		db = b.db
		stores = b.db.GetStores()
	} else {
		db, stores, err = OpenCanonicalDBService(cfg.Gateway.DataDir, cfg.Gateway.VaultDir, logger, cfg.Gateway.VaultKeyPath, nil, b.fileSvc)
		if err != nil {
			return nil, fmt.Errorf("gateway: failed to initialize database: %w", err)
		}
	}

	wsHandler := NewGatewayWebSocketHandler(logger)

	// --- Secret manager ---
	sm := db.GetSecretManager()

	// --- PKI ---
	pki := newPKIAuthority(b.fileSvc, stores.DocStore, sm, logger)

	// --- Core services ---
	userSvc := NewUserService(stores.DocStore, logger)
	res := response.NewWriter(logger)

	var jwksProvider *JWKSProvider
	if cfg.Gateway.JWKSURL != "" {
		jwksProvider = NewJWKSProvider(cfg.Gateway.JWKSURL)
	}

	personaSvc := NewPersonaService(stores.DocStore, logger)
	for _, persona := range DefaultPersonaDefinitions() {
		existing, err := personaSvc.GetByID(persona.ID)
		if err != nil {
			return nil, fmt.Errorf("gateway: failed to check existing persona %s: %w", persona.ID, err)
		}
		if existing == nil {
			if err := personaSvc.CreatePersona(&persona); err != nil {
				return nil, fmt.Errorf("gateway: failed to create persona %s: %w", persona.ID, err)
			}
		}
	}

	jwtRoleClaim := cfg.Gateway.JWTRoleClaim
	jwtIssuer := cfg.Gateway.JWTIssuer
	jwtAudience := cfg.Gateway.JWTAudience
	auth := NewAuthService(stores.DocStore, pki, logger, userSvc, personaSvc, res, jwksProvider, jwtRoleClaim, jwtIssuer, jwtAudience)
	userSvc.SetAuthService(auth)

	// Wire the pubsub command relay dependencies: the gateway intercepts
	// app-published command intent on cmd: channels, validates the target
	// operator session, fetches the current state root, and constructs the
	// governed GovernanceEnvelope before fan-out.
	wsHandler.SetCommandRelayDeps(stores.StateRootSvc, auth, string(cfg.Gateway.Posture))

	// Wire the pubsub receipt relay dependencies: the gateway intercepts
	// operator-published signed ActionReceipts on receipts: channels,
	// verifies the receipt signature against the operator's actuator public
	// key (via the SignerStore), records the receipt in the gateway's
	// SQLAuditStore, and fans out the envelope to subscribers.
	wsHandler.SetReceiptRelayDeps(stores.SignerStore, stores.AuditStore)

	cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
	operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
	webSessionSvc := NewWebSessionService(stores.DocStore, logger)

	// --- Certificate identity and PKI initialization ---
	extraIPs, extraDNSNames, err := resolveGatewayCertificateIdentity(cfg.Gateway.CertMode, cfg.Gateway.NetworkIdentityFile, network.NewDetector(logger), logger)
	if err != nil {
		return nil, err
	}
	if len(extraDNSNames) > 0 {
		if err := pki.InitializePKIWithNames(extraIPs, extraDNSNames); err != nil {
			return nil, fmt.Errorf("gateway: failed to initialize PKI hierarchy: %w", err)
		}
	} else {
		if err := pki.InitializePKI(extraIPs); err != nil {
			return nil, fmt.Errorf("gateway: failed to initialize PKI hierarchy: %w", err)
		}
	}

	reg := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)

	// --- Passkey ---
	passkeyCfg := &PasskeyConfig{
		RpID:      cfg.Gateway.PasskeyRpID,
		RpName:    cfg.Gateway.PasskeyRpName,
		RpOrigins: cfg.Gateway.PasskeyRpOrigins,
		HTTPPort:  cfg.Gateway.HTTPPort,
		HTTPSPort: cfg.Gateway.HTTPSPort,
	}
	passkey, err := NewPasskeyService(stores.DocStore, logger, passkeyCfg)
	if err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize passkey service: %w", err)
	}

	// --- Suspended transaction service ---
	suspendedTxConfig := &storage.SuspendedTransactionConfig{
		DBPath:               paths.GetSuspendedTransactionsDBPath(cfg.Gateway.DataDir),
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}
	suspendedTxService, err := storage.NewSuspendedTransactionService(suspendedTxConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize suspended transaction service: %w", err)
	}

	// --- Scrubbing and MCP gateway ---
	scrubbingConfig := scrubbing.DefaultConfig()
	scrubbingService, err := scrubbing.NewScrubbingService(context.Background(), scrubbingConfig, logger, nil)
	if err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize scrubbing service: %w", err)
	}

	publicBaseURL := cfg.Gateway.PublicBaseURL
	if publicBaseURL == "" {
		publicBaseURL = network.LocalhostHTTPSURL(cfg.Gateway.HTTPSPort)
	}

	doctrine, err := governance.NewL1DoctrineFromDir(cfg.Gateway.DoctrineDir)
	if err != nil {
		return nil, fmt.Errorf("gateway: load doctrine: %w", err)
	}

	actuatorPriv, actuatorKeyID, err := sm.GetActuatorKey()
	if err != nil {
		return nil, fmt.Errorf("gateway: load actuator signing key: %w", err)
	}
	auditorPriv, auditorKeyID, err := sm.GetAuditorKey()
	if err != nil {
		return nil, fmt.Errorf("gateway: load auditor signing key: %w", err)
	}

	// --- Consensus bootstrap (C2 inverted order: before pubsub and mcpGateway) ---
	consensusSvc := b.consensusSvc
	l2Deliberator := b.deliberator
	if consensusSvc == nil && cfg.Gateway.Posture.RequiresL2() && cfg.Gateway.ConsensusID != "" {
		policy, err := stores.ConsensusStore.GetConsensus(cfg.Gateway.ConsensusID)
		if err == nil && policy != nil {
			fileProvider := consensus.NewFileKeyProvider(cfg.Gateway.SecretsDir, cfg.Gateway.ConsensusID)
			keyProvider := consensus.KeyProviderFunc(func(appID string) (ed25519.PrivateKey, error) {
				if key, err := fileProvider.GetMemberKey(appID); err == nil {
					return key, nil
				}
				if appID == actuatorKeyID {
					return actuatorPriv, nil
				}
				return nil, fmt.Errorf("bootstrap consensus: %w: %s", constants.ErrConsensusMemberKeyNotFound, appID)
			})
			cs, err := consensus.NewConsensusFromPolicy(policy, keyProvider, doctrine, logger, res)
			if err == nil {
				consensusSvc = cs
				l2Deliberator = consensus.NewLocalDeliberator(consensusSvc)
			}
		}
	}

	// --- Command Service (OperatorPubSubService) ---
	cmdSvc := b.cmdSvc
	if cmdSvc == nil {
		cliVerifier := NewCLISessionVerifier(stores.DocStore, pki, logger, userSvc, cliSessionSvc)
		l3Notary := governance.NewGatewayL3Notary(cliVerifier, passkey, logger)
		platformDeps := &pubsub.PlatformEnrollmentDeps{
			DocStore:         stores.DocStore,
			PKI:              pki,
			CLISessions:      cliSessionSvc,
			OperatorSessions: operatorSessionSvc,
			Posture:          string(cfg.Gateway.Posture),
		}
		govCore := pubsub.GovernanceCoreDeps{
			ReplayStore:       stores.ReplayStore,
			StateRootProvider: stores.StateRootSvc,
			TransactionAudit:  stores.DocStore,
			L3Notary:          l3Notary,
			SignerStore:       stores.SignerStore,
			Doctrine:          doctrine,
		}
		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)
		loopbackClient := pubsub.NewInProcessPubSubClient(wsHandler)

		govModeDeps := &pubsub.GatewayModeDeps{
			GovernanceCoreDeps:     govCore,
			GovernedDocStore:       stores.DocStore,
			ConsensusPolicyStore:   stores.ConsensusStore,
			FieldReader:            stores.DocStore,
			Consensus:              consensusSvc,
			PlatformEnrollmentDeps: platformDeps,
			Posture:                cfg.Gateway.Posture,
		}

		cmdSvc, err = pubsub.NewGatewayOperatorPubSubService(pubsub.GatewayCommandServiceConfig{
			CommandServiceConfig: pubsub.CommandServiceConfig{
				Config:             cfg,
				Logger:             logger,
				Execution:          execSvc,
				FileEdit:           fileEditSvc,
				PubSubClient:       loopbackClient,
				AuditStore:         stores.AuditStore,
				Scrubbing:          scrubbingService,
				ActuatorSigningKey: actuatorPriv,
				ActuatorKeyID:      actuatorKeyID,
				AuditorSigningKey:  auditorPriv,
				AuditorKeyID:       auditorKeyID,
			},
			GovDeps: govModeDeps,
		})
		if err != nil {
			return nil, fmt.Errorf("gateway: failed to initialize command service: %w", err)
		}
	}

	// --- Platform enrollment service (C2: concrete cmdSvc injected) ---
	platformEnrollmentSvc := NewPlatformEnrollmentService(stores.DocStore, userSvc, cmdSvc, stores.StateRootSvc, string(cfg.Gateway.Posture), logger)

	// --- MCP gateway (C2: concrete cmdSvc, auditLogger, l2Deliberator injected) ---
	var auditLogger mcp.AuditLogger
	if stores.AuditStore != nil {
		auditLogger = pubsub.NewAuditLogger(stores.AuditStore, logger)
	}

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:                 logger,
		Responder:              res,
		SuspendedStore:         suspendedTxService,
		ScrubbingService:       scrubbingService,
		ThreatScanner:          doctrine,
		MaxPayloadBytes:        cfg.Gateway.MaxPayloadBytes,
		Posture:                string(cfg.Gateway.Posture),
		A2ADownstreamURL:       cfg.Gateway.A2ADownstreamURL,
		PublicBaseURL:          publicBaseURL,
		AuditStore:             stores.AuditStore,
		AuditReceiptQuery:      stores.AuditStore,
		EnvProc:                cmdSvc,
		StateRootProvider:      stores.StateRootSvc,
		SigningKey:             actuatorPriv,
		KeyID:                  actuatorKeyID,
		DownstreamURL:          cfg.Gateway.MCPDownstreamURL,
		DBService:              stores.DocStore,
		SessionValidator:       cmdSvc,
		AuditLogger:            auditLogger,
		L2ConsensusDeliberator: l2Deliberator,
	})
	if err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize MCP gateway: %w", err)
	}

	// The command service processes MCP envelopes for the MCP gateway, while the MCP gateway performs verified command-service egress. Complete this unavoidable construction cycle exactly once before either service starts.
	if err := cmdSvc.BindMCPGateway(mcpGateway); err != nil {
		return nil, fmt.Errorf("gateway: bind MCP gateway to command service: %w", err)
	}

	// --- Passkey orchestrator and handler ---
	passkeyOrchestrator, err := NewPasskeyOrchestrator(mcpGateway, suspendedTxService, stores.SSEStore, wsHandler, logger)
	if err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize passkey orchestrator: %w", err)
	}
	enrollmentTokenSvc := NewEnrollmentTokenService(stores.DocStore, logger)
	passkeyHandler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:            passkey,
		WebSessionSvc:      webSessionSvc,
		EnrollmentTokenSvc: enrollmentTokenSvc,
		Responder:          res,
		MaxPayload:         cfg.Gateway.MaxPayloadBytes,
		Orchestrator:       passkeyOrchestrator,
	})

	ls := &GatewayModeService{
		cfg:                   cfg,
		logger:                logger,
		fileSvc:               b.fileSvc,
		doctrine:              doctrine,
		db:                    db,
		docStore:              stores.DocStore,
		consensusStore:        stores.ConsensusStore,
		signerStore:           stores.SignerStore,
		auditStore:            stores.AuditStore,
		stateRootSvc:          stores.StateRootSvc,
		kvStore:               stores.KVStore,
		replayStore:           stores.ReplayStore,
		pubsub:                wsHandler,
		auth:                  auth,
		pki:                   pki,
		reg:                   reg,
		passkey:               passkeyHandler,
		enrollmentTokenSvc:    enrollmentTokenSvc,
		userSvc:               userSvc,
		cliSessionSvc:         cliSessionSvc,
		operatorSessionSvc:    operatorSessionSvc,
		webSessionSvc:         webSessionSvc,
		suspendedTxService:    suspendedTxService,
		extraIPs:              extraIPs,
		mcpGateway:            mcpGateway,
		cmdSvc:                cmdSvc,
		envProc:               cmdSvc,
		platformEnrollmentSvc: platformEnrollmentSvc,
		consensusSvc:          consensusSvc,
		dispatchSvc:           NewDispatchService(logger, wsHandler, stores.StateRootSvc, auth, string(cfg.Gateway.Posture)),
		responder:             res,
	}

	// Build the HTTP handler and servers now that all dependencies are constructed.
	if err := ls.initHTTPHandler(); err != nil {
		return nil, fmt.Errorf("gateway: initialize HTTP handler: %w", err)
	}

	return ls, nil
}

// NewGatewayModeService creates a new gateway mode service.
func NewGatewayModeService(cfg *config.Config, fileSvc fs.RuntimeFileService, logger *slog.Logger) (*GatewayModeService, error) {
	return newGatewayServiceBuilder(cfg, fileSvc, logger).build()
}

// NewGatewayModeServiceWithConsensus creates a new gateway mode service with pre-configured consensus.
func NewGatewayModeServiceWithConsensus(cfg *config.Config, fileSvc fs.RuntimeFileService, logger *slog.Logger, cs *consensus.ConsensusService, delib *consensus.LocalDeliberator) (*GatewayModeService, error) {
	return newGatewayServiceBuilder(cfg, fileSvc, logger).withConsensus(cs, delib).build()
}

// NewGatewayModeServiceWithDB creates a new gateway mode service with existing DB and pre-configured consensus.
func NewGatewayModeServiceWithDB(cfg *config.Config, fileSvc fs.RuntimeFileService, logger *slog.Logger, db *CanonicalDBService, cs *consensus.ConsensusService, delib *consensus.LocalDeliberator) (*GatewayModeService, error) {
	return newGatewayServiceBuilder(cfg, fileSvc, logger).withDB(db).withConsensus(cs, delib).build()
}

type networkIdentityDetector interface {
	DetectAll(context.Context) (*network.NetworkIdentity, error)
}

func resolveGatewayCertificateIdentity(certMode, identityFile string, detector networkIdentityDetector, logger *slog.Logger) ([]net.IP, []string, error) {
	switch certMode {
	case "localhost":
		return resolveLocalhostCertificateIdentity(detector, logger)
	default:
		return resolveFullCertificateIdentity(identityFile, detector, logger)
	}
}

func resolveLocalhostCertificateIdentity(detector networkIdentityDetector, logger *slog.Logger) ([]net.IP, []string, error) {
	logger.Info("Using localhost-only mode for certificate")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	netIdentity, err := detector.DetectAll(ctx)
	if err != nil {
		logger.Warn("Failed to detect localhost identities, using defaults", "error", err)
		return []net.IP{net.ParseIP("127.0.0.1")}, []string{"localhost"}, nil
	}

	var extraIPs []net.IP
	for _, ip := range netIdentity.GetAllIPs() {
		if ip.IsLoopback() {
			extraIPs = append(extraIPs, ip)
		}
	}
	return extraIPs, []string{"localhost"}, nil
}

func resolveFullCertificateIdentity(identityFile string, detector networkIdentityDetector, logger *slog.Logger) ([]net.IP, []string, error) {
	if identityFile != "" {
		logger.Info("Using pre-detected network identity from file", "file", identityFile)
		identityData, err := os.ReadFile(identityFile)
		if err != nil {
			return nil, nil, fmt.Errorf("gateway: failed to read network identity file: %w", err)
		}

		var netIdentity network.NetworkIdentity
		if err := json.Unmarshal(identityData, &netIdentity); err != nil {
			return nil, nil, fmt.Errorf("gateway: failed to unmarshal network identity: %w", err)
		}

		extraIPs := netIdentity.GetAllIPs()
		extraDNSNames := netIdentity.GetAllDNSNames()
		logger.Info("Network identity loaded from file for certificate", "dns_names", len(extraDNSNames), "ips", len(extraIPs))
		return extraIPs, extraDNSNames, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	netIdentity, err := detector.DetectAll(ctx)
	if err != nil {
		logger.Warn("Failed to detect full network identity, falling back to basic IP detection", "error", err)
		extraIPs := detectBasicNonLoopbackIPv4Addresses()
		return extraIPs, nil, nil
	}

	extraIPs := netIdentity.GetAllIPs()
	extraDNSNames := netIdentity.GetAllDNSNames()
	logger.Info("Network identity detected for certificate", "dns_names", len(extraDNSNames), "ips", len(extraIPs))
	return extraIPs, extraDNSNames, nil
}

func detectBasicNonLoopbackIPv4Addresses() []net.IP {
	var extraIPs []net.IP
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
					extraIPs = append(extraIPs, ip)
				}
			}
		}
	}
	return extraIPs
}

// initHTTPHandler is the internal constructor for the HTTP handler and
// servers. Called once by NewGatewayModeService after all dependencies
// are constructed.
func (ls *GatewayModeService) initHTTPHandler() error {
	cfg := ls.cfg
	logger := ls.logger
	pubsub := ls.pubsub
	auth := ls.auth
	pki := ls.pki
	cliSessionSvc := ls.cliSessionSvc
	operatorSessionSvc := ls.operatorSessionSvc
	reg := ls.reg
	passkey := ls.passkey
	userSvc := ls.userSvc

	envProc := ls.envProc
	if envProc == nil && ls.cmdSvc != nil {
		envProc = ls.cmdSvc
	}

	// Initialize AppEnrollmentService for external app enrollment
	appEnrollment := NewAppEnrollmentService(ls.docStore, pki, logger)

	handler, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:    cfg,
		Logger: logger,
		Auth:   auth,
		PKIControllerDeps: PKIControllerDeps{
			Cfg:           cfg,
			Logger:        logger,
			PKI:           pki,
			AppEnrollment: appEnrollment,
			Registration:  reg,
			Responder:     ls.responder,
		},
		AuditControllerDeps: AuditControllerDeps{
			Cfg:        cfg,
			Logger:     logger,
			AuditStore: ls.db.GetStores().AuditStore,
			Responder:  ls.responder,
		},
		DataControllerDeps: DataControllerDeps{
			Cfg:       cfg,
			Logger:    logger,
			DocStore:  ls.db.GetStores().DocStore,
			KVStore:   ls.db.GetStores().KVStore,
			SSEStore:  ls.db.GetStores().SSEStore,
			BlobStore: ls.db.GetStores().BlobStore,
			Pubsub:    pubsub,
			Responder: ls.responder,
		},
		SignerControllerDeps: SignerControllerDeps{
			Cfg:         cfg,
			Logger:      logger,
			DocStore:    ls.db.GetStores().DocStore,
			SignerStore: ls.db.GetStores().SignerStore,
			Responder:   ls.responder,
		},
		BootstrapControllerDeps: BootstrapControllerDeps{
			Cfg:                cfg,
			Logger:             logger,
			DocStore:           ls.docStore,
			UserSvc:            userSvc,
			PKI:                pki,
			CLISessionSvc:      cliSessionSvc,
			OperatorSessionSvc: operatorSessionSvc,
			Responder:          ls.responder,
			ActuatorKeyReader:  &fileActuatorKeyReader{path: paths.Infra.ActuatorPubJSONPath},
		},
		CLIRecoveryControllerDeps: CLIRecoveryControllerDeps{
			Cfg:                cfg,
			Logger:             logger,
			UserSvc:            userSvc,
			PKI:                pki,
			CLISessionSvc:      cliSessionSvc,
			OperatorSessionSvc: operatorSessionSvc,
			DocStore:           ls.docStore,
			Responder:          ls.responder,
		},
		CLIRotationControllerDeps: CLIRotationControllerDeps{
			Cfg:           cfg,
			Logger:        logger,
			PKI:           pki,
			CLISessionSvc: cliSessionSvc,
			UserSvc:       userSvc,
			Responder:     ls.responder,
		},
		CLIRefreshControllerDeps: CLIRefreshControllerDeps{
			Cfg:                cfg,
			Logger:             logger,
			CLISessionSvc:      cliSessionSvc,
			OperatorSessionSvc: operatorSessionSvc,
			UserSvc:            userSvc,
			Responder:          ls.responder,
		},
		EnrollmentTokenControllerDeps: EnrollmentTokenControllerDeps{
			Cfg:                cfg,
			Logger:             logger,
			EnrollmentTokenSvc: ls.enrollmentTokenSvc,
			Responder:          ls.responder,
		},
		UserControllerDeps: UserControllerDeps{
			Cfg:       cfg,
			Logger:    logger,
			UserSvc:   userSvc,
			Responder: ls.responder,
		},
		SessionControllerDeps: SessionControllerDeps{
			Logger:      logger,
			DocStore:    ls.docStore,
			Responder:   ls.responder,
			CrossOrigin: len(cfg.Gateway.AllowedOrigins) > 0,
		},
		AdminControllerDeps: AdminControllerDeps{
			Cfg:            cfg,
			Logger:         logger,
			DocStore:       ls.docStore,
			SignerStore:    ls.signerStore,
			ConsensusStore: ls.consensusStore,
			UserSvc:        userSvc,
			Responder:      ls.responder,
		},
		OperatorControllerDeps: OperatorControllerDeps{
			Cfg:       cfg,
			Logger:    logger,
			Reg:       reg,
			Auth:      auth,
			Responder: ls.responder,
		},
		DispatchControllerDeps: DispatchControllerDeps{
			DispatchSvc: ls.dispatchSvc,
			Responder:   ls.responder,
			Logger:      logger,
		},
		SSEControllerDeps: SSEControllerDeps{
			Cfg:       cfg,
			Logger:    logger,
			DocStore:  ls.docStore,
			KVStore:   ls.kvStore,
			SSEStore:  ls.db.GetStores().SSEStore,
			Pubsub:    pubsub,
			Auth:      auth,
			Responder: ls.responder,
		},
		HealthControllerDeps: HealthControllerDeps{
			Cfg:               cfg,
			Logger:            logger,
			DocStore:          ls.docStore,
			StateRootSvc:      ls.stateRootSvc,
			Responder:         ls.responder,
			IsReady:           ls.IsReady,
			IsGovernanceReady: ls.IsGovernanceReady,
		},
		GovernanceControllerDeps: GovernanceControllerDeps{
			Cfg:       cfg,
			Logger:    logger,
			Responder: ls.responder,
			Consensus: ls.consensusSvc,
			EnvProc:   envProc,
		},
		MCPControllerDeps: MCPControllerDeps{
			MCPGateway: ls.mcpGateway,
		},
		PubSubControllerDeps: PubSubControllerDeps{
			Handler: pubsub,
		},
		PasskeyControllerDeps: PasskeyControllerDeps{
			Handler: passkey,
		},
		PlatformEnrollmentControllerDeps: PlatformEnrollmentControllerDeps{
			Cfg:       cfg,
			Logger:    logger,
			EnrollSvc: ls.platformEnrollmentSvc,
			UserSvc:   userSvc,
			Responder: ls.responder,
		},
	})
	if err != nil {
		return fmt.Errorf("gateway: failed to create HTTP handler: %w", err)
	}
	ls.handler = handler

	// Register in-process handler so the broker delivers heartbeats to the DB.
	pubsub.SetHeartbeatHandler(func(channel string, data []byte) {
		ls.handleHeartbeatPublish(channel, data)
	})

	// Build a map of ports to identify port assignments.
	// HTTP port uses plain HTTP for bootstrap and MCP routes.
	// HTTPS port uses mTLS for all other surfaces.
	portUsage := make(map[int][]string)
	portUsage[cfg.Gateway.HTTPPort] = append(portUsage[cfg.Gateway.HTTPPort], "HTTP")
	portUsage[cfg.Gateway.HTTPSPort] = append(portUsage[cfg.Gateway.HTTPSPort], "HTTPS")

	// Validate up front so collisions fail during init.
	// Port 0 is reserved for tests (net.Listen picks a random free port per server).
	for port, usage := range portUsage {
		if port == 0 {
			continue
		}
		if len(usage) > 1 {
			return fmt.Errorf("gateway: listen: port %d has multiple surface assignments %v; assign distinct ports", port, usage)
		}
	}

	// TLS verification: client certs are accepted and verified when present, but not required.
	// mTLS enforcement for protected routes happens at the application layer via auth.Middleware(),
	// which uses RouteAuthRegistry to classify routes by auth mode (RouteAuthNone, RouteAuthMTLS,
	// RouteAuthWebSession, RouteAuthDual) and enforces the appropriate authentication.
	// Browser clients (console, WebAuthn flows) reach public routes without a client cert.
	tlsConfig := pki.TLSConfig()
	tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	ls.server = &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.Gateway.HTTPPort),
		Handler:           ls.handler.buildHTTPRouter(),
		ReadHeaderTimeout: cfg.Gateway.ReadHeaderTimeout,
		ReadTimeout:       cfg.Gateway.ReadTimeout,
		WriteTimeout:      cfg.Gateway.WriteTimeout,
		IdleTimeout:       cfg.Gateway.IdleTimeout,
		MaxHeaderBytes:    cfg.Gateway.MaxHeaderBytes,
	}

	// HTTPS server: mTLS for all routes (API, public, enrollment)
	ls.publicServer = &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", cfg.Gateway.HTTPSPort),
		Handler:           ls.handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: cfg.Gateway.ReadHeaderTimeout,
		ReadTimeout:       cfg.Gateway.ReadTimeout,
		WriteTimeout:      cfg.Gateway.WriteTimeout,
		IdleTimeout:       cfg.Gateway.IdleTimeout,
		MaxHeaderBytes:    cfg.Gateway.MaxHeaderBytes,
	}

	return nil
}

// GetDocStore returns the document store service.
func (ls *GatewayModeService) GetDocStore() *DocumentStoreService {
	return ls.docStore
}

// GetPKI returns the PKI authority service.
func (ls *GatewayModeService) GetPKI() *PKIAuthority {
	return ls.pki
}

// GetCLISessionService returns the CLI session service.
func (ls *GatewayModeService) GetCLISessionService() *CLISessionService {
	return ls.cliSessionSvc
}

// GetOperatorSessionService returns the operator session service.
func (ls *GatewayModeService) GetOperatorSessionService() *OperatorSessionService {
	return ls.operatorSessionSvc
}

// GetUserService returns the user service.
func (ls *GatewayModeService) GetUserService() *UserService {
	return ls.userSvc
}

// GetRegistrationService returns the registration service.
func (ls *GatewayModeService) GetRegistrationService() *RegistrationService {
	return ls.reg
}

// GetConsensusStore returns the consensus store service.
func (ls *GatewayModeService) GetConsensusStore() *ConsensusStoreService {
	return ls.consensusStore
}

// GetSignerStore returns the signer store service.
func (ls *GatewayModeService) GetSignerStore() *SignerStoreService {
	return ls.signerStore
}

// GetAuditStore returns the audit store service.
func (ls *GatewayModeService) GetAuditStore() *storage.SQLAuditStore {
	return ls.auditStore
}

// GetStateRootSvc returns the state root service.
func (ls *GatewayModeService) GetStateRootSvc() *StateRootService {
	return ls.stateRootSvc
}

// GetKVStore returns the key-value store service.
func (ls *GatewayModeService) GetKVStore() *KVStoreService {
	return ls.kvStore
}

// GetReplayStore returns the replay store service.
func (ls *GatewayModeService) GetReplayStore() *ReplayStoreService {
	return ls.replayStore
}

// GetSecretManager returns the secret manager initialized during database open.
func (ls *GatewayModeService) GetSecretManager() (*SecretManager, error) {
	return ls.db.GetSecretManager(), nil
}

// GetCommandService returns the in-process operator pubsub command service.
func (ls *GatewayModeService) GetCommandService() *pubsub.OperatorPubSubService {
	return ls.cmdSvc
}

// GetPlatformEnrollmentService returns the platform enrollment service
// that owns the owner-approved workload enrollment lifecycle.
func (ls *GatewayModeService) GetPlatformEnrollmentService() *PlatformEnrollmentService {
	return ls.platformEnrollmentSvc
}

// GetHTTPHandler returns the HTTP handler. The handler is built during
// NewGatewayModeService construction; this returns nil only if construction
// failed before the handler was initialized.
func (ls *GatewayModeService) GetHTTPHandler() *HTTPHandler {
	return ls.handler
}

// GetMCPGateway returns the MCP gateway service.
func (ls *GatewayModeService) GetMCPGateway() *mcp.GatewayService {
	return ls.mcpGateway
}

// GetGatewayWebSocketHandler returns the pub/sub websocket handler.
func (ls *GatewayModeService) GetGatewayWebSocketHandler() *GatewayWebSocketHandler {
	return ls.pubsub
}

// GetDispatchService returns the command dispatch service for sending signed
// commands to operators over the WS pub/sub cmd channel.
func (ls *GatewayModeService) GetDispatchService() *DispatchService {
	return ls.dispatchSvc
}

// GetHTTPPort returns the actual HTTP port the server is listening on.
// After Start(), this reflects the dynamically assigned port when configured as 0.
func (ls *GatewayModeService) GetHTTPPort() int {
	if ls.server == nil {
		return 0
	}
	_, portStr, _ := net.SplitHostPort(ls.server.Addr)
	port, _ := strconv.Atoi(portStr)
	return port
}

// GetHTTPSPort returns the actual HTTPS port the server is listening on.
// After Start(), this reflects the dynamically assigned port when configured as 0.
func (ls *GatewayModeService) GetHTTPSPort() int {
	if ls.publicServer == nil {
		return 0
	}
	_, portStr, _ := net.SplitHostPort(ls.publicServer.Addr)
	port, _ := strconv.Atoi(portStr)
	return port
}

func (ls *GatewayModeService) IsReady() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.ready
}

func (ls *GatewayModeService) IsGovernanceReady() bool {
	// When L2 is audited rather than enforced, governance is ready as soon as
	// the service is running without requiring registered L2 signers.
	if !ls.cfg.Gateway.Posture.RequiresL2() {
		return true
	}
	ready, err := ls.signerStore.HasTrustedSigners()
	if err != nil {
		ls.logger.Error("Failed to check if governance is ready", "state", string(constants.ConnectionStateError), "error", err)
		return false
	}
	return ready
}

// GetGovernanceDeps returns the governance dependencies for transaction verification.
// This enables the in-process OperatorPubSubService to perform fail-closed verification.
// The L3 notary handles both WebAuthn (web sessions) and mTLS (CLI sessions).
func (ls *GatewayModeService) GetGovernanceDeps() *pubsub.GatewayModeDeps {
	cliVerifier := NewCLISessionVerifier(ls.docStore, ls.pki, ls.logger, ls.userSvc, ls.cliSessionSvc)

	l3Notary := governance.NewGatewayL3Notary(cliVerifier, ls.passkey.PasskeyService, ls.logger)
	platformDeps := &pubsub.PlatformEnrollmentDeps{
		DocStore:         ls.docStore,
		PKI:              ls.pki,
		CLISessions:      ls.cliSessionSvc,
		OperatorSessions: ls.operatorSessionSvc,
		Posture:          string(ls.cfg.Gateway.Posture),
	}

	return &pubsub.GatewayModeDeps{
		GovernanceCoreDeps: pubsub.GovernanceCoreDeps{
			ReplayStore:       ls.replayStore,
			StateRootProvider: ls.stateRootSvc,
			TransactionAudit:  ls.docStore,
			L3Notary:          l3Notary,
			SignerStore:       ls.signerStore,
			Doctrine:          ls.doctrine,
		},
		GovernedDocStore:       ls.docStore,
		ConsensusPolicyStore:   ls.consensusStore,
		FieldReader:            ls.docStore,
		Consensus:              ls.consensusSvc,
		PlatformEnrollmentDeps: platformDeps,
		Posture:                ls.cfg.Gateway.Posture,
	}
}

// Start begins serving HTTP/WS requests. Blocks until the context is cancelled
// or the server encounters a fatal error.
func (ls *GatewayModeService) Start(ctx context.Context) error {
	ls.mu.Lock()
	if ls.running {
		ls.mu.Unlock()
		return constants.ErrGatewayAlreadyRunning
	}
	ls.running = true
	ls.mu.Unlock()

	ls.logger.Info("operator Gateway Mode ready",
		"posture", ls.cfg.Gateway.Posture,
		"http_port", ls.cfg.Gateway.HTTPPort,
		"https_port", ls.cfg.Gateway.HTTPSPort,
		"data_dir", ls.cfg.Gateway.DataDir)

	ls.logger.Info("Gateway servers starting", "http_port", ls.cfg.Gateway.HTTPPort, "https_port", ls.cfg.Gateway.HTTPSPort)

	// Start background maintenance for MCP gateway
	go ls.mcpGateway.RunMaintenance(ctx)

	// Start background service certificate renewal loop
	go ls.runServiceCertRenewalLoop(ctx)

	// Start background enrollment token cleanup
	go ls.runEnrollmentTokenCleanup(ctx)

	// Start managed cleanup for platform enrollment (expired lease
	// reconciliation and terminal request retention cleanup).
	ls.platformEnrollmentSvc.StartCleanup(ctx)

	errChan := make(chan error, 5)
	readyChan := make(chan struct{}, 5)

	// Identify unique servers to start
	uniqueServers := make(map[*http.Server]string)
	if ls.server != nil {
		uniqueServers[ls.server] = "HTTP"
	}
	if ls.publicServer != nil {
		if _, ok := uniqueServers[ls.publicServer]; !ok {
			uniqueServers[ls.publicServer] = "HTTPS"
		}
	}

	startServer := func(s *http.Server, name string) {
		// Use a temporary gateway to signal readiness before blocking on Serve
		ln, err := net.Listen(string(constants.NetworkProtocolTCP), s.Addr)
		if err != nil {
			ls.logger.Error("Failed to listen", "server", name, "addr", s.Addr, "state", string(constants.ConnectionStateError), "error", err)
			errChan <- err
			return
		}

		// Update server Addr if it was dynamic
		if s.Addr == "0.0.0.0:0" {
			s.Addr = ln.Addr().String()
		}

		lnToServe := ln
		tlsMode := "plain"
		if s.TLSConfig != nil {
			lnToServe = tls.NewListener(ln, s.TLSConfig)
			// All servers now use mTLS (RequireAndVerifyClientCert)
			if s.TLSConfig.ClientAuth == tls.RequireAndVerifyClientCert {
				tlsMode = "mTLS"
			} else {
				tlsMode = "TLS"
			}
		}

		ls.logger.Info("Gateway server listening",
			"server", name,
			"addr", s.Addr,
			"tls", tlsMode)

		readyChan <- struct{}{}
		errChan <- s.Serve(lnToServe)
	}

	for s, name := range uniqueServers {
		go startServer(s, name)
	}

	// Wait for all unique servers to be ready before marking service as ready
	numServers := len(uniqueServers)
	go func() {
		for i := 0; i < numServers; i++ {
			select {
			case <-readyChan:
			case <-ctx.Done():
				return
			}
		}
		ls.mu.Lock()
		ls.ready = true
		ls.mu.Unlock()
		ls.logger.Info("operator Gateway Mode fully operational",
			"posture", ls.cfg.Gateway.Posture)
	}()

	// Listen for context cancellation and trigger shutdown
	// nolint:gosec // G118: ctx is already cancelled, need fresh context for shutdown timeout
	go func() {
		<-ctx.Done()
		ls.logger.Info("Context cancelled, initiating server shutdown")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if ls.server != nil {
			if err := ls.server.Shutdown(shutdownCtx); err != nil {
				ls.logger.Error("Failed to shutdown server", "error", err)
			}
		}
		if ls.publicServer != nil {
			if err := ls.publicServer.Shutdown(shutdownCtx); err != nil {
				ls.logger.Error("Failed to shutdown public server", "error", err)
			}
		}
	}()

	return <-errChan
}

// Stop gracefully shuts down the HTTP server and closes the database.
// Enforces a strict 30-second timeout - if shutdown hangs, the process will
// force-kill itself to prevent zombie processes.
func (ls *GatewayModeService) Stop(ctx context.Context) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !ls.running {
		// Even if the service was never started, resources like the
		// database, pub/sub broker, and suspended transaction service are
		// opened during build() and must be closed to avoid leaking
		// goroutines and file handles (especially on Windows where open
		// files cannot be deleted from TempDir).
		ls.closeResources()
		return nil
	}

	ls.logger.Info("Shutting down gateway service...")

	ls.ready = false

	// Enforce strict 30-second timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ls.server.Shutdown(shutdownCtx); err != nil {
		if shutdownCtx.Err() == context.DeadlineExceeded {
			ls.logger.Error("HTTP server shutdown timeout - forcing exit to prevent zombie process")
			return constants.ErrGatewayShutdownTimeout
		}
		ls.logger.Error("HTTP server shutdown error", "state", string(constants.ConnectionStateError), "error", err)
	}
	if err := ls.publicServer.Shutdown(shutdownCtx); err != nil {
		if shutdownCtx.Err() == context.DeadlineExceeded {
			ls.logger.Error("HTTPS server shutdown timeout - forcing exit to prevent zombie process")
			return constants.ErrGatewayShutdownTimeout
		}
		ls.logger.Error("HTTPS server shutdown error", "state", string(constants.ConnectionStateError), "error", err)
	}

	ls.closeResources()

	ls.running = false
	ls.logger.Info("Gateway service stopped")
	return nil
}

// closeResources releases all resources allocated during build(): the
// pub/sub broker, suspended transaction service, and the canonical database.
// All close methods are idempotent, so this is safe to call even if some
// resources have already been closed.
func (ls *GatewayModeService) closeResources() {
	if ls.platformEnrollmentSvc != nil {
		ls.platformEnrollmentSvc.StopCleanup()
	}
	if ls.pubsub != nil {
		ls.pubsub.Close()
	}
	if ls.suspendedTxService != nil {
		if err := ls.suspendedTxService.Close(); err != nil {
			ls.logger.Error("Suspended transaction service close error", "state", string(constants.ConnectionStateError), "error", err)
		}
	}
	if ls.db != nil {
		if err := ls.db.Close(); err != nil {
			ls.logger.Error("Database close error", "state", string(constants.ConnectionStateError), "error", err)
		}
	}
}

// runEnrollmentTokenCleanup periodically removes expired enrollment tokens
// to prevent unbounded growth of the enrollment_tokens collection.
// Runs every 5 minutes, matching the token TTL.
func (ls *GatewayModeService) runEnrollmentTokenCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ls.handler == nil || ls.handler.enrollmentTokenController == nil || ls.handler.enrollmentTokenController.enrollmentTokenSvc == nil {
				continue
			}
			if err := ls.handler.enrollmentTokenController.enrollmentTokenSvc.CleanupExpiredTokens(); err != nil {
				ls.logger.Warn("Enrollment token cleanup error", "error", err)
			}
		}
	}
}

// runServiceCertRenewalLoop runs a background goroutine that periodically checks
// and renews the service certificate if it is expiring soon.
func (ls *GatewayModeService) runServiceCertRenewalLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Check immediately on startup, but only if the context is still active
	if ctx.Err() != nil {
		return
	}
	if err := ls.renewServiceCertWithIdentity(ctx); err != nil {
		ls.logger.Error("Failed to renew service certificate on startup", "state", string(constants.ConnectionStateError), "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ls.renewServiceCertWithIdentity(ctx); err != nil {
				ls.logger.Error("Failed to renew service certificate", "state", string(constants.ConnectionStateError), "error", err)
			}
		}
	}
}

// renewServiceCertWithIdentity renews the service certificate with current network identity.
func (ls *GatewayModeService) renewServiceCertWithIdentity(ctx context.Context) error {
	// Detect current network identity
	netDetector := network.NewDetector(ls.logger)
	netIdentity, err := netDetector.DetectAll(ctx)
	if err != nil {
		ls.logger.Warn("Failed to detect network identity for renewal, using cached IPs", "error", err)
		return ls.pki.RenewServiceCert(ls.extraIPs)
	}

	// Use detected identity for renewal
	extraIPs := netIdentity.GetAllIPs()
	extraDNSNames := netIdentity.GetAllDNSNames()
	return ls.pki.RenewServiceCertWithNames(extraIPs, extraDNSNames)
}

// heartbeatUpdate is the typed patch payload for operator document heartbeat updates.
type heartbeatUpdate struct {
	LatestHeartbeatSnapshot json.RawMessage `json:"latest_heartbeat_snapshot"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

// handleHeartbeatPublish processes a heartbeat published to the pub/sub broker,
// updating the operator document's latest_heartbeat_snapshot in the DB.
func (ls *GatewayModeService) handleHeartbeatPublish(channel string, data []byte) {
	var env commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(data, &env); err != nil {
		ls.logger.Warn("heartbeat: failed to decode envelope", "channel", channel, "error", err)
		return
	}
	if env.GetOperatorId() == "" {
		ls.logger.Warn("heartbeat: envelope missing operator_id", "channel", channel)
		return
	}

	var snapshot json.RawMessage
	if env.IntentData != nil {
		snapshotBytes, err := protojson.Marshal(env.IntentData)
		if err != nil {
			ls.logger.Warn("heartbeat: failed to marshal intent data", "operator_id", env.GetOperatorId(), "error", err)
			return
		}
		snapshot = snapshotBytes
	} else {
		snapshot = json.RawMessage(data)
	}

	update, err := json.Marshal(heartbeatUpdate{
		LatestHeartbeatSnapshot: snapshot,
		UpdatedAt:               time.Now().UTC(),
	})
	if err != nil {
		ls.logger.Warn("heartbeat: failed to build update", "operator_id", env.GetOperatorId(), "error", err)
		return
	}

	if _, err := ls.docStore.DocUpdate(string(constants.CollectionOperators), env.GetOperatorId(), update); err != nil {
		ls.logger.Warn("heartbeat: failed to update operator document", "operator_id", env.GetOperatorId(), "error", err)
		return
	}

	ls.logger.Debug("heartbeat: operator snapshot updated", "operator_id", env.GetOperatorId(), "channel", channel)
}
