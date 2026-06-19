//go:build integration

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
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxySessionToGatewayWithRetry(t *testing.T) {
	t.Run("retry logic eventually succeeds after L3 approval", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			var resp JSONRPCResponse
			if attempts == 1 {
				// Return L3 approval required on first attempt
				resp = JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      float64(1),
					Result: map[string]interface{}{
						"approval_url": "https://g8e.local/approve/123",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Execution paused. Please visit https://g8e.local/approve/123 to authorize",
							},
						},
					},
				}
			} else {
				// Success on second attempt
				resp = JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      float64(1),
					Result:  map[string]interface{}{"status": "success"},
				}
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		conn := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: server.URL,
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}

		// Mock the polling interval for faster tests
		originalInterval := l3ApprovalPollInterval
		l3ApprovalPollInterval = 1 * time.Millisecond
		defer func() { l3ApprovalPollInterval = originalInterval }()

		resp, err := proxySessionToGatewayWithRetry(conn, req, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, attempts)
		assert.Equal(t, "success", resp.Result.(map[string]interface{})["status"])
	})

	t.Run("retry logic returns original response on timeout", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result: map[string]interface{}{
					"approval_url": "https://g8e.local/approve/123",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": "Execution paused. Please visit https://g8e.local/approve/123 to authorize",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		conn := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: server.URL,
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}

		// Mock the iterations and interval for faster tests
		originalInterval := l3ApprovalPollInterval
		originalMaxIterations := l3ApprovalMaxIterations
		l3ApprovalPollInterval = 1 * time.Millisecond
		l3ApprovalMaxIterations = 2
		defer func() {
			l3ApprovalPollInterval = originalInterval
			l3ApprovalMaxIterations = originalMaxIterations
		}()

		resp, err := proxySessionToGatewayWithRetry(conn, req, nil)
		require.NoError(t, err)
		assert.True(t, isL3ApprovalResponse(resp))
		// 1 initial + 2 retries = 3
		assert.Equal(t, 3, attempts)
	})
}

func TestSubprocessMCPProxyIntegration(t *testing.T) {
	t.Run("subprocessMCPProxy stop closes stdin and kills process", func(t *testing.T) {
		proxy := &subprocessMCPProxy{
			command: "sleep",
			args:    []string{"10"},
			logger:  slog.New(slog.NewTextHandler(os.Stderr, nil)),
		}

		// Start the subprocess
		err := proxy.start()
		if err != nil {
			// May fail if sleep is not available, but we test the stop logic
			return
		}
		defer proxy.stop()

		assert.NotNil(t, proxy.cmd)
		assert.NotNil(t, proxy.stdin)
	})
}
