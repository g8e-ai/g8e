//go:build integration

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package services

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/services/auth"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	pubsubtest "github.com/g8e-ai/g8e/v2/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/v2/internal/services/vault"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestG8eoService_Start_SuccessFlow(t *testing.T) {
	// 1. Setup mock client server for bootstrap
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/operators/reauth" {
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
	fmt.Sscanf(u, "127.0.0.1:%d", &cfg.HTTPSPort)
	cfg.PubSubURL = "wss://127.0.0.1:0" // dummy
	cfg.NoGit = true

	// Initialize paths with test directory
	require.NoError(t, paths.InitWithBase(cfg.WorkDir))

	// Initialize vault for encryption (required since storage refactor)
	vaultDir := paths.Infra.VaultDir
	require.NoError(t, os.MkdirAll(vaultDir, 0700))
	testKey := []byte("g8e_test_abc123xyz789_TEST_KEY_1")
	keyPath := filepath.Join(vaultDir, "key")
	require.NoError(t, os.WriteFile(keyPath, []byte(hex.EncodeToString(testKey)), 0600))
	header, dek, err := vault.NewVaultHeader(testKey)
	require.NoError(t, err)
	vault.SecureZero(dek)
	require.NoError(t, header.Save(vaultDir))

	// Initialize keystore with in-memory keyring for the master key (required for gateway database)
	fileSvc, secretsDir := keystoretest.NewTestFileService(t)
	_ = secretsDir
	testBackend := keystoretest.NewMemoryKeyring()
	ks, err := keystore.NewWithKeyringAndFS(testutil.NewVerboseTestLogger(t), testBackend, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())

	service, err := NewG8eoService(cfg, testutil.NewVerboseTestLogger(t), newTestTLSConfig(t), fileSvc)
	require.NoError(t, err)

	// 3. Inject mocks
	require.NotNil(t, service.bootstrap)
	service.bootstrap.SetHTTPClient(server.Client())

	// Inject test keystore (bypasses OS keychain for cross-platform CI)
	service.mu.Lock()
	service.keystore = ks
	service.mu.Unlock()

	// Inject Mock PubSub Client
	mockPubSub := pubsubtest.NewMockOperatorPubSubClient()
	service.mu.Lock()
	service.pubSubClient = mockPubSub
	service.mu.Unlock()

	// 4. Start the service
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
