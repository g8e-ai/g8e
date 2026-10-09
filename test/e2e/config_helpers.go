// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package e2e

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// e2eRuntimeRootEnv names the directory whose .g8e/ tree the suite targets
// instead of the repository root. Scenarios that run their own isolated
// Gateway (g8e test scale) set it so the suite never reads
// the developer's own runtime.
const e2eRuntimeRootEnv = "G8E_E2E_RUNTIME_ROOT"

// resolveRuntimeRoot returns the directory holding the .g8e/ tree under test:
// G8E_E2E_RUNTIME_ROOT when set, otherwise the repository root.
func resolveRuntimeRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv(e2eRuntimeRootEnv)); root != "" {
		return filepath.Clean(root), nil
	}
	return resolveRepoRoot()
}

// Scenarios whose isolated Gateway listens on non-default ports (g8e test
// scale) name them here; the suite and the CLI processes it starts then dial
// those ports on localhost instead of the defaults.
const (
	e2eGatewayHTTPPortEnv  = "G8E_E2E_GATEWAY_HTTP_PORT"
	e2eGatewayHTTPSPortEnv = "G8E_E2E_GATEWAY_HTTPS_PORT"
)

// e2eGatewayPorts returns the Gateway ports named by the environment, or
// zeros when neither is set. Setting only one of them is an error.
func e2eGatewayPorts() (httpPort, httpsPort int, err error) {
	rawHTTP := strings.TrimSpace(os.Getenv(e2eGatewayHTTPPortEnv))
	rawHTTPS := strings.TrimSpace(os.Getenv(e2eGatewayHTTPSPortEnv))
	if rawHTTP == "" && rawHTTPS == "" {
		return 0, 0, nil
	}
	parse := func(name, raw string) (int, error) {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return 0, fmt.Errorf("%s must be a TCP port, got %q", name, raw)
		}
		return port, nil
	}
	if httpPort, err = parse(e2eGatewayHTTPPortEnv, rawHTTP); err != nil {
		return 0, 0, err
	}
	if httpsPort, err = parse(e2eGatewayHTTPSPortEnv, rawHTTPS); err != nil {
		return 0, 0, err
	}
	return httpPort, httpsPort, nil
}

// e2eGatewayEndpointArgs returns the global CLI flags that point a child g8e
// process at the environment's Gateway ports, or nil for the defaults.
func e2eGatewayEndpointArgs() ([]string, error) {
	httpPort, httpsPort, err := e2eGatewayPorts()
	if err != nil || httpPort == 0 {
		return nil, err
	}
	return []string{"-e", net.JoinHostPort(constants.LocalhostHostname, strconv.Itoa(httpPort)), "-p", strconv.Itoa(httpsPort)}, nil
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
	if creds.UserID == "" {
		return fmt.Errorf("credentials missing user_id")
	}
	if creds.OperatorID == "" {
		return fmt.Errorf("credentials missing operator_id")
	}
	if creds.OperatorSessionID == "" {
		return fmt.Errorf("credentials missing operator_session_id")
	}
	return nil
}
