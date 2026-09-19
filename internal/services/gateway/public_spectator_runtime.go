// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// PublicSpectatorConfig configures the gateway-owned public mirror, publisher
// retransmit loop, and evaluation explorer static host.
type PublicSpectatorConfig struct {
	Enabled               bool
	PrivateListenAddress  string
	PublicListenAddress   string
	ExplorerListenAddress string
	PublicBaseURL         string
	SourceID              string
	ExplorerRoot          string
}

// DefaultPublicSpectatorConfig returns loopback defaults for local development.
func DefaultPublicSpectatorConfig() PublicSpectatorConfig {
	return PublicSpectatorConfig{
		Enabled:               true,
		PrivateListenAddress:  fmt.Sprintf("127.0.0.1:%d", constants.PublicSpectatorPrivatePort),
		PublicListenAddress:   fmt.Sprintf("127.0.0.1:%d", constants.PublicSpectatorPublicPort),
		ExplorerListenAddress: fmt.Sprintf("127.0.0.1:%d", constants.EvalExplorerDefaultPort),
		SourceID:              defaultPublicSpectatorSourceID,
	}
}

// PublicSpectatorRuntime owns the in-process public mirror listeners and the
// optional evaluation explorer static host.
type PublicSpectatorRuntime struct {
	cfg       PublicSpectatorConfig
	fileSvc   fs.RuntimeFileService
	logger    *slog.Logger
	publisher *PublicPublisherService
	mirror    *PublicMirrorServer

	privateServer  *http.Server
	publicServer   *http.Server
	explorerServer *http.Server

	mu      sync.Mutex
	running bool
}

// NewPublicSpectatorRuntime prepares the gateway-owned public spectator stack.
func NewPublicSpectatorRuntime(cfg PublicSpectatorConfig, fileSvc fs.RuntimeFileService, logger *slog.Logger) (*PublicSpectatorRuntime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.PrivateListenAddress == "" {
		cfg.PrivateListenAddress = DefaultPublicSpectatorConfig().PrivateListenAddress
	}
	if cfg.PublicListenAddress == "" {
		cfg.PublicListenAddress = DefaultPublicSpectatorConfig().PublicListenAddress
	}
	if cfg.ExplorerListenAddress == "" {
		cfg.ExplorerListenAddress = DefaultPublicSpectatorConfig().ExplorerListenAddress
	}
	allowContainerBind := allowContainerMirrorBind()
	if err := ValidatePublicMirrorListenAddresses(cfg.PrivateListenAddress, cfg.PublicListenAddress, allowContainerBind); err != nil {
		return nil, err
	}
	if err := ValidatePublicExplorerListenAddress(cfg.ExplorerListenAddress, cfg.PrivateListenAddress, cfg.PublicListenAddress, allowContainerBind); err != nil {
		return nil, err
	}
	return &PublicSpectatorRuntime{
		cfg:     cfg,
		fileSvc: fileSvc,
		logger:  logger,
	}, nil
}

// Start launches mirror listeners, wires the publisher retransmit loop, and
// serves the evaluation explorer when static assets are available.
func (runtime *PublicSpectatorRuntime) Start(ctx context.Context) error {
	if runtime == nil {
		return fmt.Errorf("public spectator: %w", constants.ErrMissingRequiredField)
	}
	runtime.mu.Lock()
	if runtime.running {
		runtime.mu.Unlock()
		return nil
	}
	runtime.mu.Unlock()

	privateMirrorOrigin := "http://" + runtime.cfg.PrivateListenAddress
	exportConfig, err := EnsureLocalPublicFeed(ctx, runtime.fileSvc, runtime.cfg.SourceID, privateMirrorOrigin)
	if err != nil {
		return fmt.Errorf("public spectator: ensure feed: %w", err)
	}
	if exportConfig.MirrorOrigin == "" {
		exportConfig.MirrorOrigin = "http://" + runtime.cfg.PrivateListenAddress
	} else if _, port, splitErr := net.SplitHostPort(runtime.cfg.PrivateListenAddress); splitErr == nil {
		expectedPrivate := "http://" + runtime.cfg.PrivateListenAddress
		expectedLocalhost := fmt.Sprintf("http://localhost:%s", port)
		if exportConfig.MirrorOrigin != expectedPrivate && exportConfig.MirrorOrigin != expectedLocalhost {
			runtime.logger.Warn("Public feed mirror origin does not match gateway private listener",
				"mirror_origin", exportConfig.MirrorOrigin,
				"private_listen", runtime.cfg.PrivateListenAddress)
		}
	}

	key, err := readPublicSecret(ctx, runtime.fileSvc, constants.PublicFeedSigningKeyPath, ed25519.PrivateKeySize, constants.ErrPublicFeedSigningKeyRequired)
	if err != nil {
		return fmt.Errorf("public spectator: %w", err)
	}
	token, err := readPublicSecret(ctx, runtime.fileSvc, constants.PublicFeedIngestTokenPath, constants.PublicFeedIngestTokenBytes, constants.ErrPublicFeedIngestTokenRequired)
	if err != nil {
		return fmt.Errorf("public spectator: %w", err)
	}

	mirror, err := NewPublicMirrorServer(runtime.logger, NewRuntimePublicMirrorStore(runtime.fileSvc))
	if err != nil {
		return err
	}
	mirror.SetIngestAuthToken(hex.EncodeToString(token))
	privateKey := ed25519.PrivateKey(key)
	if err := mirror.RegisterSourceKey(ctx, exportConfig.SourceID, exportConfig.SigningKeyID, privateKey.Public().(ed25519.PublicKey)); err != nil {
		return fmt.Errorf("public spectator: register source key: %w", err)
	}

	publisher := NewPublicPublisherService(nil, runtime.fileSvc, runtime.logger, exportConfig, privateKey, exportConfig.SigningKeyID)
	publisher.SetIngestAuthToken(hex.EncodeToString(token))
	publisher.SetMirrorOrigin("http://" + runtime.cfg.PrivateListenAddress)

	runtime.privateServer = newPublicMirrorHTTPServer(runtime.cfg.PrivateListenAddress, mirror.Handler())
	mirrorOrigin := resolveEvalExplorerMirrorOrigin(runtime.cfg.PublicBaseURL, runtime.cfg.PublicListenAddress)
	publicHandler := mirror.PublicHandler()
	explorerHandler, explorerErr := NewEvalExplorerHandler(runtime.cfg.ExplorerRoot, mirrorOrigin, false)
	if explorerErr == nil {
		publicHandler = combinePublicSpectatorHandler(publicHandler, explorerHandler)
	} else {
		runtime.logger.Warn("Evaluation explorer static assets unavailable", "error", explorerErr)
	}
	runtime.publicServer = newPublicMirrorHTTPServer(runtime.cfg.PublicListenAddress, publicHandler)
	if explorerErr == nil && shouldServeDedicatedExplorer(runtime.cfg) {
		// Dedicated explorer listeners (for example :5173) must target the local
		// public mirror API (:8082), not the gateway HTTPS base URL used for
		// approval links and tunnel fronting.
		dedicatedMirrorOrigin := publicMirrorURL(runtime.cfg.PublicListenAddress)
		dedicatedHandler, dedicatedErr := NewEvalExplorerHandler(runtime.cfg.ExplorerRoot, dedicatedMirrorOrigin, true)
		if dedicatedErr != nil {
			return fmt.Errorf("public spectator: dedicated explorer: %w", dedicatedErr)
		}
		runtime.explorerServer = newPublicMirrorHTTPServer(runtime.cfg.ExplorerListenAddress, dedicatedHandler)
	}
	runtime.mirror = mirror
	runtime.publisher = publisher

	errCh := make(chan error, 4)
	startServer := func(server *http.Server, label string) {
		if server == nil {
			return
		}
		go func() {
			runtime.logger.Info("Public spectator listener started", "surface", label, "addr", server.Addr)
			if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				errCh <- fmt.Errorf("public spectator %s: %w", label, serveErr)
			}
		}()
	}
	startServer(runtime.privateServer, "mirror-private")
	startServer(runtime.publicServer, "mirror-public")
	startServer(runtime.explorerServer, "explorer")

	runtime.mu.Lock()
	runtime.running = true
	runtime.mu.Unlock()

	go runtime.runPublisherRetransmitLoop(ctx)

	go func() {
		select {
		case serveErr := <-errCh:
			if serveErr != nil {
				runtime.logger.Error("Public spectator listener failed", "error", serveErr)
			}
		case <-ctx.Done():
		}
	}()

	return nil
}

// Serve starts the spectator listeners and blocks until ctx is cancelled.
func (runtime *PublicSpectatorRuntime) Serve(ctx context.Context) error {
	if err := runtime.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runtime.Stop(stopCtx)
}

// Stop shuts down mirror and explorer listeners.
func (runtime *PublicSpectatorRuntime) Stop(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	if !runtime.running {
		runtime.mu.Unlock()
		return nil
	}
	runtime.running = false
	runtime.mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, server := range []*http.Server{runtime.privateServer, runtime.publicServer, runtime.explorerServer} {
		if server == nil {
			continue
		}
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("public spectator: shutdown %s: %w", server.Addr, err)
		}
	}
	return nil
}

// Running reports whether the spectator listeners were started.
func (runtime *PublicSpectatorRuntime) Running() bool {
	if runtime == nil {
		return false
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.running
}

// Publisher returns the in-process publisher used for outbox retransmit.
func (runtime *PublicSpectatorRuntime) Publisher() *PublicPublisherService {
	if runtime == nil {
		return nil
	}
	return runtime.publisher
}

// PublicListenAddress returns the anonymous mirror listen address.
func (runtime *PublicSpectatorRuntime) PublicListenAddress() string {
	if runtime == nil {
		return ""
	}
	return runtime.cfg.PublicListenAddress
}

func allowContainerMirrorBind() bool {
	return strings.TrimSpace(os.Getenv("G8E_DOCKER_COMPOSE")) == "1"
}

func shouldServeDedicatedExplorer(cfg PublicSpectatorConfig) bool {
	if strings.TrimSpace(cfg.ExplorerListenAddress) == "" {
		return false
	}
	return !sameListenAddress(cfg.ExplorerListenAddress, cfg.PublicListenAddress)
}

func (runtime *PublicSpectatorRuntime) runPublisherRetransmitLoop(ctx context.Context) {
	if runtime.publisher == nil {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := runtime.publisher.RetransmitOutbox(ctx); err != nil {
				runtime.logger.Debug("Public publisher retransmit", "error", err)
			}
		}
	}
}
