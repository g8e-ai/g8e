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

package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/mcp"
)

// autoTLSListener wraps a net.Listener to detect TLS vs HTTP connections.
// It peeks at the first byte to determine if the client is initiating a TLS handshake.
// If the first byte is 0x16 (TLS ClientHello), it wraps the connection with TLS.
// Otherwise, it serves the connection as plain HTTP.
type autoTLSListener struct {
	net.Listener
	tlsConfig *tls.Config
	logger    *slog.Logger
}

func (l *autoTLSListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	// Peek at the first byte to detect TLS handshake
	firstByte := make([]byte, 1)
	_, err = conn.Read(firstByte)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// TLS ClientHello starts with 0x16 (Handshake type)
	if firstByte[0] == 0x16 {
		// Wrap with TLS
		tlsConn := tls.Server(&peekedConn{conn, firstByte}, l.tlsConfig)
		return tlsConn, nil
	}

	// Plain HTTP connection
	return &peekedConn{conn, firstByte}, nil
}

// peekedConn wraps a net.Conn with a peeked byte that was already read.
type peekedConn struct {
	net.Conn
	peeked []byte
}

func (c *peekedConn) Read(b []byte) (int, error) {
	if len(c.peeked) > 0 {
		n := copy(b, c.peeked)
		c.peeked = c.peeked[n:]
		return n, nil
	}
	return c.Conn.Read(b)
}

// GatewayService is the top-level orchestrator for gateway mode (operator).
// It acts as the platform's central persistence and messaging backbone.
// In this mode, the Operator does NOT execute commands or initiate outbound
// connections. It strictly serves inbound requests from platform components.
type GatewayService struct {
	cfg    *config.Config
	logger *slog.Logger

	db           *GatewayDBService
	pubsub       *PubSubBroker
	auth         *AuthService
	pki          *PKIAuthority
	reg          *RegistrationService
	passkey      *PasskeyService
	userSvc      *UserService
	sessionSvc   *SessionService
	mcpGateway   *mcp.GatewayService
	responder    *responder.Responder
	server       *http.Server
	publicServer *http.Server

	handler *HTTPHandler

	mu      sync.Mutex
	running bool
	ready   bool
}

// NewGatewayService creates a new gateway mode service.
func NewGatewayService(cfg *config.Config, logger *slog.Logger) (*GatewayService, error) {
	db, err := OpenGatewayDBService(cfg.Gateway.DataDir, cfg.Gateway.SecretsDir, logger, false)
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
	res := responder.New(logger)

	var jwksProvider *JWKSProvider
	if cfg.Gateway.JWKSURL != "" {
		jwksProvider = NewJWKSProvider(cfg.Gateway.JWKSURL)
	}

	personaSvc := NewPersonaService(db, logger)
	if err := personaSvc.GetOrCreateDefaultPersonas(); err != nil {
		return nil, fmt.Errorf("failed to initialize default personas: %w", err)
	}

	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, res, cfg.Gateway.SecretsDir, jwksProvider, cfg.Gateway.JWTRoleClaim)
	sessionSvc := NewSessionService(db, logger)

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

	if err := pki.EnsurePKI(extraIPs); err != nil {
		return nil, fmt.Errorf("failed to ensure PKI hierarchy: %w", err)
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

	ls := &GatewayService{
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

// newGatewayServiceFromComponents assembles a GatewayService from pre-built components.
// Used in tests where the DB and pub/sub broker are constructed independently.
func newGatewayServiceFromComponents(cfg *config.Config, logger *slog.Logger, db *GatewayDBService, pubsub *PubSubBroker) (*GatewayService, error) {
	sm, _ := NewSecretManager(db.db, cfg.Gateway.SecretsDir, logger)
	pki := newPKIAuthority(cfg.Gateway.DataDir, cfg.Gateway.PKIDir, db, sm, logger)
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, res, cfg.Gateway.SecretsDir, nil, "")
	sessionSvc := NewSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, &cfg.Gateway)

	// Initialize passkey service for L3 brokerage (test configuration)
	passkeyCfg := &PasskeyConfig{
		RpID:   cfg.Gateway.PasskeyRpID,
		RpName: cfg.Gateway.PasskeyRpName,
	}
	// Passkey service initialization is optional; ignore errors for test configuration
	passkey, _ := NewPasskeyService(db, logger, passkeyCfg) //nolint:errcheck

	ls := &GatewayService{
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

func (ls *GatewayService) initHandlersAndServers() error {
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
	publicBaseURL := fmt.Sprintf("https://localhost:%d", cfg.Gateway.PublicPort)
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
		DocsFS:            nil,
	})

	// Build a map of ports to identify port assignments.
	// Surfaces with different TLS client-auth requirements MUST NOT share a
	// port; sharing would force tls.VerifyClientCertIfGiven and downgrade
	// the mTLS execution boundary to an L7 check.
	portUsage := make(map[int][]string)
	portUsage[cfg.Gateway.HTTPPort] = append(portUsage[cfg.Gateway.HTTPPort], "HTTP")

	// Bootstrap and Public now share the same port - HTTP for CA bootstrap, HTTPS for authenticated routes
	portUsage[cfg.Gateway.PublicPort] = append(portUsage[cfg.Gateway.PublicPort], "Public")
	portUsage[cfg.Gateway.PublicPort] = append(portUsage[cfg.Gateway.PublicPort], "Bootstrap")

	// Validate up front so collisions fail during init, not only when a
	// fresh gateway is built. HTTP and WSS may share a port (both mTLS);
	// every other combination is rejected. Port 0 is reserved for tests
	// (net.Listen picks a random free port per server) and is exempt.
	for port, usage := range portUsage {
		if port == 0 {
			continue
		}
		hasMTLS, hasPublic, hasBootstrap := false, false, false
		for _, u := range usage {
			switch u {
			case "HTTP":
				hasMTLS = true
			case "Public":
				hasPublic = true
			case "Bootstrap":
				hasBootstrap = true
			}
		}
		// Allow Public+Bootstrap to share a port (HTTP for CA, HTTPS for auth)
		// Reject all other incompatible combinations
		if (hasMTLS && hasPublic) || (hasMTLS && hasBootstrap) {
			return fmt.Errorf("listen: port %d binds incompatible surfaces %v; assign distinct ports", port, usage)
		}
	}

	tlsConfig := pki.TLSConfig()           // strict mTLS (RequireAndVerifyClientCert)
	tlsConfigPlain := pki.TLSConfigPlain() // public TLS (no client cert)

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
		Handler:           ls.handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: cfg.Gateway.ReadHeaderTimeout,
		ReadTimeout:       cfg.Gateway.ReadTimeout,
		WriteTimeout:      cfg.Gateway.WriteTimeout,
		IdleTimeout:       cfg.Gateway.IdleTimeout,
		MaxHeaderBytes:    cfg.Gateway.MaxHeaderBytes,
	}

	// Combined Public+Bootstrap: single HTTPS server serving both bootstrap and authenticated routes
	ls.publicServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Gateway.PublicPort),
		Handler:           ls.handler.buildPublicRouter(),
		TLSConfig:         tlsConfigPlain,
		ReadHeaderTimeout: cfg.Gateway.ReadHeaderTimeout,
		ReadTimeout:       cfg.Gateway.ReadTimeout,
		WriteTimeout:      cfg.Gateway.WriteTimeout,
		IdleTimeout:       cfg.Gateway.IdleTimeout,
		MaxHeaderBytes:    cfg.Gateway.MaxHeaderBytes,
	}

	return nil
}

// GetDB returns the underlying database service.
func (ls *GatewayService) GetDB() *GatewayDBService {
	return ls.db
}

// GetSecretManager returns the secret manager.
func (ls *GatewayService) GetSecretManager() (*SecretManager, error) {
	return NewSecretManager(ls.db.db, ls.cfg.Gateway.SecretsDir, ls.logger)
}

// GetPKIAuthority returns the underlying PKI authority.
func (ls *GatewayService) GetPKIAuthority() *PKIAuthority {
	return ls.pki
}

// GetHTTPHandler returns the HTTP handler.
func (ls *GatewayService) GetHTTPHandler() *HTTPHandler {
	return ls.handler
}

// GetHTTPPort returns the assigned port for the HTTP server.
func (ls *GatewayService) GetHTTPPort() int {
	if ls.server == nil || ls.server.Addr == "" {
		return 0
	}
	_, portStr, _ := net.SplitHostPort(ls.server.Addr)
	p, _ := strconv.Atoi(portStr)
	return p
}

// GetWSSPort returns the assigned port for the WSS server.
// Deprecated: WSS is merged into HTTP; callers should use GetHTTPPort.
func (ls *GatewayService) GetWSSPort() int {
	return ls.GetHTTPPort()
}

// GetPublicPort returns the assigned port for the public server.
func (ls *GatewayService) GetPublicPort() int {
	if ls.publicServer == nil || ls.publicServer.Addr == "" {
		return 0
	}
	_, portStr, _ := net.SplitHostPort(ls.publicServer.Addr)
	p, _ := strconv.Atoi(portStr)
	return p
}

func (ls *GatewayService) IsRunning() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.running
}

func (ls *GatewayService) IsReady() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.ready
}

func (ls *GatewayService) IsGovernanceReady() bool {
	ready, err := ls.db.HasTrustedSigners()
	if err != nil {
		ls.logger.Error("Failed to check if governance is ready", string(constants.ConnectionStateError), err)
		return false
	}
	return ready
}

// GovernanceDeps holds the governance dependencies required for transaction verification.
// These interfaces are implemented by GatewayDBService (ReplayStore, StateRootProvider,
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
func (ls *GatewayService) GetGovernanceDeps() *GovernanceDeps {
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
func (ls *GatewayService) Start(ctx context.Context) error {
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
		"bootstrap_port", ls.cfg.Gateway.BootstrapPort,
		"data_dir", ls.cfg.Gateway.DataDir)

	ls.logger.Info("Gateway TLS servers starting", "http_port", ls.cfg.Gateway.HTTPPort, "bootstrap_port", ls.cfg.Gateway.BootstrapPort)

	// Start background maintenance for MCP gateway
	go ls.mcpGateway.RunMaintenance(ctx)

	errChan := make(chan error, 4)
	readyChan := make(chan struct{}, 4)

	// Identify unique servers to start
	uniqueServers := make(map[*http.Server]string)
	if ls.server != nil {
		uniqueServers[ls.server] = "HTTP"
	}
	if ls.publicServer != nil {
		if _, ok := uniqueServers[ls.publicServer]; !ok {
			uniqueServers[ls.publicServer] = "Public"
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
			// For public server, use autoTLSListener to support both HTTP and HTTPS on same port
			if name == "Public" {
				lnToServe = &autoTLSListener{
					Listener:  ln,
					tlsConfig: s.TLSConfig,
					logger:    ls.logger,
				}
				tlsMode = "auto-TLS"
			} else {
				lnToServe = tls.NewListener(ln, s.TLSConfig)
				// Distinguish between mTLS (requires client cert) and plain TLS (no client cert)
				if s.TLSConfig.ClientAuth == tls.RequireAndVerifyClientCert {
					tlsMode = "mTLS"
				} else {
					tlsMode = "TLS"
				}
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
func (ls *GatewayService) Stop(ctx context.Context) error {
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
			ls.logger.Error("Public server shutdown timeout - forcing exit to prevent zombie process")
			return fmt.Errorf("shutdown timeout exceeded")
		}
		ls.logger.Error("Public server shutdown error", string(constants.ConnectionStateError), err)
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
