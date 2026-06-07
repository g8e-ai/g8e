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
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/network"
)

// GatewayModeService is the top-level orchestrator for gateway mode (operator).
// It acts as the platform's central persistence and messaging backbone.
// In this mode, the Operator does NOT execute commands or initiate outbound
// connections. It strictly serves inbound requests from platform components.
type GatewayModeService struct {
	cfg    *config.Config
	logger *slog.Logger

	db           *CanonicalDBService
	pubsub       *PubSubBroker
	auth         *AuthService
	pki          *PKIAuthority
	reg          *RegistrationService
	passkey      *PasskeyService
	userSvc      *UserService
	sessionSvc   *SessionsService
	mcpGateway   *mcp.GatewayService
	responder    *response.Writer
	server       *http.Server
	publicServer *http.Server

	handler *HTTPHandler

	extraIPs []net.IP

	mu      sync.Mutex
	running bool
	ready   bool
}

// NewGatewayModeService creates a new gateway mode service.
func NewGatewayModeService(cfg *config.Config, logger *slog.Logger) (*GatewayModeService, error) {
	db, err := OpenCanonicalDBService(cfg.Gateway.DataDir, cfg.Gateway.SecretsDir, cfg.Gateway.VaultDir, logger, false, cfg.Gateway.VaultKeyPath, cfg.Gateway.VaultRequireUnlock)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	pubsub := NewPubSubBroker(logger)
	sm, err := NewSecretManager(db.db, cfg.Gateway.SecretsDir, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize secret manager: %w", err)
	}
	pki := newPKIAuthority(cfg.Gateway.DataDir, cfg.Gateway.PKIDir, db, sm, logger)
	userSvc := NewUserService(db, logger)
	res := response.NewWriter(logger)

	var jwksProvider *JWKSProvider
	if cfg.Gateway.JWKSURL != "" {
		jwksProvider = NewJWKSProvider(cfg.Gateway.JWKSURL)
	}

	personaSvc := NewPersonaService(db, logger)
	if err := personaSvc.GetOrCreateDefaultPersonas(); err != nil {
		return nil, fmt.Errorf("failed to initialize default personas: %w", err)
	}

	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, res, cfg.Gateway.SecretsDir, jwksProvider, cfg.Gateway.JWTRoleClaim, cfg.Gateway.JWTIssuer, cfg.Gateway.JWTAudience)
	sessionSvc := NewSessionService(db, logger)

	// Detect network identity for certificate generation based on mode
	extraIPs, extraDNSNames, err := resolveGatewayCertificateIdentity(cfg.Gateway.CertMode, cfg.Gateway.NetworkIdentityFile, network.NewDetector(logger), logger)
	if err != nil {
		return nil, err
	}
	if len(extraDNSNames) > 0 {
		if err := pki.EnsurePKIWithNames(extraIPs, extraDNSNames); err != nil {
			return nil, fmt.Errorf("failed to ensure PKI hierarchy: %w", err)
		}
	} else {
		if err := pki.EnsurePKI(extraIPs); err != nil {
			return nil, fmt.Errorf("failed to ensure PKI hierarchy: %w", err)
		}
	}

	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, &cfg.Gateway)

	// Initialize passkey service for L3 brokerage
	passkeyCfg := &PasskeyConfig{
		RpID:   cfg.Gateway.PasskeyRpID,
		RpName: cfg.Gateway.PasskeyRpName,
	}
	passkey, err := NewPasskeyService(db, logger, passkeyCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkey service: %w", err)
	}

	ls := &GatewayModeService{
		cfg:        cfg,
		logger:     logger,
		db:         db,
		pubsub:     pubsub,
		auth:       auth,
		pki:        pki,
		reg:        reg,
		passkey:    passkey,
		userSvc:    userSvc,
		sessionSvc: sessionSvc,
		extraIPs:   extraIPs,
		mcpGateway: mcp.NewGatewayService(mcp.Dependencies{
			Logger:          logger,
			Responder:       res,
			SuspendedStore:  db,
			MaxPayloadBytes: cfg.Gateway.MaxPayloadBytes,
		}),
		responder: res,
	}

	if err := ls.initHandlersAndServers(); err != nil {
		return nil, fmt.Errorf("failed to initialize handlers and servers: %w", err)
	}

	return ls, nil
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
	netIdentity, err := detector.DetectAll(context.Background())
	if err != nil {
		logger.Warn("Failed to detect localhost identities, using defaults", "error", err)
		return []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}, []string{"localhost"}, nil
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
			return nil, nil, fmt.Errorf("failed to read network identity file: %w", err)
		}

		var netIdentity network.NetworkIdentity
		if err := json.Unmarshal(identityData, &netIdentity); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal network identity: %w", err)
		}

		extraIPs := netIdentity.GetAllIPs()
		extraDNSNames := netIdentity.GetAllDNSNames()
		logger.Info("Network identity loaded from file for certificate", "dns_names", len(extraDNSNames), "ips", len(extraIPs))
		return extraIPs, extraDNSNames, nil
	}

	netIdentity, err := detector.DetectAll(context.Background())
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

// newGatewayModeServiceFromComponents assembles a GatewayModeService from pre-built components.
// Used in tests where the DB and pub/sub broker are constructed independently.
func newGatewayModeServiceFromComponents(cfg *config.Config, logger *slog.Logger, db *CanonicalDBService, pubsub *PubSubBroker) (*GatewayModeService, error) {
	sm, _ := NewSecretManager(db.db, cfg.Gateway.SecretsDir, logger)
	pki := newPKIAuthority(cfg.Gateway.DataDir, cfg.Gateway.PKIDir, db, sm, logger)
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := response.NewWriter(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, res, cfg.Gateway.SecretsDir, nil, "", "", "")
	sessionSvc := NewSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, &cfg.Gateway)

	// Initialize passkey service for L3 brokerage (test configuration)
	passkeyCfg := &PasskeyConfig{
		RpID:   cfg.Gateway.PasskeyRpID,
		RpName: cfg.Gateway.PasskeyRpName,
	}
	// Passkey service initialization is optional; ignore errors for test configuration
	passkey, _ := NewPasskeyService(db, logger, passkeyCfg) //nolint:errcheck

	ls := &GatewayModeService{
		cfg:        cfg,
		logger:     logger,
		db:         db,
		pubsub:     pubsub,
		auth:       auth,
		pki:        pki,
		reg:        reg,
		passkey:    passkey,
		userSvc:    userSvc,
		sessionSvc: sessionSvc,
		extraIPs:   nil, // Test configuration does not use extra IPs
		mcpGateway: mcp.NewGatewayService(mcp.Dependencies{
			Logger:          logger,
			Responder:       res,
			SuspendedStore:  db,
			MaxPayloadBytes: cfg.Gateway.MaxPayloadBytes,
		}),
		responder: res,
	}

	if err := ls.initHandlersAndServers(); err != nil {
		return nil, fmt.Errorf("failed to initialize handlers and servers: %w", err)
	}

	return ls, nil
}

func (ls *GatewayModeService) initHandlersAndServers() error {
	cfg := ls.cfg
	logger := ls.logger
	db := ls.db
	pubsub := ls.pubsub
	auth := ls.auth
	pki := ls.pki
	sessionSvc := ls.sessionSvc
	reg := ls.reg
	passkey := ls.passkey
	userSvc := ls.userSvc

	// Initialize AppEnrollmentService for external app enrollment
	appEnrollment := NewAppEnrollmentService(db, pki, logger)

	ls.mcpGateway.SetA2ADependencies(cfg.Gateway.A2ADownstreamURL)
	publicBaseURL := fmt.Sprintf("https://localhost:%d", cfg.Gateway.HTTPSPort)
	ls.mcpGateway.SetPublicBaseURL(publicBaseURL)
	ls.handler = newHTTPHandler(HTTPHandlerDependencies{
		Cfg:               cfg,
		Logger:            logger,
		DB:                db,
		Pubsub:            pubsub,
		Auth:              auth,
		PKI:               pki,
		SessionSvc:        sessionSvc,
		Reg:               reg,
		Passkey:           passkey,
		UserSvc:           userSvc,
		Responder:         ls.responder,
		MCPGateway:        ls.mcpGateway,
		AppEnrollment:     appEnrollment,
		IsReady:           ls.IsReady,
		IsGovernanceReady: ls.IsGovernanceReady,
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
			return fmt.Errorf("listen: port %d has multiple surface assignments %v; assign distinct ports", port, usage)
		}
	}

	tlsConfig := pki.TLSConfig() // strict mTLS (RequireAndVerifyClientCert)

	// Fail-closed assertion: mTLS surface MUST use RequireAndVerifyClientCert
	// If this is downgraded to VerifyClientCertIfGiven, the execution boundary
	// becomes an L7 check instead of a TLS-layer gate, which is a security regression.
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		panic(fmt.Sprintf("gateway: mTLS port %d configured with ClientAuth=%v; must be RequireAndVerifyClientCert for fail-closed execution boundary", cfg.Gateway.HTTPPort, tlsConfig.ClientAuth))
	}

	// Each surface gets its own dedicated gateway with a TLS config that
	// matches the surface's authentication contract. The mTLS surface MUST
	// use RequireAndVerifyClientCert; mixing it with any non-mTLS surface
	// on the same port would force VerifyClientCertIfGiven and downgrade
	// the execution boundary to an L7 check.
	ls.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Gateway.HTTPPort),
		Handler:           ls.handler.buildHTTPRouter(),
		ReadHeaderTimeout: cfg.Gateway.ReadHeaderTimeout,
		ReadTimeout:       cfg.Gateway.ReadTimeout,
		WriteTimeout:      cfg.Gateway.WriteTimeout,
		IdleTimeout:       cfg.Gateway.IdleTimeout,
		MaxHeaderBytes:    cfg.Gateway.MaxHeaderBytes,
	}

	// HTTPS server: mTLS for all routes (API, public, enrollment)
	ls.publicServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Gateway.HTTPSPort),
		Handler:           ls.handler.buildPublicRouter(),
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

// GetSecretManager returns the secret manager.
func (ls *GatewayModeService) GetSecretManager() (*SecretManager, error) {
	return NewSecretManager(ls.db.db, ls.cfg.Gateway.SecretsDir, ls.logger)
}

// GetPKIAuthority returns the underlying PKI authority.
func (ls *GatewayModeService) GetPKIAuthority() *PKIAuthority {
	return ls.pki
}

// GetHTTPHandler returns the HTTP handler.
func (ls *GatewayModeService) GetHTTPHandler() *HTTPHandler {
	return ls.handler
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
	ready, err := ls.db.HasTrustedSigners()
	if err != nil {
		ls.logger.Error("Failed to check if governance is ready", string(constants.ConnectionStateError), err)
		return false
	}
	return ready
}

// GovernanceDeps holds the governance dependencies required for transaction verification.
// These interfaces are implemented by CanonicalDBService (ReplayStore, StateRootProvider,
// TransactionAuditStore) and CompositeL3Verifier (L3Notary).
type GovernanceDeps struct {
	ReplayStore       governance.ReplayStore
	StateRootProvider governance.StateRootProvider
	TransactionAudit  governance.TransactionAuditStore
	L3Notary          governance.L3Notary
	SignerStore       governance.SignerStore
	AppPolicyStore    governance.AppPolicyStore
}

// GetGovernanceDeps returns the governance dependencies for transaction verification.
// This enables the in-process PubSubCommandService to perform fail-closed verification.
// The L3 notary is a composite that handles both WebAuthn (web sessions) and mTLS (CLI sessions).
func (ls *GatewayModeService) GetGovernanceDeps() *GovernanceDeps {
	// Create composite L3 notary that handles both web and CLI sessions
	cliL3 := NewCLIL3Notary(ls.db, ls.pki, ls.logger, ls.userSvc, ls.sessionSvc)
	compositeL3 := NewCompositeL3Verifier(ls.passkey, cliL3, ls.logger)

	return &GovernanceDeps{
		ReplayStore:       ls.db,
		StateRootProvider: ls.db,
		TransactionAudit:  ls.db,
		L3Notary:          compositeL3,
		SignerStore:       ls.db,
		AppPolicyStore:    ls.db,
	}
}

// Start begins serving HTTP/WS requests. Blocks until the context is cancelled
// or the server encounters a fatal error.
func (ls *GatewayModeService) Start(ctx context.Context) error {
	ls.mu.Lock()
	if ls.running {
		ls.mu.Unlock()
		return fmt.Errorf("gateway service already running")
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
			ls.logger.Error("Failed to listen", "server", name, "addr", s.Addr, string(constants.ConnectionStateError), err)
			errChan <- err
			return
		}

		// Update server Addr if it was dynamic
		if s.Addr == ":0" {
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

	return <-errChan
}

// Stop gracefully shuts down the HTTP server and closes the database.
// Enforces a strict 30-second timeout - if shutdown hangs, the process will
// force-kill itself to prevent zombie processes.
func (ls *GatewayModeService) Stop(ctx context.Context) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !ls.running {
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
			return fmt.Errorf("shutdown timeout exceeded")
		}
		ls.logger.Error("HTTP server shutdown error", string(constants.ConnectionStateError), err)
	}
	if err := ls.publicServer.Shutdown(shutdownCtx); err != nil {
		if shutdownCtx.Err() == context.DeadlineExceeded {
			ls.logger.Error("HTTPS server shutdown timeout - forcing exit to prevent zombie process")
			return fmt.Errorf("shutdown timeout exceeded")
		}
		ls.logger.Error("HTTPS server shutdown error", string(constants.ConnectionStateError), err)
	}

	// Close pub/sub broker (disconnects all WebSocket clients)
	ls.pubsub.Close()

	// Close database
	if err := ls.db.Close(); err != nil {
		ls.logger.Error("Database close error", string(constants.ConnectionStateError), err)
	}

	ls.running = false
	ls.logger.Info("Gateway service stopped")
	return nil
}

// runServiceCertRenewalLoop runs a background goroutine that periodically checks
// and renews the service certificate if it is expiring soon.
func (ls *GatewayModeService) runServiceCertRenewalLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// Check immediately on startup
	if err := ls.renewServiceCertWithIdentity(ctx); err != nil {
		ls.logger.Error("Failed to renew service certificate on startup", string(constants.ConnectionStateError), err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := ls.renewServiceCertWithIdentity(ctx); err != nil {
				ls.logger.Error("Failed to renew service certificate", string(constants.ConnectionStateError), err)
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
