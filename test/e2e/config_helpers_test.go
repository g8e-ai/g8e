// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package e2e

// This file contains Tier 1 unit tests for the non-e2e-tagged config helpers.
// The os.Chdir usage in TestResolveRepoRoot_error_outside_any_module is
// legitimate source-tree discovery: it changes cwd to a temp directory with no
// go.mod to exercise the `go list -m` error path. It does not align .g8e/
// runtime state — the temp dir has no .g8e/ tree and no RuntimeFileService is
// constructed.

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRepoRoot(t *testing.T) {
	t.Run("success in module directory", func(t *testing.T) {
		// resolveRepoRoot runs `go list -m` from the process working directory.
		// The test process runs inside the g8e module, so it must succeed and
		// return a clean, non-empty path.
		root, err := resolveRepoRoot()
		require.NoError(t, err)
		assert.NotEmpty(t, root)
		assert.Equal(t, filepath.Clean(root), root)
	})

	t.Run("error outside any module", func(t *testing.T) {
		// Run resolveRepoRoot from a temp directory with no go.mod to exercise
		// the error-wrapping path deterministically. resolveRepoRoot shells out
		// to `go list -m`, which fails outside a module. t.TempDir() creates a
		// directory under the system temp root, which has no go.mod ancestor.
		tmpDir := t.TempDir()

		// resolveRepoRoot uses the process cwd, so change into the temp dir.
		origDir, err := os.Getwd()
		require.NoError(t, err)
		require.NoError(t, os.Chdir(tmpDir))
		t.Cleanup(func() { _ = os.Chdir(origDir) })

		_, err = resolveRepoRoot()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "go list -m")
	})
}

func TestReplacePort(t *testing.T) {
	tests := []struct {
		name          string
		inputURL      string
		port          int
		expectedURL   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "http URL with port replacement",
			inputURL:    "http://localhost:8080",
			port:        8000,
			expectedURL: "http://localhost:8000",
			expectError: false,
		},
		{
			name:        "https URL with port replacement",
			inputURL:    "https://example.com:443",
			port:        3000,
			expectedURL: "https://example.com:3000",
			expectError: false,
		},
		{
			name:        "http URL without explicit port",
			inputURL:    "http://localhost",
			port:        8000,
			expectedURL: "http://localhost:8000",
			expectError: false,
		},
		{
			name:          "invalid scheme (ftp)",
			inputURL:      "ftp://localhost:8080",
			port:          8000,
			expectError:   true,
			errorContains: "expected http or https scheme",
		},
		{
			name:          "invalid scheme (ws)",
			inputURL:      "ws://localhost:8080",
			port:          8000,
			expectError:   true,
			errorContains: "expected http or https scheme",
		},
		{
			name:        "empty host",
			inputURL:    "http://:8080",
			port:        8000,
			expectedURL: "http://:8000",
			expectError: false,
			// Go's URL parser accepts empty host with port, so this is actually valid
		},
		{
			name:          "malformed URL",
			inputURL:      "not-a-url",
			port:          8000,
			expectError:   true,
			errorContains: "expected http or https scheme",
		},
		{
			name:        "URL with path preserved",
			inputURL:    "http://localhost:8080/path/to/resource",
			port:        8000,
			expectedURL: "http://localhost:8000/path/to/resource",
			expectError: false,
		},
		{
			name:        "URL with query preserved",
			inputURL:    "http://localhost:8080?query=value",
			port:        8000,
			expectedURL: "http://localhost:8000?query=value",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := replacePort(tt.inputURL, tt.port)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedURL, result)
				// Verify the result is a valid URL
				parsed, err := url.Parse(result)
				require.NoError(t, err)
				// parsed.Port() returns string, convert to int for comparison
				portStr := parsed.Port()
				require.NotEmpty(t, portStr)
				var portInt int
				_, err = fmt.Sscanf(portStr, "%d", &portInt)
				require.NoError(t, err)
				assert.Equal(t, tt.port, portInt)
			}
		})
	}
}

func TestDeriveEnsembleURL(t *testing.T) {
	tests := []struct {
		name          string
		gatewayHTTP   string
		expectedURL   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "standard gateway URL",
			gatewayHTTP: "http://localhost:8443",
			expectedURL: "http://localhost:8000",
			expectError: false,
		},
		{
			name:        "gateway with different port",
			gatewayHTTP: "http://192.168.1.100:9000",
			expectedURL: "http://192.168.1.100:8000",
			expectError: false,
		},
		{
			name:          "invalid scheme",
			gatewayHTTP:   "ftp://localhost:8443",
			expectError:   true,
			errorContains: "expected http or https scheme",
		},
		{
			name:        "empty host",
			gatewayHTTP: "http://:8443",
			expectedURL: "http://:8000",
			expectError: false,
			// Go's URL parser accepts empty host with port
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := deriveEnsembleURL(tt.gatewayHTTP)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedURL, result)
			}
		})
	}
}

func TestDeriveDashboardURL(t *testing.T) {
	tests := []struct {
		name          string
		gatewayHTTP   string
		expectedURL   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "standard gateway URL",
			gatewayHTTP: "http://localhost:8443",
			expectedURL: "http://localhost:3000",
			expectError: false,
		},
		{
			name:        "gateway with different port",
			gatewayHTTP: "http://192.168.1.100:9000",
			expectedURL: "http://192.168.1.100:3000",
			expectError: false,
		},
		{
			name:          "invalid scheme",
			gatewayHTTP:   "ftp://localhost:8443",
			expectError:   true,
			errorContains: "expected http or https scheme",
		},
		{
			name:        "empty host",
			gatewayHTTP: "http://:8443",
			expectedURL: "http://:3000",
			expectError: false,
			// Go's URL parser accepts empty host with port
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := deriveDashboardURL(tt.gatewayHTTP)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedURL, result)
			}
		})
	}
}

// TestValidateCredentials covers the partial-credential error paths extracted
// from loadE2EConfig. These run as Tier 1 unit tests with no file system or
// platform dependency.
func TestValidateCredentials(t *testing.T) {
	tests := []struct {
		name          string
		creds         *auth.Credentials
		expectError   bool
		errorContains string
	}{
		{
			name:          "nil credentials",
			creds:         nil,
			expectError:   true,
			errorContains: "no owner credentials found",
		},
		{
			name: "missing cli_session_id",
			creds: &auth.Credentials{
				OperatorSessionID: "op-session-123",
				UserID:            "user-456",
				OperatorID:        "op-789",
				CLISessionID:      "",
			},
			expectError:   true,
			errorContains: "missing cli_session_id",
		},
		{
			name: "valid credentials with cli_session_id",
			creds: &auth.Credentials{
				OperatorSessionID: "op-session-123",
				UserID:            "user-456",
				OperatorID:        "op-789",
				CLISessionID:      "cli-session-abc",
			},
			expectError: false,
		},
		{
			name: "valid credentials with only cli_session_id",
			creds: &auth.Credentials{
				CLISessionID: "cli-session-xyz",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCredentials(tt.creds)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}