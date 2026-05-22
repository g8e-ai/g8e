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

package listen

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

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/mcp"
)

// ListenService is the top-level orchestrator for --listen mode (operator).
// It acts as the platform's central persistence and messaging backbone.
// In this mode, the Operator does NOT execute commands or initiate outbound
// connections. It strictly serves inbound requests from platform components.
type ListenService struct {
	cfg    *config.Config
	logger *slog.Logger

	db              *ListenDBService
	pubsub          *PubSubBroker
	auth            *AuthService
	pki             *PKIAuthority
	reg             *RegistrationService
	passkey         *PasskeyService
	userSvc         *UserService
	sessionSvc      *SessionService
	apiKeySvc       *ApiKeyService
	mcpGateway      *mcp.GatewayService
	server          *http.Server
	bootstrapServer *http.Server
	publicServer    *http.Server

	handler *HTTPHandler

	mu      sync.Mutex
	running bool
	ready   bool
}

// NewListenService creates a new listen mode service.
func NewListenService(cfg *config.Config, logger *slog.Logger) (*ListenService, error) {
	db, err := OpenListenDBService(cfg.Listen.DataDir, cfg.Listen.SecretsDir, logger, false)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	pubsub := NewPubSubBroker(logger)
	pki := newPKIAuthority(cfg.Listen.DataDir, cfg.Listen.PKIDir, db, logger)
	userSvc := NewUserService(db, logger)
	auth := NewAuthService(db, pki, logger, userSvc, cfg.Listen.SecretsDir)
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

	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc)

	// Initialize passkey service for L3 brokerage
	passkeyCfg := &PasskeyConfig{
		RpID:   cfg.Listen.PasskeyRpID,
		RpName: cfg.Listen.PasskeyRpName,
	}
	passkey, err := NewPasskeyService(db, logger, passkeyCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkey service: %w", err)
	}

	apiKeySvc := NewApiKeyService(db, logger)

	ls := &ListenService{
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
		apiKeySvc:  apiKeySvc,
		mcpGateway: mcp.NewGatewayService(logger, db),
	}

	ls.initHandlersAndServers()

	return ls, nil
}

// newListenServiceFromComponents assembles a ListenService from pre-built components.
// Used in tests where the DB and pub/sub broker are constructed independently.
func newListenServiceFromComponents(cfg *config.Config, logger *slog.Logger, db *ListenDBService, pubsub *PubSubBroker) *ListenService {
	pki := newPKIAuthority(cfg.Listen.DataDir, cfg.Listen.PKIDir, db, logger)
	userSvc := NewUserService(db, logger)
	auth := NewAuthService(db, pki, logger, userSvc, cfg.Listen.SecretsDir)
	sessionSvc := NewSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc)

	// Initialize passkey service for L3 brokerage (test configuration)
	passkeyCfg := &PasskeyConfig{
		RpID:   cfg.Listen.PasskeyRpID,
		RpName: cfg.Listen.PasskeyRpName,
	}
	// Passkey service initialization is optional; ignore errors for test configuration
	passkey, _ := NewPasskeyService(db, logger, passkeyCfg) //nolint:errcheck

	apiKeySvc := NewApiKeyService(db, logger)

	ls := &ListenService{
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
		apiKeySvc:  apiKeySvc,
		mcpGateway: mcp.NewGatewayService(logger, db),
	}

	ls.initHandlersAndServers()

	return ls
}

func (ls *ListenService) initHandlersAndServers() {
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
	apiKeySvc := ls.apiKeySvc

	ls.mcpGateway.SetA2ADependencies(cfg.Listen.A2ADownstreamURL)
	publicBaseURL := fmt.Sprintf("https://localhost:%d", cfg.Listen.PublicPort)
	ls.mcpGateway.SetPublicBaseURL(publicBaseURL)
	ls.handler = newHTTPHandler(cfg, logger, db, pubsub, auth, pki, sessionSvc, reg, passkey, userSvc, apiKeySvc, ls.mcpGateway, ls.IsReady, ls.IsGovernanceReady)

	// Build a map of ports to identify port assignments.
	// Surfaces with different TLS client-auth requirements MUST NOT share a
	// port; sharing would force tls.VerifyClientCertIfGiven and downgrade
	// the mTLS execution boundary to an L7 check.
	portUsage := make(map[int][]string)
	portUsage[cfg.Listen.HTTPPort] = append(portUsage[cfg.Listen.HTTPPort], "HTTP")

	portUsage[cfg.Listen.BootstrapPort] = append(portUsage[cfg.Listen.BootstrapPort], "Bootstrap")
	portUsage[cfg.Listen.PublicPort] = append(portUsage[cfg.Listen.PublicPort], "Public")

	// Validate up front so collisions panic during init, not only when a
	// fresh listener is built. HTTP and WSS may share a port (both mTLS);
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
		if (hasMTLS && hasPublic) || (hasMTLS && hasBootstrap) || (hasPublic && hasBootstrap) {
			panic(fmt.Sprintf("listen: port %d binds incompatible surfaces %v; assign distinct ports", port, usage))
		}
	}

	tlsConfig := pki.TLSConfig()           // strict mTLS (RequireAndVerifyClientCert)
	tlsConfigPlain := pki.TLSConfigPlain() // public TLS (no client cert)

	// Each surface gets its own dedicated listener with a TLS config that
	// matches the surface's authentication contract. The mTLS surface MUST
	// use RequireAndVerifyClientCert; mixing it with any non-mTLS surface
	// on the same port would force VerifyClientCertIfGiven and downgrade
	// the execution boundary to an L7 check.
	ls.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.HTTPPort),
		Handler:           ls.handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ls.bootstrapServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.BootstrapPort),
		Handler:           ls.handler.buildBootstrapRouter(),
		TLSConfig:         nil, // Plain HTTP for bootstrap discovery
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ls.publicServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.PublicPort),
		Handler:           ls.handler.buildPublicRouter(),
		TLSConfig:         tlsConfigPlain,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// GetDB returns the underlying database service.
func (ls *ListenService) GetDB() *ListenDBService {
	return ls.db
}

// GetSecretManager returns the secret manager.
func (ls *ListenService) GetSecretManager() (*SecretManager, error) {
	return NewSecretManager(ls.db.db, ls.cfg.Listen.SecretsDir, ls.logger)
}

// GetPKIAuthority returns the underlying PKI authority.
func (ls *ListenService) GetPKIAuthority() *PKIAuthority {
	return ls.pki
}

// GetHTTPHandler returns the HTTP handler.
func (ls *ListenService) GetHTTPHandler() *HTTPHandler {
	return ls.handler
}

// GetHTTPPort returns the assigned port for the HTTP server.
func (ls *ListenService) GetHTTPPort() int {
	if ls.server == nil || ls.server.Addr == "" {
		return 0
	}
	_, portStr, _ := net.SplitHostPort(ls.server.Addr)
	p, _ := strconv.Atoi(portStr)
	return p
}

// GetWSSPort returns the assigned port for the WSS server.
// Deprecated: WSS is merged into HTTP; callers should use GetHTTPPort.
func (ls *ListenService) GetWSSPort() int {
	return ls.GetHTTPPort()
}

// GetBootstrapPort returns the assigned port for the bootstrap server.
func (ls *ListenService) GetBootstrapPort() int {
	if ls.bootstrapServer == nil || ls.bootstrapServer.Addr == "" {
		return 0
	}
	_, portStr, _ := net.SplitHostPort(ls.bootstrapServer.Addr)
	p, _ := strconv.Atoi(portStr)
	return p
}

// GetPublicPort returns the assigned port for the public server.
func (ls *ListenService) GetPublicPort() int {
	if ls.publicServer == nil || ls.publicServer.Addr == "" {
		return 0
	}
	_, portStr, _ := net.SplitHostPort(ls.publicServer.Addr)
	p, _ := strconv.Atoi(portStr)
	return p
}

func (ls *ListenService) IsRunning() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.running
}

func (ls *ListenService) IsReady() bool {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return ls.ready
}

func (ls *ListenService) IsGovernanceReady() bool {
	ready, err := ls.db.HasTrustedSigners()
	if err != nil {
		ls.logger.Error("Failed to check if governance is ready", string(constants.ConnectionStateError), err)
		return false
	}
	return ready
}

// GovernanceDeps holds the governance dependencies required for transaction verification.
// These interfaces are implemented by ListenDBService (ReplayStore, StateRootProvider,
// TransactionAuditStore) and CompositeL3Verifier (L3Notary).
type GovernanceDeps struct {
	ReplayStore       governance.ReplayStore
	StateRootProvider governance.StateRootProvider
	TransactionAudit  governance.TransactionAuditStore
	L3Notary          governance.L3Notary
	SignerStore       governance.SignerStore
}

// GetGovernanceDeps returns the governance dependencies for transaction verification.
// This enables the in-process PubSubCommandService to perform fail-closed verification.
// The L3 notary is a composite that handles both WebAuthn (web sessions) and mTLS (CLI sessions).
func (ls *ListenService) GetGovernanceDeps() *GovernanceDeps {
	// Create composite L3 notary that handles both web and CLI sessions
	cliL3 := NewCLIL3Notary(ls.db, ls.pki, ls.logger, ls.userSvc, ls.sessionSvc)
	compositeL3 := NewCompositeL3Verifier(ls.passkey, cliL3, ls.logger)

	return &GovernanceDeps{
		ReplayStore:       ls.db,
		StateRootProvider: ls.db,
		TransactionAudit:  ls.db,
		L3Notary:          compositeL3,
		SignerStore:       ls.db,
	}
}

// Start begins serving HTTP/WS requests. Blocks until the context is cancelled
// or the server encounters a fatal error.
func (ls *ListenService) Start(ctx context.Context) error {
	ls.mu.Lock()
	if ls.running {
		ls.mu.Unlock()
		return fmt.Errorf("listen service already running")
	}
	ls.running = true
	ls.mu.Unlock()

	ls.logger.Info("operator Listen Mode ready",
		"http_port", ls.cfg.Listen.HTTPPort,

		"bootstrap_port", ls.cfg.Listen.BootstrapPort,
		"data_dir", ls.cfg.Listen.DataDir)

	ls.logger.Info("Listen TLS servers starting", "http_port", ls.cfg.Listen.HTTPPort, "bootstrap_port", ls.cfg.Listen.BootstrapPort)

	// Start background maintenance for MCP gateway
	go ls.mcpGateway.RunMaintenance(ctx)

	errChan := make(chan error, 4)
	readyChan := make(chan struct{}, 4)

	// Identify unique servers to start
	uniqueServers := make(map[*http.Server]string)
	if ls.server != nil {
		uniqueServers[ls.server] = "HTTP"
	}
	if ls.bootstrapServer != nil {
		if _, ok := uniqueServers[ls.bootstrapServer]; !ok {
			uniqueServers[ls.bootstrapServer] = "Bootstrap"
		}
	}
	if ls.publicServer != nil {
		if _, ok := uniqueServers[ls.publicServer]; !ok {
			uniqueServers[ls.publicServer] = "Public"
		}
	}

	startServer := func(s *http.Server, name string) {
		ls.logger.Info("Starting listener", "server", name, "addr", s.Addr)

		// Use a temporary listener to signal readiness before blocking on Serve
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

		ls.logger.Info("TCP listener bound", "server", name, "addr", s.Addr)

		var lnToServe net.Listener = ln
		if s.TLSConfig != nil {
			lnToServe = tls.NewListener(ln, s.TLSConfig)
			ls.logger.Info("TLS listener bound", "server", name, "addr", s.Addr)
		} else {
			ls.logger.Info("Plain HTTP listener bound", "server", name, "addr", s.Addr)
		}

		readyChan <- struct{}{}
		ls.logger.Info("Starting server Serve", "server", name, "addr", s.Addr)
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
		ls.logger.Info("operator Listen Mode fully operational")
	}()

	return <-errChan
}

// Stop gracefully shuts down the HTTP server and closes the database.
func (ls *ListenService) Stop(ctx context.Context) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if !ls.running {
		return nil
	}

	ls.logger.Info("Shutting down listen service...")

	ls.ready = false

	if err := ls.server.Shutdown(ctx); err != nil {
		ls.logger.Error("HTTP server shutdown error", string(constants.ConnectionStateError), err)
	}
	if ls.bootstrapServer != nil {
		if err := ls.bootstrapServer.Shutdown(ctx); err != nil {
			ls.logger.Error("Bootstrap server shutdown error", string(constants.ConnectionStateError), err)
		}
	}
	if err := ls.publicServer.Shutdown(ctx); err != nil {
		ls.logger.Error("Public server shutdown error", string(constants.ConnectionStateError), err)
	}

	// Close pub/sub broker (disconnects all WebSocket clients)
	ls.pubsub.Close()

	// Close database
	if err := ls.db.Close(); err != nil {
		ls.logger.Error("Database close error", string(constants.ConnectionStateError), err)
	}

	ls.running = false
	ls.logger.Info("Listen service stopped")
	return nil
}
