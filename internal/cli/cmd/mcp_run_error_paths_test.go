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

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// ─── buildGatewayConn error paths ────────────────────────────────────────────

func TestBuildGatewayConn_ErrorPaths(t *testing.T) {
	t.Run("fails when trust bundle and cert files do not exist", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)
		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}
		_, err = buildGatewayConn(fileSvc, cfg, stdioCredentialFlags{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToReadTrustBundle)
	})

	t.Run("fails when CA bundle does not exist", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		certPath, keyPath, _ := generateTestCerts(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  filepath.Dir(certPath),
		}
		// Set env to point to non-existent CA bundle
		t.Setenv(envG8ECABundle, filepath.Join(tempDir, "nonexistent-ca.pem"))
		// Also set cert/key env to the generated test certs
		t.Setenv(envG8EClientCert, certPath)
		t.Setenv(envG8EClientKey, keyPath)

		_, err = buildGatewayConn(fileSvc, cfg, stdioCredentialFlags{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToReadTrustBundle)
	})

	t.Run("succeeds with valid certs and custom gateway URL", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		t.Setenv(envG8EClientCert, certPath)
		t.Setenv(envG8EClientKey, keyPath)
		t.Setenv(envG8ECABundle, caPath)
		t.Setenv(envG8EGatewayURL, "https://127.0.0.1:9999/mcp")

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  filepath.Dir(certPath),
		}

		conn, err := buildGatewayConn(fileSvc, cfg, stdioCredentialFlags{})
		require.NoError(t, err)
		assert.NotNil(t, conn)
		assert.Equal(t, "https://127.0.0.1:9999/mcp", conn.gatewayURL)
	})
}

// ─── runMCPStdioProxy config load error ──────────────────────────────────────

func TestRunMCPStdioProxy_ConfigLoadError(t *testing.T) {
	t.Run("returns wrapped error when config load fails", func(t *testing.T) {
		chdirTemp(t)

		originalLoad := configLoad
		configLoad = func(string) (*config.Config, error) {
			return nil, errors.New("no config on disk")
		}
		t.Cleanup(func() { configLoad = originalLoad })

		cmd := mcpStdioCmd()
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mcp: load config")
	})
}

// ─── startGatewayIfNeeded config load error ──────────────────────────────────

func TestStartGatewayIfNeeded_ConfigLoadError(t *testing.T) {
	t.Run("returns wrapped error when config load fails", func(t *testing.T) {
		chdirTemp(t)

		originalLoad := configLoad
		configLoad = func(string) (*config.Config, error) {
			return nil, errors.New("no config on disk")
		}
		t.Cleanup(func() { configLoad = originalLoad })

		err := startGatewayIfNeeded(newFileSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mcp: load config")
	})
}

// ─── launchAgentWithGovernance error path (startGatewayIfNeeded fails) ───────

func TestLaunchAgentWithGovernance_ConfigLoadError(t *testing.T) {
	t.Run("returns ErrGatewayNotReady when config load fails", func(t *testing.T) {
		chdirTemp(t)

		originalLoad := configLoad
		configLoad = func(string) (*config.Config, error) {
			return nil, errors.New("no config")
		}
		t.Cleanup(func() { configLoad = originalLoad })

		err := launchAgentWithGovernance("claude", nil, false, newFileSvc)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrGatewayNotReady)
	})
}

// ─── proxySessionToGateway connection refused ────────────────────────────────

func TestProxySessionToGateway_ConnectionRefused(t *testing.T) {
	t.Run("returns error when gateway is unreachable", func(t *testing.T) {
		session := &gatewayConn{
			client:     &http.Client{Timeout: 1 * time.Second},
			gatewayURL: "http://127.0.0.1:1/mcp", // port 1 should refuse connections
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/list",
		}

		_, err := proxySessionToGateway(session, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mcp: execute request")
	})
}

// ─── runMCPAgentRun subprocess start failure ─────────────────────────────────

func TestRunMCPAgentRun_SubprocessStartFailure(t *testing.T) {
	t.Run("returns ErrProcessStartFailed for non-existent command", func(t *testing.T) {
		err := runMCPAgentRun([]string{"nonexistent-command-xyz-12345"}, "", false, newFileSvc)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrProcessStartFailed)
	})
}

// ─── runMCPAgentRun HTTP proxy mode with empty stdin ─────────────────────────

func TestRunMCPAgentRun_HTTPProxyEmptyStdin(t *testing.T) {
	t.Run("returns nil with empty stdin and --url flag", func(t *testing.T) {
		// Create a mock downstream server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result:  map[string]interface{}{"status": "ok"},
			})
		}))
		defer server.Close()

		// Replace os.Stdin with an empty pipe
		r, w, err := os.Pipe()
		require.NoError(t, err)
		_ = w.Close() // close write end so reads get EOF immediately
		defer r.Close()

		originalStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = originalStdin })

		err = runMCPAgentRun(nil, server.URL, false, newFileSvc)
		require.NoError(t, err)
	})
}

// ─── runMCPAgentRun HTTP proxy with L1 blocked tool call ─────────────────────

func TestRunMCPAgentRun_HTTPProxyL1Blocked(t *testing.T) {
	t.Run("L1 blocks dangerous tool call and sends error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result:  map[string]interface{}{"status": "ok"},
			})
		}))
		defer server.Close()

		// Create a pipe with a tools/call request that triggers L1 block
		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()

		// Write a tools/call request with a dangerous command (rm -rf)
		go func() {
			defer w.Close()
			req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"execute_bash","arguments":{"command":"rm -rf /var/log/g8e"}}}` + "\n"
			_, _ = w.Write([]byte(req))
		}()

		originalStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = originalStdin })

		err = runMCPAgentRun(nil, server.URL, false, newFileSvc)
		require.NoError(t, err)
	})
}

// ─── runMCPAgentRun HTTP proxy with notification (dropped) ───────────────────

func TestRunMCPAgentRun_HTTPProxyNotificationDropped(t *testing.T) {
	t.Run("notifications are silently dropped", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{JSONRPC: "2.0"})
		}))
		defer server.Close()

		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()

		go func() {
			defer w.Close()
			// A notification (no id field) should be dropped
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"))
		}()

		originalStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = originalStdin })

		err = runMCPAgentRun(nil, server.URL, false, newFileSvc)
		require.NoError(t, err)
	})
}

// ─── runMCPAgentRun HTTP proxy with parse error ──────────────────────────────

func TestRunMCPAgentRun_HTTPProxyParseError(t *testing.T) {
	t.Run("invalid JSON sends parse error and continues", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{JSONRPC: "2.0"})
		}))
		defer server.Close()

		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()

		go func() {
			defer w.Close()
			_, _ = w.Write([]byte("not valid json\n"))
		}()

		originalStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = originalStdin })

		err = runMCPAgentRun(nil, server.URL, false, newFileSvc)
		require.NoError(t, err)
	})
}

// ─── runMCPAgentRun HTTP proxy with empty lines ──────────────────────────────

func TestRunMCPAgentRun_HTTPProxyEmptyLines(t *testing.T) {
	t.Run("empty lines are skipped", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(JSONRPCResponse{JSONRPC: "2.0"})
		}))
		defer server.Close()

		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()

		go func() {
			defer w.Close()
			_, _ = w.Write([]byte("\n\n\n"))
		}()

		originalStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = originalStdin })

		err = runMCPAgentRun(nil, server.URL, false, newFileSvc)
		require.NoError(t, err)
	})
}

// ─── runMCPAgentRun HTTP proxy with downstream error on initialize ───────────

func TestRunMCPAgentRun_HTTPProxyInitializeFallback(t *testing.T) {
	t.Run("initialize falls back to handleInitialize on downstream error", func(t *testing.T) {
		// Server that returns 500 to trigger downstream error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`error`))
		}))
		defer server.Close()

		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()

		go func() {
			defer w.Close()
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"))
		}()

		originalStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = originalStdin })

		err = runMCPAgentRun(nil, server.URL, false, newFileSvc)
		require.NoError(t, err)
	})
}

// ─── runMCPAgentRun HTTP proxy with downstream error on non-initialize ───────

func TestRunMCPAgentRun_HTTPProxyDownstreamError(t *testing.T) {
	t.Run("non-initialize downstream error sends error response", func(t *testing.T) {
		// Server that returns 500 to trigger downstream error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`error`))
		}))
		defer server.Close()

		r, w, err := os.Pipe()
		require.NoError(t, err)
		defer r.Close()

		go func() {
			defer w.Close()
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"))
		}()

		originalStdin := os.Stdin
		os.Stdin = r
		t.Cleanup(func() { os.Stdin = originalStdin })

		err = runMCPAgentRun(nil, server.URL, false, newFileSvc)
		require.NoError(t, err)
	})
}

// ─── agent_harness.go error paths ────────────────────────────────────────────

func TestRunAgentHarness_ConfigLoadError(t *testing.T) {
	t.Run("returns error when config file does not exist", func(t *testing.T) {
		chdirTemp(t)

		// Reset harness flags to known state
		harnessConfigPath = filepath.Join(testutil.TempDir(t), "nonexistent-config.json")
		harnessMTLSURL = ""
		harnessPublicURL = ""
		harnessCert = ""
		harnessKey = ""
		harnessCA = ""
		harnessAPIKey = ""
		harnessSessionID = ""
		harnessOutDir = ""
		harnessVerbose = false
		harnessPhase = "all"

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := runAgentHarness(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scenarios run: load config")
	})
}

// ─── switchDemoPosture error path ────────────────────────────────────────────

func TestSwitchDHSPosture_ErrorPath(t *testing.T) {
	t.Run("returns error when docker compose file does not exist", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		err := switchDHSPosture(tempDir, "consensus")
		require.Error(t, err)
		// Docker will fail because the compose file doesn't exist
		assert.Contains(t, err.Error(), "stop gateway")
	})
}

func TestSwitchFedRAMPPosture_ErrorPath(t *testing.T) {
	t.Run("returns error when docker compose file does not exist", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		err := switchFedRAMPPosture(tempDir, "consensus")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stop gateway")
	})
}

// ─── printMCPConfigLocal with valid config ───────────────────────────────────

func TestPrintMCPConfigLocal_WithValidCerts(t *testing.T) {
	t.Run("generates config when certs exist", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

		certPath, keyPath, caPath := generateTestCerts(t)

		// Create the expected directory structure
		cfgDir := filepath.Join(tempDir, constants.RuntimeDirname, constants.PkiDirname, constants.PkiSubdirClient)
		require.NoError(t, os.MkdirAll(cfgDir, 0755))

		// Copy test certs to expected locations
		cliCert := filepath.Join(cfgDir, constants.CliCertFilename)
		cliKey := filepath.Join(cfgDir, constants.CliKeyFilename)
		caDir := filepath.Join(tempDir, constants.RuntimeDirname, constants.PkiDirname, constants.PkiSubdirTrust)
		require.NoError(t, os.MkdirAll(caDir, 0755))
		caBundle := filepath.Join(caDir, constants.PkiFileGatewayBundle)

		certData, err := os.ReadFile(certPath)
		require.NoError(t, err)
		keyData, err := os.ReadFile(keyPath)
		require.NoError(t, err)
		caData, err := os.ReadFile(caPath)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(cliCert, certData, 0644))
		require.NoError(t, os.WriteFile(cliKey, keyData, 0644))
		require.NoError(t, os.WriteFile(caBundle, caData, 0644))

		t.Setenv("G8E_PROJECT_ROOT", tempDir)

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = printMCPConfigLocal(cmd)
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "mcpServers")
		assert.Contains(t, output, "g8e")
	})
}

func TestPrintMCPConfigIP_WithValidCerts(t *testing.T) {
	t.Run("generates IP config when certs exist", func(t *testing.T) {
		tempDir := testutil.TempDir(t)

		certPath, keyPath, caPath := generateTestCerts(t)

		cfgDir := filepath.Join(tempDir, constants.RuntimeDirname, constants.PkiDirname, constants.PkiSubdirClient)
		require.NoError(t, os.MkdirAll(cfgDir, 0755))

		cliCert := filepath.Join(cfgDir, constants.CliCertFilename)
		cliKey := filepath.Join(cfgDir, constants.CliKeyFilename)
		caDir := filepath.Join(tempDir, constants.RuntimeDirname, constants.PkiDirname, constants.PkiSubdirTrust)
		require.NoError(t, os.MkdirAll(caDir, 0755))
		caBundle := filepath.Join(caDir, constants.PkiFileGatewayBundle)

		certData, _ := os.ReadFile(certPath)
		keyData, _ := os.ReadFile(keyPath)
		caData, _ := os.ReadFile(caPath)

		require.NoError(t, os.WriteFile(cliCert, certData, 0644))
		require.NoError(t, os.WriteFile(cliKey, keyData, 0644))
		require.NoError(t, os.WriteFile(caBundle, caData, 0644))

		t.Setenv("G8E_PROJECT_ROOT", tempDir)

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err := printMCPConfigIP(cmd)
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "mcpServers")
		assert.Contains(t, output, "g8e")
	})
}

// ─── buildGatewayConn flag resolution ─────────────────────────────────────────

func TestBuildGatewayConn_FlagResolution(t *testing.T) {
	t.Run("flag-only credentials succeed", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			ClientCert: certPath,
			ClientKey:  keyPath,
			CABundle:   caPath,
		}

		conn, err := buildGatewayConn(fileSvc, cfg, flags)
		require.NoError(t, err)
		assert.NotNil(t, conn)
	})

	t.Run("flag beats env for cert/key", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		// Set env to wrong values, flags to correct values
		t.Setenv(envG8EClientCert, "/nonexistent/env-cert.crt")
		t.Setenv(envG8EClientKey, "/nonexistent/env-key.key")
		t.Setenv(envG8ECABundle, caPath)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			ClientCert: certPath,
			ClientKey:  keyPath,
		}

		conn, err := buildGatewayConn(fileSvc, cfg, flags)
		require.NoError(t, err)
		assert.NotNil(t, conn)
	})

	t.Run("gateway-url flag honored verbatim", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			ClientCert: certPath,
			ClientKey:  keyPath,
			CABundle:   caPath,
			GatewayURL: "https://10.0.0.99:8443/mcp",
		}

		conn, err := buildGatewayConn(fileSvc, cfg, flags)
		require.NoError(t, err)
		assert.Equal(t, "https://10.0.0.99:8443/mcp", conn.gatewayURL)
	})

	t.Run("ca-bundle under runtime root reads through fileSvc", func(t *testing.T) {
		certPath, keyPath, _ := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		// Write a CA bundle inside the runtime tree
		caRel := filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)
		caData := []byte("fake-ca-bundle")
		require.NoError(t, fileSvc.WriteFile(context.Background(), caRel, caData, constants.PermFilePublic))

		caAbs := fileSvc.Resolve(caRel)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			ClientCert: certPath,
			ClientKey:  keyPath,
			CABundle:   caAbs,
		}

		conn, err := buildGatewayConn(fileSvc, cfg, flags)
		require.NoError(t, err)
		assert.NotNil(t, conn)
	})
}

// ─── buildGatewayConn fail-closed ─────────────────────────────────────────────

func TestBuildGatewayConn_FailClosed(t *testing.T) {
	t.Run("app-cert without app-key returns ErrIncompleteCredentialPair", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			AppCert: "/tmp/app.crt",
		}

		_, err = buildGatewayConn(fileSvc, cfg, flags)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrIncompleteCredentialPair)
	})

	t.Run("client-key without client-cert returns ErrIncompleteCredentialPair", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			ClientKey: "/tmp/client.key",
		}

		_, err = buildGatewayConn(fileSvc, cfg, flags)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrIncompleteCredentialPair)
	})

	t.Run("http gateway URL returns ErrMCPConfigGatewayURLInvalidScheme", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			ClientCert: certPath,
			ClientKey:  keyPath,
			CABundle:   caPath,
			GatewayURL: "http://g8e.local:8443/mcp",
		}

		_, err = buildGatewayConn(fileSvc, cfg, flags)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrMCPConfigGatewayURLInvalidScheme)
	})

	t.Run("empty host in gateway URL returns ErrMCPConfigGatewayURLHostEmpty", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			ClientCert: certPath,
			ClientKey:  keyPath,
			CABundle:   caPath,
			GatewayURL: "https:///mcp",
		}

		_, err = buildGatewayConn(fileSvc, cfg, flags)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrMCPConfigGatewayURLHostEmpty)
	})
}

// ─── parseStdioCredentialFlags ────────────────────────────────────────────────

func TestParseStdioCredentialFlags(t *testing.T) {
	t.Run("zero value when nothing is set", func(t *testing.T) {
		cmd := mcpStdioCmd()
		cmd.SetArgs([]string{})
		// Execute to register flags
		require.NoError(t, cmd.ParseFlags([]string{}))

		flags, err := parseStdioCredentialFlags(cmd)
		require.NoError(t, err)
		assert.Equal(t, stdioCredentialFlags{}, flags)
	})

	t.Run("exact values after flag set", func(t *testing.T) {
		cmd := mcpStdioCmd()
		require.NoError(t, cmd.ParseFlags([]string{
			"--client-cert", "/tmp/cli.crt",
			"--client-key", "/tmp/cli.key",
			"--ca-bundle", "/tmp/ca.pem",
			"--gateway-url", "https://g8e.local:8443/mcp",
			"--app-cert", "/tmp/app.crt",
			"--app-key", "/tmp/app.key",
		}))

		flags, err := parseStdioCredentialFlags(cmd)
		require.NoError(t, err)
		assert.Equal(t, "/tmp/cli.crt", flags.ClientCert)
		assert.Equal(t, "/tmp/cli.key", flags.ClientKey)
		assert.Equal(t, "/tmp/ca.pem", flags.CABundle)
		assert.Equal(t, "https://g8e.local:8443/mcp", flags.GatewayURL)
		assert.Equal(t, "/tmp/app.crt", flags.AppCert)
		assert.Equal(t, "/tmp/app.key", flags.AppKey)
	})
}
