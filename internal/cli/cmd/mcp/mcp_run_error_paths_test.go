// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"

	// ─── buildGatewayConn error paths ────────────────────────────────────────────
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/demos"
)

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
		_, err = buildGatewayConn(fileSvc, cfg, stdioCredentialFlags{
			ClientCert: certPath,
			ClientKey:  keyPath,
			CABundle:   filepath.Join(tempDir, "nonexistent-ca.pem"),
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToReadTrustBundle)
	})

	t.Run("succeeds with valid certs and custom gateway URL", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  filepath.Dir(certPath),
		}

		conn, err := buildGatewayConn(fileSvc, cfg, stdioCredentialFlags{
			ClientCert: certPath,
			ClientKey:  keyPath,
			CABundle:   caPath,
			GatewayURL: "https://127.0.0.1:9999/mcp",
		})
		require.NoError(t, err)
		assert.NotNil(t, conn)
		assert.Equal(t, "https://127.0.0.1:9999/mcp", conn.gatewayURL)
	})
}

// ─── runMCPStdioProxy config load error ──────────────────────────────────────

func TestRunMCPStdioProxy_ConfigLoadError(t *testing.T) {
	t.Run("returns wrapped error when config load fails", func(t *testing.T) {
		cmdtest.ChdirTemp(t)

		originalLoad := shared.ConfigLoad
		shared.ConfigLoad = func(string) (*config.Config, error) {
			return nil, fmt.Errorf("no config on disk")
		}
		t.Cleanup(func() { shared.ConfigLoad = originalLoad })

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
		cmdtest.ChdirTemp(t)

		originalLoad := shared.ConfigLoad
		shared.ConfigLoad = func(string) (*config.Config, error) {
			return nil, fmt.Errorf("no config on disk")
		}
		t.Cleanup(func() { shared.ConfigLoad = originalLoad })

		err := startGatewayIfNeeded(shared.NewFileSvc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mcp: load config")
	})
}

// ─── launchAgentWithGovernance error path (startGatewayIfNeeded fails) ───────

func TestLaunchAgentWithGovernance_ConfigLoadError(t *testing.T) {
	t.Run("returns ErrGatewayNotReady when config load fails", func(t *testing.T) {
		cmdtest.ChdirTemp(t)

		originalLoad := shared.ConfigLoad
		shared.ConfigLoad = func(string) (*config.Config, error) {
			return nil, fmt.Errorf("no config")
		}
		t.Cleanup(func() { shared.ConfigLoad = originalLoad })

		err := launchAgentWithGovernance("claude", nil, false, shared.NewFileSvc, authcmd.PanickingEnrollerFactory())
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

// ─── agent_harness.go error paths ────────────────────────────────────────────

func TestRunAgentHarness_ConfigLoadError(t *testing.T) {
	t.Run("returns error when config file does not exist", func(t *testing.T) {
		cmdtest.ChdirTemp(t)

		// Reset harness flags to known state
		demos.HarnessConfigPath = filepath.Join(testutil.TempDir(t), "nonexistent-config.json")
		demos.HarnessMTLSURL = ""
		demos.HarnessPublicURL = ""
		demos.HarnessCert = ""
		demos.HarnessKey = ""
		demos.HarnessCA = ""
		demos.HarnessAPIKey = ""
		demos.HarnessSessionID = ""
		demos.HarnessOutDir = ""
		demos.HarnessVerbose = false
		demos.HarnessPhase = "all"

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := demos.RunAgentHarness(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scenarios run: load config")
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

	t.Run("ambient G8E_* env credentials are ignored", func(t *testing.T) {
		certPath, keyPath, caPath := generateTestCerts(t)
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		// Valid credentials supplied only through the retired env channel must not
		// be consulted: with no flags and no managed trust bundle the bridge fails
		// closed rather than silently adopting them.
		t.Setenv("G8E_CLIENT_CERT", certPath)
		t.Setenv("G8E_CLIENT_KEY", keyPath)
		t.Setenv("G8E_CA_BUNDLE", caPath)
		t.Setenv("G8E_GATEWAY_URL", "https://127.0.0.1:9999/mcp")

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		_, err = buildGatewayConn(fileSvc, cfg, stdioCredentialFlags{})
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrFailedToReadTrustBundle)
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
	t.Run("client-cert without client-key returns ErrIncompleteCredentialPair", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)

		cfg := &config.Config{
			ProjectRoot: tempDir,
			RuntimeDir:  tempDir,
		}

		flags := stdioCredentialFlags{
			ClientCert: "/tmp/client.crt",
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
			"--app", "test-app",
		}))

		flags, err := parseStdioCredentialFlags(cmd)
		require.NoError(t, err)
		assert.Equal(t, "/tmp/cli.crt", flags.ClientCert)
		assert.Equal(t, "/tmp/cli.key", flags.ClientKey)
		assert.Equal(t, "/tmp/ca.pem", flags.CABundle)
		assert.Equal(t, "https://g8e.local:8443/mcp", flags.GatewayURL)
		assert.Equal(t, "test-app", flags.App)
	})
}
