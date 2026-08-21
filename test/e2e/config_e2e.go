// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// loadE2EConfig resolves the repository root, loads CLI configuration from the
// local .g8e/ runtime tree, and reads the owner CLI session ID from stored
// credentials. It returns an error if any step fails — callers (TestMain) fail
// closed on error rather than skipping. The gateway HTTP/HTTPS URLs are
// derived from CLI config; the ensemble and dashboard URLs use the
// docker-compose deployment default ports.
func loadE2EConfig() (*e2eConfig, error) {
	repoRoot, err := resolveRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("e2e: resolve repo root: %w", err)
	}

	cfg, err := config.Load(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("e2e: load CLI config: %w", err)
	}

	fileSvc, err := fs.NewRuntimeFileService(repoRoot, testutil.NewTestLogger())
	if err != nil {
		return nil, fmt.Errorf("e2e: create file service: %w", err)
	}

	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("e2e: load credentials: %w", err)
	}
	if err := validateCredentials(creds); err != nil {
		return nil, fmt.Errorf("e2e: %w", err)
	}

	gatewayHTTPURL := cfg.OperatorDiscoveryURL()
	gatewayHTTPSURL := cfg.OperatorPublicURL()

	ensembleURL, err := deriveEnsembleURL(gatewayHTTPURL)
	if err != nil {
		return nil, fmt.Errorf("e2e: derive ensemble URL: %w", err)
	}
	dashboardURL, err := deriveDashboardURL(gatewayHTTPURL)
	if err != nil {
		return nil, fmt.Errorf("e2e: derive dashboard URL: %w", err)
	}

	return &e2eConfig{
		gatewayHTTPURL:  gatewayHTTPURL,
		gatewayHTTPSURL: gatewayHTTPSURL,
		ensembleURL:     ensembleURL,
		dashboardURL:    dashboardURL,
		cliCertPath:     cfg.CLICertFile(),
		cliKeyPath:      cfg.CLIKeyFile(),
		caBundleRelPath: cfg.DefaultTrustBundleRelPath(),
		cliSessionID:    creds.CLISessionID,
		fileSvc:         fileSvc,
		cfg:             cfg,
	}, nil
}

// readCABundle reads the CA bundle from the runtime tree via fileSvc. Returns
// the raw PEM bytes.
func (c *e2eConfig) readCABundle(ctx context.Context) ([]byte, error) {
	return auth.ReadTrustBundle(c.fileSvc, c.cfg)
}