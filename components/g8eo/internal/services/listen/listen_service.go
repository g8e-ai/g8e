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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/internal/config"
	"github.com/g8e-ai/g8e/components/g8eo/internal/services/governance"
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
	apiKeySvc       *ApiKeyService
	server          *http.Server
	wssServer       *http.Server
	bootstrapServer *http.Server
	publicServer    *http.Server

	handler *HTTPHandler

	mu      sync.Mutex
	running bool
	ready   bool
}

// NewListenService creates a new listen mode service.
func NewListenService(cfg *config.Config, logger *slog.Logger) (*ListenService, error) {
	db, err := NewListenDBService(cfg.Listen.DataDir, cfg.Listen.SecretsDir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	pubsub := NewPubSubBroker(logger)
	pki := newPKIAuthority(cfg.Listen.DataDir, cfg.Listen.PKIDir, db, logger)
	auth := NewAuthService(db, pki, logger, cfg.Listen.SecretsDir)

	var tlsConfig *tls.Config

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
	tlsConfig = pki.TLSConfig()
	tlsConfigPlain := pki.TLSConfigPlain()

	reg := NewRegistrationService(db, pki, logger)

	// Initialize passkey service for L3 brokerage
	passkeyCfg := &PasskeyConfig{
		RpID:   cfg.Listen.PasskeyRpID,
		RpName: cfg.Listen.PasskeyRpName,
	}
	passkey, err := NewPasskeyService(db, logger, passkeyCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize passkey service: %w", err)
	}

	userSvc := NewUserService(db, logger)
	apiKeySvc := NewApiKeyService(db, logger)

	ls := &ListenService{
		cfg:       cfg,
		logger:    logger,
		db:        db,
		pubsub:    pubsub,
		auth:      auth,
		pki:       pki,
		reg:       reg,
		passkey:   passkey,
		userSvc:   userSvc,
		apiKeySvc: apiKeySvc,
	}

	ls.handler = newHTTPHandler(cfg, logger, db, pubsub, auth, pki, reg, passkey, userSvc, apiKeySvc, ls.IsReady, ls.IsGovernanceReady)
	ls.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.HTTPPort),
		Handler:           ls.handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ls.wssServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.WSSPort),
		Handler:           ls.handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ls.bootstrapServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.BootstrapPort),
		Handler:           ls.handler.buildBootstrapRouter(),
		TLSConfig:         tlsConfigPlain,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ls.publicServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.PublicPort),
		Handler:           ls.handler.buildPublicRouter(),
		TLSConfig:         tlsConfigPlain, // No mTLS for browser BYO frontend
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return ls, nil
}

// newListenServiceFromComponents assembles a ListenService from pre-built components.
// Used in tests where the DB and pub/sub broker are constructed independently.
func newListenServiceFromComponents(cfg *config.Config, logger *slog.Logger, db *ListenDBService, pubsub *PubSubBroker) *ListenService {
	pki := newPKIAuthority(cfg.Listen.DataDir, cfg.Listen.PKIDir, db, logger)
	auth := NewAuthService(db, pki, logger, cfg.Listen.SecretsDir)
	reg := NewRegistrationService(db, pki, logger)

	// Initialize passkey service for L3 brokerage (test configuration)
	passkeyCfg := &PasskeyConfig{
		RpID:   cfg.Listen.PasskeyRpID,
		RpName: cfg.Listen.PasskeyRpName,
	}
	passkey, _ := NewPasskeyService(db, logger, passkeyCfg)

	userSvc := NewUserService(db, logger)
	apiKeySvc := NewApiKeyService(db, logger)

	ls := &ListenService{
		cfg:       cfg,
		logger:    logger,
		db:        db,
		pubsub:    pubsub,
		auth:      auth,
		pki:       pki,
		reg:       reg,
		passkey:   passkey,
		userSvc:   userSvc,
		apiKeySvc: apiKeySvc,
	}

	ls.handler = newHTTPHandler(cfg, logger, db, pubsub, auth, pki, reg, passkey, userSvc, apiKeySvc, ls.IsReady, ls.IsGovernanceReady)
	ls.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.HTTPPort),
		Handler:           ls.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ls.wssServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.WSSPort),
		Handler:           ls.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ls.bootstrapServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Listen.BootstrapPort),
		Handler:           ls.handler.buildBootstrapRouter(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return ls
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
	// Governance is ready if at least one trusted L2 signer is provisioned.
	// These are stored at <PKIDir>/trusted_signers/*.pub
	signersDir := filepath.Join(ls.cfg.Listen.PKIDir, "trusted_signers")
	entries, err := os.ReadDir(signersDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".pub") {
			return true
		}
	}
	return false
}

// GetDB returns the underlying database service.
func (ls *ListenService) GetDB() *ListenDBService {
	return ls.db
}

// GetSecretManager returns the secret manager.
func (ls *ListenService) GetSecretManager() *SecretManager {
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

// GetPubSubBroker returns the PubSub broker.
func (h *HTTPHandler) GetPubSubBroker() *PubSubBroker {
	return h.pubsub
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
func (ls *ListenService) GetWSSPort() int {
	if ls.wssServer == nil || ls.wssServer.Addr == "" {
		return 0
	}
	_, portStr, _ := net.SplitHostPort(ls.wssServer.Addr)
	p, _ := strconv.Atoi(portStr)
	return p
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

// GovernanceDeps holds the governance dependencies required for transaction verification.
// These interfaces are implemented by ListenDBService (ReplayStore, StateRootProvider,
// TransactionAuditStore) and PasskeyService (L3Verifier).
type GovernanceDeps struct {
	ReplayStore       governance.ReplayStore
	StateRootProvider governance.StateRootProvider
	TransactionAudit  governance.TransactionAuditStore
	L3Verifier        governance.L3Verifier
	SignerStore       governance.SignerStore
}

// GetGovernanceDeps returns the governance dependencies for transaction verification.
// This enables the in-process PubSubCommandService to perform fail-closed verification.
func (ls *ListenService) GetGovernanceDeps() *GovernanceDeps {
	return &GovernanceDeps{
		ReplayStore:       ls.db,
		StateRootProvider: ls.db,
		TransactionAudit:  ls.db,
		L3Verifier:        ls.passkey,
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
		"wss_port", ls.cfg.Listen.WSSPort,
		"bootstrap_port", ls.cfg.Listen.BootstrapPort,
		"data_dir", ls.cfg.Listen.DataDir)

	ls.logger.Info("Listen TLS servers starting", "http_port", ls.cfg.Listen.HTTPPort, "wss_port", ls.cfg.Listen.WSSPort, "bootstrap_port", ls.cfg.Listen.BootstrapPort)

	errChan := make(chan error, 4)
	readyChan := make(chan struct{}, 4)

	startServer := func(s *http.Server, name string) {
		ls.logger.Info("Starting TLS listener", "server", name, "addr", s.Addr)

		// Use a temporary listener to signal readiness before blocking on Serve
		ln, err := net.Listen("tcp", s.Addr)
		if err != nil {
			ls.logger.Error("Failed to listen", "server", name, "addr", s.Addr, "error", err)
			errChan <- err
			return
		}

		// Update server Addr if it was dynamic
		if s.Addr == ":0" {
			s.Addr = ln.Addr().String()
		}

		ls.logger.Info("TCP listener bound", "server", name, "addr", s.Addr)

		tlsLn := tls.NewListener(ln, s.TLSConfig)
		ls.logger.Info("TLS listener bound", "server", name, "addr", s.Addr)
		readyChan <- struct{}{}
		ls.logger.Info("Starting server Serve", "server", name, "addr", s.Addr)
		errChan <- s.Serve(tlsLn)
	}

	go startServer(ls.server, "HTTP")
	go startServer(ls.wssServer, "WSS")
	go startServer(ls.bootstrapServer, "Bootstrap")
	go startServer(ls.publicServer, "Public")

	// Wait for all four to be ready before marking service as ready
	go func() {
		for i := 0; i < 4; i++ {
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
		ls.logger.Error("HTTP server shutdown error", "error", err)
	}
	if err := ls.wssServer.Shutdown(ctx); err != nil {
		ls.logger.Error("WSS server shutdown error", "error", err)
	}
	if err := ls.bootstrapServer.Shutdown(ctx); err != nil {
		ls.logger.Error("Bootstrap server shutdown error", "error", err)
	}
	if err := ls.publicServer.Shutdown(ctx); err != nil {
		ls.logger.Error("Public server shutdown error", "error", err)
	}

	// Close pub/sub broker (disconnects all WebSocket clients)
	ls.pubsub.Close()

	// Close database
	if err := ls.db.Close(); err != nil {
		ls.logger.Error("Database close error", "error", err)
	}

	ls.running = false
	ls.logger.Info("Listen service stopped")
	return nil
}
