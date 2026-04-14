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

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/services/auth"
	"github.com/g8e-ai/g8e/components/g8eo/services/pubsub"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestG8eoService_Start_SuccessFlow(t *testing.T) {
	// 1. Setup mock g8ed server for bootstrap
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/operator" {
			resp := auth.AuthServicesResponse{
				Success:           true,
				OperatorID:        "test-op-1",
				OperatorSessionId: "test-sess-1",
				Config: &auth.BootstrapConfig{
					MaxConcurrentTasks: 10,
					MaxMemoryMB:        1024,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, testutil_MarshalJSON(t, resp))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// 2. Configure g8eo to point to the mock server
	cfg := testutil.NewTestConfig(t)
	// Extract host and port from httptest server URL
	u := server.URL[8:] // strip https://
	cfg.Endpoint = "127.0.0.1"
	fmt.Sscanf(u, "127.0.0.1:%d", &cfg.HTTPPort)
	cfg.PubSubURL = "ws://127.0.0.1:0" // dummy
	cfg.NoGit = true

	service, err := NewG8eoService(cfg, testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)

	// 3. Inject mocks
	require.NotNil(t, service.bootstrap)
	service.bootstrap.SetHTTPClient(server.Client())

	// Inject Mock PubSub Client
	mockPubSub := pubsub.NewMockG8esPubSubClient()
	service.SetPubSubClient(mockPubSub)

	// 4. Start the service
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- service.Start(ctx)
	}()

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("Timed out waiting for G8eoService to start")
	}

	assert.True(t, service.running)

	// Clean up to avoid background goroutines logging after test completion
	service.Stop(context.Background())
}

func testutil_MarshalJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
