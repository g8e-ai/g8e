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

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/paths"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/internal/services/pubsub"
	"github.com/g8e-ai/g8e/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/services/tribunal"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// GatewayModeService is the top-level orchestrator for gateway mode (operator).
// It acts as the platform's central persistence and messaging backbone.
// In this mode, the Operator does NOT execute commands or initiate outbound
// connections. It strictly serves inbound requests from platform components.
type GatewayModeService struct {
	cfg     *config.Config
	logger  *slog.Logger
	fileSvc fs.RuntimeFileService

	db                 *CanonicalDBService
	pubsub             *GatewayWebSocketHandler
	auth               *AuthService
	pki                *PKIAuthority
	sm                 *SecretManager
	reg                *RegistrationService
	passkey            *PasskeyHandler
	userSvc            *UserService
	cliSessionSvc      *CLISessionService
	operatorSessionSvc *OperatorSessionService
	webSessionSvc      *WebSessionService
	suspendedTxService storage.SuspendedTransactionStore
	suspendedTxCloser  *storage.SuspendedTransactionService
	mcpGateway         *mcp.GatewayService
	tribunal           *tribunal.TribunalService
	responder          *response.Writer
	server             *http.Server
	publicServer       *http.Server

	handler *HTTPHandler

	extraIPs []net.IP

	mu      sync.Mutex
	running bool
	ready   bool
}

// gatewayServiceBuilder constructs a GatewayModeService from configuration,
// with optional pre-built components for test environments.
type gatewayServiceBuilder struct {
	cfg     *config.Config
	logger  *slog.Logger
	fileSvc fs.RuntimeFileService

	// Pre-built components (test only). When nil, build() constructs them.
	preBuiltDB     *CanonicalDBService
	preBuiltPubsub *GatewayWebSocketHandler

	// testMode skips production-only initialization (persona seeding,
	// certificate identity resolution, JWKS, strict error checking on
	// SecretManager and passkey service).
	testMode bool
}

// newGatewayServiceBuilder creates a builder for production use.
func newGatewayServiceBuilder(cfg *config.Config, fileSvc fs.RuntimeFileService, logger *slog.Logger) *gatewayServiceBuilder {
	return &gatewayServiceBuilder{cfg: cfg, logger: logger, fileSvc: fileSvc}
}

// withPreBuiltDB sets a pre-built CanonicalDBService (test only).
func (b *gatewayServiceBuilder) withPreBuiltDB(db *CanonicalDBService) *gatewayServiceBuilder {
	b.preBuiltDB = db
	return b
}

// withPreBuiltPubsub sets a pre-built GatewayWebSocketHandler (test only).
func (b *gatewayServiceBuilder) withPreBuiltPubsub(pubsub *GatewayWebSocketHandler) *gatewayServiceBuilder {
	b.preBuiltPubsub = pubsub
	return b
}

// withTestMode enables test mode, which skips production-only initialization.
func (b *gatewayServiceBuilder) withTestMode() *gatewayServiceBuilder {
	b.testMode = true
	return b
}

// build assembles the GatewayModeService from the builder's configuration.
func (b *gatewayServiceBuilder) build() (*GatewayModeService, error) {
	cfg := b.cfg
	logger := b.logger

	// --- DB and pubsub ---
	db := b.preBuiltDB
	if db == nil {
		var err error
		db, err = OpenCanonicalDBService(cfg.Gateway.DataDir, cfg.Gateway.VaultDir, logger, false, cfg.Gateway.VaultKeyPath, nil, b.fileSvc)
		if err != nil {
			return nil, fmt.Errorf("gateway: failed to initialize database: %w", err)
		}
	}

	pubsub := b.preBuiltPubsub
	if pubsub == nil {
		pubsub = NewGatewayWebSocketHandler(logger)
	}

	// --- Secret manager ---
	sm, err := NewSecretManager(db.db, b.fileSvc, logger)
	if err != nil && !b.testMode {
		return nil, fmt.Errorf("gateway: initialize secret manager: %w", err)
	}

	// --- PKI ---
	pki := newPKIAuthority(b.fileSvc, db, sm, logger)

	// --- Core services ---
	userSvc := NewUserService(db, logger)
	res := response.NewWriter(logger)

	var jwksProvider *JWKSProvider
	if !b.testMode && cfg.Gateway.JWKSURL != "" {
		jwksProvider = NewJWKSProvider(cfg.Gateway.JWKSURL)
	}

	personaSvc := NewPersonaService(db, logger)
	if !b.testMode {
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
	}

	jwtRoleClaim := cfg.Gateway.JWTRoleClaim
	jwtIssuer := cfg.Gateway.JWTIssuer
	jwtAudience := cfg.Gateway.JWTAudience
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, res, cfg.Gateway.SecretsDir, jwksProvider, jwtRoleClaim, jwtIssuer, jwtAudience)
	userSvc.SetAuthService(auth)

	cliSessionSvc := NewCLISessionService(db, logger)
	operatorSessionSvc := NewOperatorSessionService(db, logger)
	webSessionSvc := NewWebSessionService(db, logger)

	// --- Certificate identity and PKI initialization ---
	var extraIPs []net.IP
	if !b.testMode {
		var extraDNSNames []string
		extraIPs, extraDNSNames, err = resolveGatewayCertificateIdentity(cfg.Gateway.CertMode, cfg.Gateway.NetworkIdentityFile, network.NewDetector(logger), logger)
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
	}

	reg := NewRegistrationService(db, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, &cfg.Gateway)

	// --- Passkey ---
	passkeyCfg := &PasskeyConfig{
		RpID:      cfg.Gateway.PasskeyRpID,
		RpName:    cfg.Gateway.PasskeyRpName,
		RpOrigins: cfg.Gateway.PasskeyRpOrigins,
		HTTPPort:  cfg.Gateway.HTTPPort,
		HTTPSPort: cfg.Gateway.HTTPSPort,
	}
	passkey, err := NewPasskeyService(db, logger, passkeyCfg)
	if err != nil && !b.testMode {
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
	scrubbingService := scrubbing.NewScrubbingService(scrubbingConfig, logger, nil)

	publicBaseURL := cfg.Gateway.PublicBaseURL
	if publicBaseURL == "" {
		publicBaseURL = network.LocalhostHTTPSURL(cfg.Gateway.HTTPSPort)
	}

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           logger,
		Responder:        res,
		SuspendedStore:   suspendedTxService,
		ScrubbingService: scrubbingService,
		MaxPayloadBytes:  cfg.Gateway.MaxPayloadBytes,
		Posture:          string(cfg.Gateway.Posture),
		A2ADownstreamURL: cfg.Gateway.A2ADownstreamURL,
		PublicBaseURL:    publicBaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize MCP gateway: %w", err)
	}

	passkeyHandler := NewPasskeyHandler(PasskeyHandlerDeps{
		Service:        passkey,
		WebSessionSvc:  webSessionSvc,
		Responder:      res,
		MaxPayload:     cfg.Gateway.MaxPayloadBytes,
		MCPSvc:         mcpGateway,
		SuspendedStore: suspendedTxService,
		SSEStore:       db.SSEStore,
		Pubsub:         pubsub,
		CrossOrigin:    len(cfg.Gateway.AllowedOrigins) > 0,
	})

	ls := &GatewayModeService{
		cfg:                cfg,
		logger:             logger,
		fileSvc:            b.fileSvc,
		db:                 db,
		pubsub:             pubsub,
		auth:               auth,
		pki:                pki,
		reg:                reg,
		sm:                 sm,
		passkey:            passkeyHandler,
		userSvc:            userSvc,
		cliSessionSvc:      cliSessionSvc,
		operatorSessionSvc: operatorSessionSvc,
		webSessionSvc:      webSessionSvc,
		suspendedTxService: suspendedTxService,
		suspendedTxCloser:  suspendedTxService,
		extraIPs:           extraIPs,
		mcpGateway:         mcpGateway,
		responder:          res,
	}

	if err := ls.initHandlersAndServers(); err != nil {
		return nil, fmt.Errorf("gateway: failed to initialize handlers and servers: %w", err)
	}

	return ls, nil
}

// NewGatewayModeService creates a new gateway mode service.
func NewGatewayModeService(cfg *config.Config, fileSvc fs.RuntimeFileService, logger *slog.Logger) (*GatewayModeService, error) {
	return newGatewayServiceBuilder(cfg, fileSvc, logger).build()
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

// newGatewayModeServiceForTest assembles a GatewayModeService from pre-built components.
// Used in tests where the DB and pub/sub broker are constructed independently.
func newGatewayModeServiceForTest(cfg *config.Config, fileSvc fs.RuntimeFileService, logger *slog.Logger, db *CanonicalDBService, pubsub *GatewayWebSocketHandler) (*GatewayModeService, error) {
	return newGatewayServiceBuilder(cfg, fileSvc, logger).
		withPreBuiltDB(db).
		withPreBuiltPubsub(pubsub).
		withTestMode().
		build()
}

func (ls *GatewayModeService) initHandlersAndServers() error {
	cfg := ls.cfg
	logger := ls.logger
	db := ls.db
	pubsub := ls.pubsub
	auth := ls.auth
	pki := ls.pki
	cliSessionSvc := ls.cliSessionSvc
	operatorSessionSvc := ls.operatorSessionSvc
	webSessionSvc := ls.webSessionSvc
	reg := ls.reg
	passkey := ls.passkey
	userSvc := ls.userSvc

	// Initialize AppEnrollmentService for external app enrollment
	appEnrollment := NewAppEnrollmentService(db, pki, logger)

	handler, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:                cfg,
		Logger:             logger,
		DB:                 db,
		Pubsub:             pubsub,
		Auth:               auth,
		PKI:                pki,
		CLISessionSvc:      cliSessionSvc,
		OperatorSessionSvc: operatorSessionSvc,
		WebSessionSvc:      webSessionSvc,
		Reg:                reg,
		Passkey:            passkey,
		UserSvc:            userSvc,
		Responder:          ls.responder,
		MCPGateway:         ls.mcpGateway,
		AppEnrollment:      appEnrollment,
		Tribunal:           ls.tribunal,
		IsReady:            ls.IsReady,
		IsGovernanceReady:  ls.IsGovernanceReady,
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

// GetDB returns the underlying database service.
func (ls *GatewayModeService) GetDB() *CanonicalDBService {
	return ls.db
}

// GetSecretManager returns the cached secret manager. If the manager was not
// initialized during build() (e.g., test mode where keychain init is skipped),
// it falls back to creating a new instance.
func (ls *GatewayModeService) GetSecretManager() (*SecretManager, error) {
	if ls.sm != nil {
		return ls.sm, nil
	}
	return NewSecretManager(ls.db.db, ls.fileSvc, ls.logger)
}

// GetPKIAuthority returns the underlying PKI authority.
func (ls *GatewayModeService) GetPKIAuthority() *PKIAuthority {
	return ls.pki
}

// GetHTTPHandler returns the HTTP handler.
func (ls *GatewayModeService) GetHTTPHandler() *HTTPHandler {
	return ls.handler
}

// SetTribunal sets the Tribunal service for L2 consensus deliberation.
// This is called by the boot sequence after the TribunalService is constructed.
// The Tribunal is registered on the mTLS mux and the HTTP deliberator is wired
// into the MCP gateway for consensus and notary postures.
func (ls *GatewayModeService) SetTribunal(ts *tribunal.TribunalService) {
	ls.tribunal = ts
	if ls.handler != nil {
		ls.handler.SetTribunal(ts)
	}
}

// GetHTTPSPort returns the actual bound HTTPS port (useful when AllowTestPortZero is true).
func (ls *GatewayModeService) GetHTTPSPort() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if !ls.running || ls.publicServer == nil {
		return 0
	}
	_, port, err := net.SplitHostPort(ls.publicServer.Addr)
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return p
}

// GetHTTPPort returns the actual bound HTTP port (useful when AllowTestPortZero is true).
func (ls *GatewayModeService) GetHTTPPort() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if !ls.running || ls.server == nil {
		return 0
	}
	_, port, err := net.SplitHostPort(ls.server.Addr)
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return p
}

func (ls *GatewayModeService) IsRunning() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.running
}

func (ls *GatewayModeService) IsReady() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.ready
}

func (ls *GatewayModeService) IsGovernanceReady() bool {
	// In doctrine posture, L2 is audited not enforced — governance is ready
	// as soon as the service is running, without requiring registered L2 signers.
	if ls.cfg.Gateway.Posture == config.PostureDoctrine || ls.cfg.Gateway.Posture == "" {
		return true
	}
	ready, err := ls.db.SignerStore.HasTrustedSigners()
	if err != nil {
		ls.logger.Error("Failed to check if governance is ready", "state", string(constants.ConnectionStateError), "error", err)
		return false
	}
	return ready
}

// GetGovernanceDeps returns the governance dependencies for transaction verification.
// This enables the in-process OperatorPubSubService to perform fail-closed verification.
// The L3 notary handles both WebAuthn (web sessions) and mTLS (CLI sessions).
func (ls *GatewayModeService) GetGovernanceDeps() *pubsub.GovernanceDeps {
	// Create unified L3 notary that handles both CLI (mTLS) and passkey (WebAuthn) proofs
	cliVerifier := NewCLISessionVerifier(ls.db, ls.pki, ls.logger, ls.userSvc, ls.cliSessionSvc)
	l3Notary := governance.NewGatewayL3Notary(ls.suspendedTxService, cliVerifier, ls.passkey.PasskeyService, ls.logger)

	return &pubsub.GovernanceDeps{
		ReplayStore:       ls.db.ReplayStore,
		StateRootProvider: ls.db.StateRootSvc,
		TransactionAudit:  ls.db.DocStore,
		L3Notary:          l3Notary,
		SignerStore:       ls.db.SignerStore,
		AppPolicyStore:    ls.db.AppPolicyStore,
		TribunalStore:     ls.db.TribunalStore,
		FieldReader:       ls.db.DocStore,
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
		// suspended transaction service are opened during build() and
		// must be closed to avoid leaking file handles (especially on
		// Windows where open files cannot be deleted from TempDir).
		if ls.suspendedTxCloser != nil {
			if err := ls.suspendedTxCloser.Close(); err != nil {
				ls.logger.Error("Suspended transaction service close error", "state", string(constants.ConnectionStateError), "error", err)
			}
		}
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

	// Close pub/sub broker (disconnects all WebSocket clients)
	ls.pubsub.Close()

	// Close suspended transaction service database
	if ls.suspendedTxCloser != nil {
		if err := ls.suspendedTxCloser.Close(); err != nil {
			ls.logger.Error("Suspended transaction service close error", "state", string(constants.ConnectionStateError), "error", err)
		}
	}

	// Close database
	if err := ls.db.Close(); err != nil {
		ls.logger.Error("Database close error", "state", string(constants.ConnectionStateError), "error", err)
	}

	ls.running = false
	ls.logger.Info("Gateway service stopped")
	return nil
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
			if ls.handler == nil || ls.handler.authController == nil || ls.handler.authController.enrollmentTokenSvc == nil {
				continue
			}
			if err := ls.handler.authController.enrollmentTokenSvc.CleanupExpiredTokens(); err != nil {
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

	if _, err := ls.db.DocStore.DocUpdate(string(constants.CollectionOperators), env.GetOperatorId(), update); err != nil {
		ls.logger.Warn("heartbeat: failed to update operator document", "operator_id", env.GetOperatorId(), "error", err)
		return
	}

	ls.logger.Debug("heartbeat: operator snapshot updated", "operator_id", env.GetOperatorId(), "channel", channel)
}
