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
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/exitcode"
	local_http_stdio "github.com/g8e-ai/g8e/internal/services/local_http_stdio"
)

// runInsecureMode starts the Operator in INSECURE MCP gateway mode.
// The Operator connects to an MCP gateway via WebSocket without any governance.
// This mode bypasses all L1/L2/L3 verification and is DANGEROUS.
// No g8e infrastructure (agent, client) is required.
func runInsecureMode(gatewayURL, token, nodeID, displayName, pathEnv, logLevel string) {
	logger, err := configureLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid log level '%s': %v\n", logLevel, err)
		os.Exit(1) // ExitConfigError equivalent
	}

	cfg, err := config.LoadLocalHttpStdio(config.LocalHttpStdioOptions{
		GatewayURL:  gatewayURL,
		Token:       token,
		NodeID:      nodeID,
		DisplayName: displayName,
		PathEnv:     pathEnv,
		LogLevel:    logLevel,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "INSECURE MCP configuration error: %v\n", err)
		os.Exit(1) // ExitConfigError equivalent
	}

	logger.Info("g8e - INSECURE MCP Gateway Mode", "version", version, "build", buildID)

	svc, err := local_http_stdio.NewLocalHttpStdioNodeService(
		cfg.GatewayURL,
		cfg.Token,
		cfg.NodeID,
		cfg.DisplayName,
		cfg.PathEnv,
		logger,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create INSECURE MCP node service: %v\n", err)
		os.Exit(exitcode.FromError(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := svc.Start(ctx); err != nil {
			logger.Error("INSECURE MCP node service failed", "error", err)
			os.Exit(exitcode.FromError(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	logger.Info("Received signal, shutting down", "signal", sig.String())
	cancel()
	svc.Stop()
	logger.Info("INSECURE MCP node host stopped")
}
