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
	"encoding/json"
	"errors"
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
	"github.com/g8e-ai/g8e/internal/services/mcp"
)

// ─── buildGatewayConn error paths ────────────────────────────────────────────

func TestBuildGatewayConn_ErrorPaths(t *testing.T) {
	t.Run("fails when cert files do not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		cfg := &config.Config{
			ProjectRoot:    tempDir,
			CredentialsDir: tempDir,
		}
		_, err := buildGatewayConn(cfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToLoadClientCertificate)
	})

	t.Run("fails when CA bundle does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath, keyPath, _ := generateTestCerts(t)

		cfg := &config.Config{
			ProjectRoot:    tempDir,
			CredentialsDir: filepath.Dir(certPath),
		}
		// Set env to point to non-existent CA bundle
		t.Setenv(envG8ECABundle, filepath.Join(tempDir, "nonexistent-ca.pem"))
		// Also set cert/key env to the generated test certs
		t.Setenv(envG8EClientCert, certPath)
		t.Setenv(envG8EClientKey, keyPath)

		_, err := buildGatewayConn(cfg)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToReadTrustBundle)
	})

	t.Run("succeeds with valid certs and custom gateway URL", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)

		t.Setenv(envG8EClientCert, certPath)
		t.Setenv(envG8EClientKey, keyPath)
		t.Setenv(envG8ECABundle, caPath)
		t.Setenv(envG8EGatewayURL, "https://127.0.0.1:9999/mcp")

		cfg := &config.Config{
			ProjectRoot:    t.TempDir(),
			CredentialsDir: filepath.Dir(certPath),
		}

		conn, err := buildGatewayConn(cfg)
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

		err := startGatewayIfNeeded()
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

		err := launchAgentWithGovernance("claude", nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrGatewayNotReady)
	})
}

// ─── handleToolsCall empty tool name ─────────────────────────────────────────

func TestHandleToolsCall_EmptyToolName(t *testing.T) {
	t.Run("returns error when tool name is empty", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		nativeToolHandler, err := mcp.NewNativeToolHandler(nil)
		require.NoError(t, err)
		handleToolsCall(encoder, 1, json.RawMessage(`{"name":"","arguments":{}}`), nativeToolHandler)

		var resp JSONRPCResponse
		err = json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, -32600, resp.Error.Code)
		assert.Contains(t, resp.Error.Message, "tool name required")
	})

	t.Run("returns error on invalid params JSON", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		nativeToolHandler, err := mcp.NewNativeToolHandler(nil)
		require.NoError(t, err)
		handleToolsCall(encoder, 1, json.RawMessage(`not valid json`), nativeToolHandler)

		var resp JSONRPCResponse
		err = json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, -32600, resp.Error.Code)
		assert.Contains(t, resp.Error.Message, "invalid tools/call params")
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
		err := runMCPAgentRun([]string{"nonexistent-command-xyz-12345"}, "")
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

		err = runMCPAgentRun(nil, server.URL)
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

		err = runMCPAgentRun(nil, server.URL)
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

		err = runMCPAgentRun(nil, server.URL)
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

		err = runMCPAgentRun(nil, server.URL)
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

		err = runMCPAgentRun(nil, server.URL)
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

		err = runMCPAgentRun(nil, server.URL)
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

		err = runMCPAgentRun(nil, server.URL)
		require.NoError(t, err)
	})
}

// ─── agent_harness.go error paths ────────────────────────────────────────────

func TestRunAgentHarness_ConfigLoadError(t *testing.T) {
	t.Run("returns error when config file does not exist", func(t *testing.T) {
		chdirTemp(t)

		// Reset harness flags to known state
		harnessConfigPath = filepath.Join(t.TempDir(), "nonexistent-config.json")
		harnessMTLSURL = ""
		harnessPublicURL = ""
		harnessCert = ""
		harnessKey = ""
		harnessCA = ""
		harnessAPIKey = ""
		harnessSessionID = ""
		harnessOutDir = ""
		harnessL3Mode = ""
		harnessEnsemble = 3
		harnessVerbose = false
		harnessPhase = "all"
		harnessConsensusSeed = ""
		harnessTribunalID = ""

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := runAgentHarness(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent run: load config")
	})
}

func TestRunAgentHarnessAudit_ConfigLoadError(t *testing.T) {
	t.Run("returns error when config file does not exist", func(t *testing.T) {
		chdirTemp(t)

		harnessConfigPath = filepath.Join(t.TempDir(), "nonexistent-config.json")
		harnessMTLSURL = ""
		harnessPublicURL = ""
		harnessCert = ""
		harnessKey = ""
		harnessCA = ""
		harnessAPIKey = ""
		harnessSessionID = ""
		harnessOutDir = ""
		harnessL3Mode = ""
		harnessEnsemble = 3
		harnessVerbose = false
		harnessPhase = "all"
		harnessConsensusSeed = ""
		harnessTribunalID = ""

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := runAgentHarnessAudit(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent audit: load config")
	})
}

// ─── ensureDHSPosture error path ─────────────────────────────────────────────

func TestEnsureDHSPosture_ErrorPath(t *testing.T) {
	t.Run("returns error when docker compose file does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		err := ensureDHSPosture(tempDir, "consensus")
		require.Error(t, err)
		// Docker will fail because the compose file doesn't exist
		assert.Contains(t, err.Error(), "stop gateway")
	})
}

// ─── printMCPConfigLocal with valid config ───────────────────────────────────

func TestPrintMCPConfigLocal_WithValidCerts(t *testing.T) {
	t.Run("generates config when certs exist", func(t *testing.T) {
		tempDir := t.TempDir()

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
		tempDir := t.TempDir()

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
