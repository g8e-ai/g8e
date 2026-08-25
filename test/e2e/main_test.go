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
	"net/http"
	"os"
	"testing"
)

// e2eCfg is the shared configuration loaded once by TestMain from the local
// .g8e/ runtime tree. All test functions read from this variable. It is nil
// only if TestMain failed before m.Run() — in which case the suite has already
// exited non-zero.
var e2eCfg *e2eConfig

// e2eClient is the shared typed E2E client constructed once by TestMain from
// e2eCfg. It owns bounded public and mTLS HTTP clients with strict TLS
// verification.
var e2eClient *E2EClient

// TestMain loads configuration from the local .g8e/ runtime tree, performs a
// bounded HTTP health check against the running gateway, and fails immediately
// if the platform is not reachable. It does not start, stop, restart, or
// inspect any containers. The user starts the production platform (docker
// compose up or ./g8e gw start) before running ./g8e test e2e.
//
// Failure semantics: E2E tests are Tier 3 and require a running platform.
// There is no opt-out. If the platform is not reachable, the suite exits
// non-zero with a concise error so a missing platform can never produce a
// false-green build with zero tests run.
func TestMain(m *testing.M) {
	cfg, err := loadE2EConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: E2E config load failed: %v\n", err)
		os.Exit(1)
	}
	e2eCfg = cfg

	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	if err := preflightHealthCheck(ctx, cfg); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "FATAL: E2E preflight failed — platform not reachable: %v\n", err)
		os.Exit(1)
	}
	cancel()

	client, err := newE2EClient(context.Background(), cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: E2E client construction failed: %v\n", err)
		os.Exit(1)
	}
	e2eClient = client

	os.Exit(m.Run())
}

// preflightHealthCheck performs bounded HTTP GET checks against the gateway,
// ensemble, and dashboard services. It returns an error if any service is not
// reachable or does not return 200 within the context deadline.
func preflightHealthCheck(ctx context.Context, cfg *e2eConfig) error {
	client := &http.Client{Timeout: healthCheckTimeout}

	// 1. Gateway health
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.gatewayHTTPURL+"/api/v1/health", nil)
	if err != nil {
		return fmt.Errorf("build gateway health request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gateway not reachable at %s: %w", cfg.gatewayHTTPURL, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway health endpoint returned status %d, expected 200", resp.StatusCode)
	}

	// 2. Ensemble health
	ensReq, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.ensembleURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("build ensemble health request: %w", err)
	}
	ensResp, err := client.Do(ensReq)
	if err != nil {
		return fmt.Errorf("ensemble not reachable at %s: %w", cfg.ensembleURL, err)
	}
	ensResp.Body.Close()
	if ensResp.StatusCode != http.StatusOK {
		return fmt.Errorf("ensemble health endpoint returned status %d, expected 200", ensResp.StatusCode)
	}

	// 3. Dashboard index
	dashReq, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.dashboardURL+"/", nil)
	if err != nil {
		return fmt.Errorf("build dashboard request: %w", err)
	}
	dashResp, err := client.Do(dashReq)
	if err != nil {
		return fmt.Errorf("dashboard not reachable at %s: %w", cfg.dashboardURL, err)
	}
	dashResp.Body.Close()
	if dashResp.StatusCode != http.StatusOK {
		return fmt.Errorf("dashboard index returned status %d, expected 200", dashResp.StatusCode)
	}

	return nil
}
