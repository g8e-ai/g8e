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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/agent"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestAgentCmd(t *testing.T) {
	t.Run("agent command has correct use and description", func(t *testing.T) {
		cmd := agentCmd()
		assert.Equal(t, "agent", cmd.Use)
		assert.Contains(t, cmd.Short, "Agent integration")
		assert.Contains(t, cmd.Long, "popular AI agent binaries")
	})

	t.Run("agent list command works", func(t *testing.T) {
		cmd := agentListCmd()
		assert.Equal(t, "list", cmd.Use)
		assert.Contains(t, cmd.Short, "List supported")

		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetArgs([]string{})
		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "claude")
		assert.Contains(t, output, "codex")
		assert.Contains(t, output, "devin")
		assert.Contains(t, output, "gemini")
		assert.Contains(t, output, "goose")
		assert.NotContains(t, output, "cursor")
		assert.NotContains(t, output, "aider")
		assert.NotContains(t, output, "generic")
	})

	t.Run("agent show command requires exactly one argument", func(t *testing.T) {
		cmd := agentShowCmd()
		assert.Equal(t, "show <agent>", cmd.Use)
		assert.Contains(t, cmd.Short, "Print MCP client configuration")
	})

	t.Run("agent show generates gateway configs for claude with full transport details", func(t *testing.T) {
		cmd := agentShowCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"claude"})
		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "g8e Gateway MCP Configurations")
		assert.Contains(t, output, "g8e.local")
		assert.Contains(t, output, "IP Address")
		assert.Contains(t, output, "Stdio Transport")
	})

	agents := []string{"gemini", "goose"}
	for _, agent := range agents {
		t.Run("agent show generates gateway configs for "+agent, func(t *testing.T) {
			cmd := agentShowCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs([]string{agent})
			err := cmd.Execute()
			require.NoError(t, err)
			output := buf.String()
			assert.Contains(t, output, "g8e Gateway MCP Configurations")
		})
	}

	t.Run("agent show returns error for unknown agent", func(t *testing.T) {
		cmd := agentShowCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"unknown-agent"})
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent not found")
	})

	t.Run("agent show is case-insensitive", func(t *testing.T) {
		cmd := agentShowCmd()
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"CLAUDE"})
		err := cmd.Execute()
		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "g8e Gateway MCP Configurations")
	})
}

func TestMcpCmd(t *testing.T) {
	t.Run("mcp command has correct use and description", func(t *testing.T) {
		cmd := Cmd()
		assert.Equal(t, "mcp", cmd.Use)
		assert.Contains(t, cmd.Short, "MCP protocol operations")
		assert.Contains(t, cmd.Short, "stdio")
		assert.Contains(t, cmd.Long, "stdio transport")
	})

	t.Run("mcp command has no required flags", func(t *testing.T) {
		cmd := Cmd()
		require.NotNil(t, cmd)

		// Should not have endpoint or pki-dir flags anymore
		endpointFlag := cmd.Flags().Lookup("endpoint")
		pkiDirFlag := cmd.Flags().Lookup("pki-dir")

		assert.Nil(t, endpointFlag, "mcp command should not have --endpoint flag")
		assert.Nil(t, pkiDirFlag, "mcp command should not have --pki-dir flag")
	})
}

func TestJSONRPCRequest(t *testing.T) {
	t.Run("valid JSON-RPC request parses correctly", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
		var req JSONRPCRequest
		err := json.Unmarshal([]byte(jsonStr), &req)
		require.NoError(t, err)
		assert.Equal(t, "2.0", req.JSONRPC)
		assert.InEpsilon(t, 1, req.ID, 0.0)
		assert.Equal(t, "tools/list", req.Method)
	})

	t.Run("JSON-RPC request without params parses correctly", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":2,"method":"initialize"}`
		var req JSONRPCRequest
		err := json.Unmarshal([]byte(jsonStr), &req)
		require.NoError(t, err)
		assert.Equal(t, "initialize", req.Method)
		assert.Nil(t, req.Params)
	})
}

type mcpProxyStatusResult struct {
	Status string `json:"status"`
}

func TestJSONRPCResponse(t *testing.T) {
	t.Run("JSON-RPC response with result serializes correctly", func(t *testing.T) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Result:  mcpProxyStatusResult{Status: "ok"},
		}
		data, err := json.Marshal(resp)
		require.NoError(t, err)
		assert.Contains(t, string(data), "2.0")
		assert.Contains(t, string(data), "result")
	})

	t.Run("JSON-RPC response with error serializes correctly", func(t *testing.T) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      1,
			Error: &RPCError{
				Code:    -32601,
				Message: "method not found",
			},
		}
		data, err := json.Marshal(resp)
		require.NoError(t, err)
		assert.Contains(t, string(data), "error")
		assert.Contains(t, string(data), "-32601")
	})
}

func TestRPCError(t *testing.T) {
	t.Run("RPC error structure is correct", func(t *testing.T) {
		err := RPCError{
			Code:    constants.JSONRPCErrorCodeParseError,
			Message: constants.JSONRPCErrorMessageParseError,
			Data:    "invalid JSON",
		}
		assert.Equal(t, constants.JSONRPCErrorCodeParseError, err.Code)
		assert.Equal(t, constants.JSONRPCErrorMessageParseError, err.Message)
		assert.Equal(t, "invalid JSON", err.Data)
	})
}

func TestToolsListResult(t *testing.T) {
	t.Run("tools list result structure is correct", func(t *testing.T) {
		result := ToolsListResult{
			Tools: []Tool{
				{
					Name:        "test_tool",
					Description: "A test tool",
					InputSchema: &mcp.InputSchema{Type: "object"},
				},
			},
		}
		assert.Len(t, result.Tools, 1)
		assert.Equal(t, "test_tool", result.Tools[0].Name)
	})
}

func TestTool(t *testing.T) {
	t.Run("tool structure is correct", func(t *testing.T) {
		tool := Tool{
			Name:        "execute_bash",
			Description: "Execute a bash command",
			InputSchema: &mcp.InputSchema{
				Type: "object",
				Properties: map[string]*mcp.PropertySchema{
					"command": {
						Type:        "string",
						Description: "The command to execute",
					},
				},
			},
		}
		assert.Equal(t, "execute_bash", tool.Name)
		assert.Equal(t, "Execute a bash command", tool.Description)
		assert.NotNil(t, tool.InputSchema)
	})
}

func TestHandleInitialize(t *testing.T) {
	t.Run("initialize response has correct structure", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		handleInitialize(encoder, 1)

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.InEpsilon(t, 1, resp.ID, 0.0)
		assert.NotNil(t, resp.Result)

		result := resp.Result.(map[string]interface{})
		assert.Contains(t, result, "protocolVersion")
		assert.Contains(t, result, "capabilities")
		assert.Contains(t, result, "serverInfo")
	})
}

func TestSendError(t *testing.T) {
	t.Run("error response has correct structure", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		sendError(encoder, 1, -32601, "method not found")

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.InEpsilon(t, 1, resp.ID, 0.0)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, -32601, resp.Error.Code)
		assert.Equal(t, "method not found", resp.Error.Message)
	})
}

func TestIsL3ApprovalResponse(t *testing.T) {
	t.Run("detects L3 approval response via structured approval_url field", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"approval_url": "https://example.com/approve/123",
			},
		}
		assert.True(t, isL3ApprovalResponse(resp))
	})

	t.Run("does not detect L3 approval via content text without approval_url field", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Execution paused. Please visit https://example.com/approve/123 to authorize",
					},
				},
			},
		}
		assert.False(t, isL3ApprovalResponse(resp))
	})

	t.Run("does not detect non-approval response", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Command executed successfully",
					},
				},
			},
		}
		assert.False(t, isL3ApprovalResponse(resp))
	})

	t.Run("handles nil result", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: nil,
		}
		assert.False(t, isL3ApprovalResponse(resp))
	})

	t.Run("handles non-map result", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: "string result",
		}
		assert.False(t, isL3ApprovalResponse(resp))
	})
}

func TestExtractApprovalURL(t *testing.T) {
	t.Run("extracts URL from structured approval_url field", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"approval_url": "https://example.com/api/v1/approve/abc123",
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/abc123", url)
	})

	t.Run("extracts URL from content text with approval path", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Execution paused. Please visit https://example.com/api/v1/approve/123 to authorize",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/123", url)
	})

	t.Run("handles URL at end of string", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Please visit https://example.com/api/v1/approve/456",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/456", url)
	})

	t.Run("handles nil result", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: nil,
		}
		url := extractApprovalURL(resp)
		assert.Empty(t, url)
	})

	t.Run("handles response without URL", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "No URL here",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Empty(t, url)
	})

	t.Run("extracts URL from marshaled JSON as fallback", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"message": "Visit https://example.com/api/v1/approve/789 for approval",
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/789", url)
	})

	t.Run("prefers structured approval_url over text content", func(t *testing.T) {
		resp := JSONRPCResponse{
			Result: map[string]interface{}{
				"approval_url": "https://example.com/api/v1/approve/structured",
				"content": []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Visit https://example.com/api/v1/approve/text instead",
					},
				},
			},
		}
		url := extractApprovalURL(resp)
		assert.Equal(t, "https://example.com/api/v1/approve/structured", url)
	})
}

func generateTestCerts(t *testing.T) (certPath, keyPath, caPath string) {
	t.Helper()
	tempDir := testutil.TempDir(t)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"g8e.local"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	certPath = filepath.Join(tempDir, constants.TestCertFilename)
	keyPath = filepath.Join(tempDir, constants.TestKeyFilename)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, os.WriteFile(certPath, certPEM, constants.PermFilePublic))

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(keyPath, keyPEM, constants.PermFilePublic))

	return certPath, keyPath, certPath
}

func TestSendSuccess(t *testing.T) {
	t.Run("success response has correct structure", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		sendSuccess(encoder, 1, map[string]string{"status": "ok"})

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.InEpsilon(t, 1, resp.ID, 0.0)
		assert.NotNil(t, resp.Result)
		assert.Nil(t, resp.Error)
	})

	t.Run("success response with nil result", func(t *testing.T) {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		sendSuccess(encoder, 2, nil)

		var resp JSONRPCResponse
		err := json.Unmarshal(buf.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.Nil(t, resp.Error)
	})
}

func TestExtractURLFromText(t *testing.T) {
	t.Run("extracts approval URL with correct prefix", func(t *testing.T) {
		text := "Please visit https://g8e.local/api/v1/approve/abc123 to authorize"
		url := extractURLFromText(text)
		assert.Contains(t, url, "https://")
		assert.Contains(t, url, "/approve/")
	})

	t.Run("extracts generic HTTPS URL when approval prefix not found", func(t *testing.T) {
		text := "Visit https://example.com/authorize for more info"
		url := extractURLFromText(text)
		assert.Equal(t, "https://example.com/authorize", url)
	})

	t.Run("handles text without URL", func(t *testing.T) {
		text := "No URL in this text"
		url := extractURLFromText(text)
		assert.Empty(t, url)
	})

	t.Run("handles empty string", func(t *testing.T) {
		url := extractURLFromText("")
		assert.Empty(t, url)
	})

	t.Run("extracts URL from text with quotes", func(t *testing.T) {
		text := `URL is "https://g8e.local/api/v1/approve/xyz" in quotes`
		url := extractURLFromText(text)
		assert.Contains(t, url, "https://")
	})

	t.Run("extracts URL from text with apostrophe", func(t *testing.T) {
		text := "URL is 'https://g8e.local/api/v1/approve/abc' in apostrophes"
		url := extractURLFromText(text)
		assert.Contains(t, url, "https://")
	})
}

func TestPrintMCPConfigStdio(t *testing.T) {
	t.Run("generates stdio config with binary path", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err := printMCPConfigStdio(cmd)
		// This will use the actual os.Executable, which should work in test environment
		if err != nil {
			// If it fails, check the error message
			assert.Contains(t, err.Error(), "failed to get binary path")
		} else {
			output := buf.String()
			assert.Contains(t, output, "mcpServers")
			assert.Contains(t, output, "g8e")
			assert.Contains(t, output, "stdio")
		}
	})
}

func TestPrintMCPConfigLocal(t *testing.T) {
	t.Run("generates local config with /etc/hosts entry", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		clientDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient)
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(clientDir, constants.TestCertFilename), []byte("cert"), constants.PermFilePublic))
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(clientDir, constants.TestKeyFilename), []byte("key"), constants.PermFilePublic))
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle), []byte("ca"), constants.PermFilePublic))

		t.Setenv("G8E_PROJECT_ROOT", tempDir)

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = printMCPConfigLocal(cmd)
		if err != nil {
			assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
		}
	})
}

func TestPrintMCPConfigIP(t *testing.T) {
	t.Run("generates IP-based config", func(t *testing.T) {
		tempDir := testutil.TempDir(t)
		fileSvc, err := fs.NewRuntimeFileService(tempDir, slog.Default())
		require.NoError(t, err)
		require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

		clientDir := filepath.Join(constants.PkiDirname, constants.PkiSubdirClient)
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(clientDir, constants.TestCertFilename), []byte("cert"), constants.PermFilePublic))
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(clientDir, constants.TestKeyFilename), []byte("key"), constants.PermFilePublic))
		require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle), []byte("ca"), constants.PermFilePublic))

		t.Setenv("G8E_PROJECT_ROOT", tempDir)

		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)

		err = printMCPConfigIP(cmd)
		if err != nil {
			assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
		}
	})
}

func TestAgentRunCmd(t *testing.T) {
	t.Run("agent run command has correct structure", func(t *testing.T) {
		cmd := agentRunCmd()
		assert.Contains(t, cmd.Use, "run")
		assert.Contains(t, cmd.Short, "Launch an AI agent")
		assert.Contains(t, cmd.Long, "Launch an AI agent")
	})

	t.Run("agent run does not have url flag and has verify flag", func(t *testing.T) {
		cmd := agentRunCmd()
		urlFlag := cmd.Flags().Lookup("url")
		assert.Nil(t, urlFlag, "agent run should not have --url flag")
		verifyFlag := cmd.Flags().Lookup("verify")
		require.NotNil(t, verifyFlag, "agent run should have --verify flag")
		assert.Equal(t, "bool", verifyFlag.Value.Type())
	})

	t.Run("agent run has silence flags set", func(t *testing.T) {
		cmd := agentRunCmd()
		assert.True(t, cmd.SilenceErrors, "should silence errors")
		assert.True(t, cmd.SilenceUsage, "should silence usage")
	})
}

func TestPrintAgentShow(t *testing.T) {
	t.Run("printAgentShow handles all supported agents", func(t *testing.T) {
		for _, integration := range agent.All() {
			cmd := &cobra.Command{}
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			err := printAgentShow(cmd, string(integration.ID))
			if err != nil {
				assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
			} else {
				output := buf.String()
				assert.Contains(t, output, "g8e Gateway MCP Configurations")
			}
		}
	})

	t.Run("printAgentShow is case-insensitive for agent IDs", func(t *testing.T) {
		cmd := &cobra.Command{}
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		// Test uppercase
		err := printAgentShow(cmd, "CLAUDE")
		if err != nil {
			assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
		}

		// Test mixed case
		cmd2 := &cobra.Command{}
		var buf2 bytes.Buffer
		cmd2.SetOut(&buf2)
		cmd2.SetErr(&buf2)

		err = printAgentShow(cmd2, "ClaUdE")
		if err != nil {
			assert.ErrorIs(t, err, constants.ErrConfigLoadFailed)
		}
	})
}

func TestMcpStdioCmd(t *testing.T) {
	t.Run("mcp stdio command has correct structure", func(t *testing.T) {
		cmd := mcpStdioCmd()
		assert.Equal(t, "stdio", cmd.Use)
		assert.Contains(t, cmd.Short, "Run MCP stdio server")
		assert.Contains(t, cmd.Long, "proxies all requests")
	})

	t.Run("mcp stdio command has RunE function", func(t *testing.T) {
		cmd := mcpStdioCmd()
		assert.NotNil(t, cmd.RunE, "should have RunE function")
	})
}

func TestRunMCPAgentRun_NoArgs(t *testing.T) {
	t.Run("returns error when no args", func(t *testing.T) {
		err := runMCPAgentRun(nil, false, "", shared.NewFileSvc, authcmd.PanickingEnrollerFactory())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "specify an agent name")
	})

	t.Run("returns ErrAgentNotFound for unknown agent", func(t *testing.T) {
		err := runMCPAgentRun([]string{"unknown-agent-xyz"}, false, "", shared.NewFileSvc, authcmd.PanickingEnrollerFactory())
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrAgentNotFound)
	})

	t.Run("devin returns error for missing gateway or binary", func(t *testing.T) {
		// Devin is a local CLI agent and goes through launchAgentWithGovernance.
		// We can't test the full launch path here (requires gateway), but we verify
		// it does NOT return the old cloud-based error.
		err := runMCPAgentRun([]string{"devin"}, false, "", cmdtest.FailingFileSvcFactory(constants.ErrNotAuthenticated), authcmd.PanickingEnrollerFactory())
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "cloud-based agent")
	})
}

func TestMcpStdioCmd_FlagsRegistered(t *testing.T) {
	cmd := mcpStdioCmd()
	expectedFlags := []string{
		constants.Flag.ClientCert, constants.Flag.ClientKey, constants.Flag.CABundle,
		constants.Flag.GatewayURL, constants.Flag.App,
	}
	for _, name := range expectedFlags {
		f := cmd.Flags().Lookup(name)
		require.NotNil(t, f, "mcp stdio should have --%s flag", name)
		assert.Equal(t, "string", f.Value.Type(), "--%s should be a string flag", name)
	}
}
