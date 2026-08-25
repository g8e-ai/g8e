// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package e2e

import (
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// resolveRepoRoot finds the repository root using go list -m, matching the
// pattern used by the integration test helpers.
func resolveRepoRoot() (string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list -m: %w", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return "", fmt.Errorf("go list -m returned empty directory")
	}
	return filepath.Clean(root), nil
}

// replacePort parses a URL, replaces its port, and returns the reconstructed
// URL string. Returns an error if the URL is not a valid http/https URL.
func replacePort(rawURL string, port int) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("expected http or https scheme, got %q in URL %q", parsed.Scheme, rawURL)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("URL %q has empty host", rawURL)
	}
	host := parsed.Hostname()
	parsed.Host = fmt.Sprintf("%s:%d", host, port)
	return parsed.String(), nil
}

// deriveEnsembleURL replaces the port in the gateway HTTP URL with the
// ensemble deployment default port. The ensemble runs on its own port
// (default 8000) alongside the gateway.
func deriveEnsembleURL(gatewayHTTPURL string) (string, error) {
	return replacePort(gatewayHTTPURL, constants.EnsembleDefaultPort)
}

// deriveDashboardURL replaces the port in the gateway HTTP URL with the
// dashboard deployment default port. The dashboard runs on its own port
// (default 3000) alongside the gateway.
func deriveDashboardURL(gatewayHTTPURL string) (string, error) {
	return replacePort(gatewayHTTPURL, constants.DashboardDefaultPort)
}

// validateCredentials checks that loaded owner credentials are present and
// contain the required CLI session ID. Returns a descriptive error for nil
// credentials or a missing session ID so TestMain fails closed with an
// actionable message rather than proceeding to authenticated requests that
// would fail with a less obvious 401.
func validateCredentials(creds *auth.Credentials) error {
	if creds == nil {
		return fmt.Errorf("no owner credentials found — run './g8e auth login' first")
	}
	if creds.CLISessionID == "" {
		return fmt.Errorf("credentials missing cli_session_id")
	}
	return nil
}
