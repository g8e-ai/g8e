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
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
)

// ListenService is the top-level orchestrator for --listen mode (VSODB).
// It acts as the platform's central persistence and messaging backbone.
// In this mode, the Operator does NOT execute commands or initiate outbound
// connections. It strictly serves inbound requests from platform components.
type ListenService struct {
	cfg    *config.Config
	logger *slog.Logger

	db        *ListenDBService
	pubsub    *PubSubBroker
	auth      *AuthService
	certs     *CertStore
	server    *http.Server
	wssServer *http.Server

	handler *HTTPHandler

	mu      sync.Mutex
	running bool
	ready   bool
}

// NewListenService creates a new listen mode service.
func NewListenService(cfg *config.Config, logger *slog.Logger) (*ListenService, error) {
	db, err := NewListenDBService(cfg.Listen.DataDir, cfg.Listen.SSLDir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	pubsub := NewPubSubBroker(logger)
	auth := NewAuthService(db, logger)

	var certs *CertStore
	var tlsConfig *tls.Config

	if cfg.Listen.TLSCertPath != "" && cfg.Listen.TLSKeyPath != "" {
		logger.Info("[CERTS] Using externally-managed TLS certificate",
			"cert", cfg.Listen.TLSCertPath, "key", cfg.Listen.TLSKeyPath)
		extCert, err := tls.LoadX509KeyPair(cfg.Listen.TLSCertPath, cfg.Listen.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
		}
		tlsConfig = &tls.Config{
			Certificates: []tls.Certificate{extCert},
			MinVersion:   tls.VersionTLS12,
		}
	} else {
		certs = newCertStore(cfg.Listen.DataDir, cfg.Listen.SSLDir, logger)

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

		if err := certs.EnsureCerts(extraIPs); err != nil {
			return nil, fmt.Errorf("failed to ensure TLS certificates: %w", err)
		}
		tlsConfig = certs.TLSConfig()
	}

	ls := &ListenService{
		cfg:    cfg,
		logger: logger,
		db:     db,
		pubsub: pubsub,
		auth:   auth,
		certs:  certs,
	}

	ls.handler = newHTTPHandler(cfg, logger, db, pubsub, auth, ls.IsReady)
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

	return ls, nil
}

// newListenServiceFromComponents assembles a ListenService from pre-built components.
// Used in tests where the DB and pub/sub broker are constructed independently.
func newListenServiceFromComponents(cfg *config.Config, logger *slog.Logger, db *ListenDBService, pubsub *PubSubBroker) *ListenService {
	auth := NewAuthService(db, logger)
	ls := &ListenService{
		cfg:    cfg,
		logger: logger,
		db:     db,
		pubsub: pubsub,
		auth:   auth,
	}

	ls.handler = newHTTPHandler(cfg, logger, db, pubsub, auth, ls.IsReady)
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

	// Start KV TTL cleanup goroutine
	go ls.db.RunTTLCleanup(ctx)

	ls.logger.Info("VSODB Listen Mode ready",
		"http_port", ls.cfg.Listen.HTTPPort,
		"wss_port", ls.cfg.Listen.WSSPort,
		"data_dir", ls.cfg.Listen.DataDir)

	ls.logger.Info("Listen TLS servers starting", "http_port", ls.cfg.Listen.HTTPPort, "wss_port", ls.cfg.Listen.WSSPort)

	errChan := make(chan error, 2)
	readyChan := make(chan struct{}, 2)

	startServer := func(s *http.Server, name string) {
		ls.logger.Info("Starting TLS listener", "server", name, "addr", s.Addr)

		// Use a temporary listener to signal readiness before blocking on Serve
		ln, err := net.Listen("tcp", s.Addr)
		if err != nil {
			ls.logger.Error("Failed to listen", "server", name, "addr", s.Addr, "error", err)
			errChan <- err
			return
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

	// Wait for both to be ready before marking service as ready
	go func() {
		for i := 0; i < 2; i++ {
			select {
			case <-readyChan:
			case <-ctx.Done():
				return
			}
		}
		ls.mu.Lock()
		ls.ready = true
		ls.mu.Unlock()
		ls.logger.Info("VSODB Listen Mode fully operational")
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

	ls.mu.Lock()
	ls.ready = false
	ls.mu.Unlock()

	if err := ls.server.Shutdown(ctx); err != nil {
		ls.logger.Error("HTTP server shutdown error", "error", err)
	}
	if err := ls.wssServer.Shutdown(ctx); err != nil {
		ls.logger.Error("WSS server shutdown error", "error", err)
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
