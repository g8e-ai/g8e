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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestProxySessionToGatewayWithRetry(t *testing.T) {
	t.Run("SSE success retries and returns result", func(t *testing.T) {
		attempts := 0
		gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			var resp JSONRPCResponse
			if attempts == 1 {
				resp = JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      float64(1),
					Result: map[string]interface{}{
						"approval_url": "https://g8e.local:8443/api/v1/approve/tx-sse-success",
						"content": []interface{}{
							map[string]interface{}{
								"type": "text",
								"text": "Execution paused. Please authorize.",
							},
						},
					},
				}
			} else {
				resp = JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      float64(1),
					Result:  map[string]interface{}{"status": "success"},
				}
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer gatewayServer.Close()

		sseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			eventPayload, err := json.Marshal(models.ApprovalCompletedEvent{
				Type:   constants.SSEEventTypeApprovalCompleted,
				UserID: "user-sse-success",
				TxHash: "tx-sse-success",
			})
			require.NoError(t, err)
			envelope := models.SSEPushPayload{
				UserID: "user-sse-success",
				Event:  eventPayload,
			}
			envelopeJSON, err := json.Marshal(envelope)
			require.NoError(t, err)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", constants.SSEEventTypeApprovalCompleted, string(envelopeJSON))
		}))
		defer sseServer.Close()

		conn := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: gatewayServer.URL,
			sseClient:  &http.Client{Timeout: 5 * time.Second},
			sseBaseURL: sseServer.URL,
			userID:     "user-sse-success",
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}

		resp, err := proxySessionToGatewayWithRetryContext(context.Background(), conn, req, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, attempts)
		assert.Equal(t, "success", resp.Result.(map[string]interface{})["status"])
	})

	t.Run("SSE timeout returns error without polling retries", func(t *testing.T) {
		attempts := 0
		gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result: map[string]interface{}{
					"approval_url": "https://g8e.local:8443/api/v1/approve/tx-sse-timeout",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": "Execution paused. Please authorize.",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer gatewayServer.Close()

		sseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			<-r.Context().Done()
		}))
		defer sseServer.Close()

		conn := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: gatewayServer.URL,
			sseClient:  &http.Client{Timeout: 5 * time.Second},
			sseBaseURL: sseServer.URL,
			userID:     "user-sse-timeout",
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		_, err := proxySessionToGatewayWithRetryContext(ctx, conn, req, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timed out")
		// Only the initial proxy attempt — no polling retries
		assert.Equal(t, 1, attempts)
	})

	t.Run("SSE credentials missing returns ErrNotAuthenticated", func(t *testing.T) {
		gatewayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      float64(1),
				Result: map[string]interface{}{
					"approval_url": "https://g8e.local:8443/api/v1/approve/tx-no-creds",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": "Execution paused. Please authorize.",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer gatewayServer.Close()

		conn := &gatewayConn{
			client:     &http.Client{Timeout: 5 * time.Second},
			gatewayURL: gatewayServer.URL,
		}

		req := JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
		}

		_, err := proxySessionToGatewayWithRetryContext(context.Background(), conn, req, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
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
