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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	clierrors "github.com/g8e-ai/g8e/internal/cli/errors"
	"github.com/g8e-ai/g8e/internal/cli/jsonrpc"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/spf13/cobra"
)

func mcpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Model Context Protocol (MCP) client utilities",
		Long:  `MCP client utilities for IDE integration and Gateway connectivity.`,
	}

	cmd.AddCommand(
		mcpStdioCmd(),
	)

	return cmd
}

func mcpStdioCmd() *cobra.Command {
	var endpoint string

	cmd := &cobra.Command{
		Use:   "stdio",
		Short: "Run as stdio MCP client for IDE integration",
		Long: `Runs g8e as a stdio-based MCP client that proxies JSON-RPC requests
from the IDE (stdin/stdout) to the remote Gateway over mTLS.

This is the recommended way to integrate with IDEs like Cursor, Windsurf, and Claude Code.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if endpoint == "" {
				endpoint = fmt.Sprintf("https://localhost:%d/api/mcp/v1", cfg.OperatorHTTPSPort())
			}

			creds, err := auth.LoadCredentials(cfg)
			if err != nil {
				return fmt.Errorf("%w: %w", clierrors.ErrFailedToLoadCredentials, err)
			}

			if creds == nil {
				return clierrors.ErrNotAuthenticated
			}

			cert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
			if err != nil {
				return fmt.Errorf("%w: %w", clierrors.ErrFailedToLoadClientCertificate, err)
			}

			trustBundle, err := os.ReadFile(cfg.TrustBundlePath())
			if err != nil {
				return fmt.Errorf("%w: %w", clierrors.ErrFailedToReadTrustBundle, err)
			}

			caPool := x509.NewCertPool()
			if !caPool.AppendCertsFromPEM(trustBundle) {
				return clierrors.ErrFailedToParseTrustBundle
			}

			tlsConfig := &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      caPool,
				MinVersion:   tls.VersionTLS13,
			}

			httpClient := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: tlsConfig,
				},
				Timeout: 30 * time.Second,
			}

			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			return runMCPStdioLoop(cmd.Context(), httpClient, endpoint, creds, logger)
		},
	}

	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Gateway endpoint URL (default: https://localhost:<port>/api/mcp/v1)")

	return cmd
}

func runMCPStdioLoop(ctx context.Context, httpClient *http.Client, endpoint string, creds *auth.Credentials, logger *slog.Logger) error {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req jsonrpc.Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			logger.Error("Failed to parse JSON-RPC request", "error", err)
			sendErrorResponse(nil, jsonrpc.NewParseErrorResponse(nil, err))
			continue
		}

		if err := req.Validate(); err != nil {
			logger.Error("Invalid JSON-RPC request", "error", err, "method", req.Method)
			sendErrorResponse(req.ID, jsonrpc.NewInvalidRequestResponse(req.ID, err.Error()))
			continue
		}

		targetPath, err := mapMethodToPath(req.Method)
		if err != nil {
			logger.Error("Unsupported MCP method", "method", req.Method)
			sendErrorResponse(req.ID, jsonrpc.NewMethodNotFoundResponse(req.ID, req.Method))
			continue
		}

		if err := forwardRequest(ctx, httpClient, endpoint+targetPath, &req, creds, logger); err != nil {
			logger.Error("Failed to forward request", "method", req.Method, "error", err)
			sendErrorResponse(req.ID, jsonrpc.NewInternalErrorResponse(req.ID, err))
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin error: %w", err)
	}

	return nil
}

func mapMethodToPath(method string) (string, error) {
	switch method {
	case "tools/list":
		return "/tools/list", nil
	case "tools/call":
		return "/tools/call", nil
	case "resources/list":
		return "/resources/list", nil
	case "resources/read":
		return "/resources/read", nil
	case "prompts/list":
		return "/prompts/list", nil
	case "prompts/get":
		return "/prompts/get", nil
	default:
		return "", fmt.Errorf("unsupported method: %s", method)
	}
}

func forwardRequest(ctx context.Context, httpClient *http.Client, url string, req *jsonrpc.Request, creds *auth.Credentials, logger *slog.Logger) error {
	var body io.Reader
	if req.Method == "tools/call" || req.Method == "resources/read" || req.Method == "prompts/get" {
		reqBytes, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		body = bytes.NewBuffer(reqBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(constants.HeaderOperatorSessionID, creds.OperatorSessionID)
	httpReq.Header.Set(constants.HeaderCLISessionID, creds.CLISessionID)
	httpReq.Header.Set(constants.HeaderUserID, creds.UserID)
	httpReq.Header.Set(constants.HeaderOperatorID, creds.OperatorID)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Warn("Gateway returned non-200 status", "status", resp.StatusCode, "body", string(respBody))
	}

	fmt.Println(string(respBody))
	return nil
}

func sendErrorResponse(id interface{}, resp *jsonrpc.Response) {
	respBytes, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal error response: %v\n", err)
		return
	}
	fmt.Println(string(respBytes))
}
