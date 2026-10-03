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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	latticeconfig "github.com/g8e-ai/g8e/v2/internal/adapters/lattice/config"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// newStartableG8eoService builds a G8eoService wired to a real bootstrap
// endpoint, runtime tree, and in-memory keyring so Start exercises the real
// storage, governance, and pub/sub construction. mutate adjusts the config
// before the service is built. Whatever Start managed to open is released on
// cleanup, including when Start fails part-way.
func newStartableG8eoService(t *testing.T, mutate func(cfg *config.Config)) *G8eoService {
	t.Helper()
	server := newG8eoBootstrapTestServer(t)
	t.Cleanup(server.Close)

	cfg := testutil.NewTestConfig(t)
	cfg.Endpoint = "127.0.0.1"
	_, err := fmt.Sscanf(server.URL[len("https://"):], "127.0.0.1:%d", &cfg.HTTPSPort)
	require.NoError(t, err)
	cfg.PubSubURL = "wss://127.0.0.1:0"
	cfg.NoGit = true
	if mutate != nil {
		mutate(cfg)
	}

	require.NoError(t, paths.InitWithBase(cfg.WorkDir))
	fileSvc, err := fs.NewRuntimeFileService(cfg.WorkDir, testutil.NewTestLogger())
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	ks, err := keystore.NewWithKeyringAndFS(testutil.NewTestLogger(), keystoretest.NewMemoryKeyring(), fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())

	service, err := NewG8eoService(cfg, testutil.NewTestLogger(), newTestTLSConfig(t), fileSvc, newTestPubSubClientFactory())
	require.NoError(t, err)
	service.bootstrap.SetHTTPClient(server.Client())
	service.keystore = ks

	t.Cleanup(func() {
		// Start can fail after opening stores; force the service into the
		// running state so Stop releases everything that was opened.
		service.mu.Lock()
		service.running = true
		service.mu.Unlock()
		_ = service.Stop(context.Background())
	})
	return service
}

func startWithTimeout(t *testing.T, service *G8eoService) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return service.Start(ctx)
}

func fakeOllamaWithModels(t *testing.T, tagsJSON string, status int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tagsJSON))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestG8eoService_Start_NoGitLeavesLedgerDisabledButKeepsHistoryHandler(t *testing.T) {
	service := newStartableG8eoService(t, nil)

	require.NoError(t, startWithTimeout(t, service))

	assert.True(t, service.running)
	assert.Nil(t, service.ledger, "--no-git must not initialize the git-backed ledger")
	assert.NotNil(t, service.historyHandler, "history stays available without file versioning")
	assert.Empty(t, service.config.GitPath)
	assert.False(t, service.config.GitAvailable)
	assert.NotNil(t, service.replayStore, "replay protection is mandatory in every mode")
	assert.NotNil(t, service.suspendedTxStore)
	assert.NotNil(t, service.executionVault)
}

func TestG8eoService_Start_InitializesGitLedgerWhenGitIsAllowed(t *testing.T) {
	service := newStartableG8eoService(t, func(cfg *config.Config) { cfg.NoGit = false })

	require.NoError(t, startWithTimeout(t, service))

	assert.NotNil(t, service.ledger, "the git-backed ledger must be initialized when git is allowed")
	assert.NotNil(t, service.historyHandler)
	assert.NotEmpty(t, service.config.GitPath)
	assert.True(t, service.config.GitAvailable)
}

func TestG8eoService_Start_ExternalShutdownRequestCancelsServiceContext(t *testing.T) {
	service := newStartableG8eoService(t, nil)
	require.NoError(t, startWithTimeout(t, service))
	require.NoError(t, service.ctx.Err(), "the service context is live after start")

	service.pubSubCommands.ShutdownChan <- "remote shutdown requested"

	select {
	case <-service.ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("an external shutdown request must cancel the service context")
	}
}

func TestG8eoService_Start_SecondStartIsRejectedAndStopIsIdempotent(t *testing.T) {
	service := newStartableG8eoService(t, nil)
	require.NoError(t, startWithTimeout(t, service))

	err := startWithTimeout(t, service)

	require.ErrorIs(t, err, constants.ErrServiceUnavailable)
	require.NoError(t, service.Stop(context.Background()))
	assert.False(t, service.running)
	require.NoError(t, service.Stop(context.Background()), "stopping an already-stopped service is a no-op")
}

func TestG8eoService_Start_FailsClosedWithoutExecutionVault(t *testing.T) {
	service := newStartableG8eoService(t, func(cfg *config.Config) { cfg.ExecutionVaultEnabled = false })

	err := startWithTimeout(t, service)

	require.ErrorIs(t, err, constants.ErrInternal)
	assert.Contains(t, err.Error(), "execution vault must be enabled")
	assert.False(t, service.running)
	assert.Nil(t, service.pubSubCommands, "no command service may be built without replay protection storage")
}

func TestG8eoService_Start_InferenceReadinessGate(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  func(t *testing.T) string
		wantErr   error
		wantReady bool
	}{
		{
			name: "provider reachable",
			endpoint: func(t *testing.T) string {
				return fakeOllamaWithModels(t, `{"models":[{"name":"qwen3:4b","digest":"sha256:aa"}]}`, http.StatusOK)
			},
			wantReady: true,
		},
		{
			name: "provider reachable with no models installed",
			endpoint: func(t *testing.T) string {
				return fakeOllamaWithModels(t, `{"models":[]}`, http.StatusOK)
			},
			wantReady: true,
		},
		{
			name:     "provider unavailable",
			endpoint: func(t *testing.T) string { return fakeOllamaWithModels(t, "", http.StatusServiceUnavailable) },
			wantErr:  constants.ErrInferenceBackendUnavailable,
		},
		{
			name:     "provider endpoint is invalid",
			endpoint: func(*testing.T) string { return "not-a-url" },
			wantErr:  constants.ErrInferenceEndpointInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := tt.endpoint(t)
			service := newStartableG8eoService(t, func(cfg *config.Config) {
				cfg.Inference = config.InferenceConfig{Enabled: true, OllamaEndpoint: endpoint}
			})

			err := startWithTimeout(t, service)

			if tt.wantReady {
				require.NoError(t, err)
				assert.True(t, service.running)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			assert.Contains(t, err.Error(), "g8eo:", "startup must fail closed with a g8eo-attributed error")
			assert.False(t, service.running)
			assert.Nil(t, service.pubSubCommands, "no command service may start behind an unready inference provider")
		})
	}
}

// An Inference Operator whose provider is not ready must fail before bootstrap
// claims a session. The bootstrap port is closed here, so a claim attempt would
// surface ErrNotAuthenticated (after its retry backoff outlasts the start
// timeout) instead of the provider error.
func TestG8eoService_Start_InferenceProviderFailureDoesNotClaimSession(t *testing.T) {
	endpoint := fakeOllamaWithModels(t, "", http.StatusServiceUnavailable)
	service := newStartableG8eoService(t, func(cfg *config.Config) {
		cfg.HTTPSPort = 1
		cfg.Inference = config.InferenceConfig{Enabled: true, OllamaEndpoint: endpoint}
	})

	sessionBefore := service.config.OperatorSessionId

	err := startWithTimeout(t, service)

	require.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
	assert.NotErrorIs(t, err, constants.ErrNotAuthenticated)
	assert.Equal(t, sessionBefore, service.config.OperatorSessionId, "no Operator session may be claimed behind an unready inference provider")
	assert.False(t, service.running)
}

func TestG8eoService_Start_EnablesProviderBoundaryObserverAndProvenanceOperator(t *testing.T) {
	storageRoot := testutil.TempDir(t)
	service := newStartableG8eoService(t, func(cfg *config.Config) {
		cfg.ProviderBoundaryObserver = config.ProviderBoundaryObserverConfig{Enabled: true, ObserverID: "observer-under-test"}
		cfg.ProvenanceOperator = config.ProvenanceOperatorConfig{Enabled: true, OperatorID: "provenance-under-test", ModelStorageRoot: storageRoot}
	})

	require.NoError(t, startWithTimeout(t, service))

	assert.True(t, service.running)
	assert.NotNil(t, service.pubSubCommands)
}

func TestG8eoService_Start_ProvenanceOperatorRequiresModelStorageRoot(t *testing.T) {
	service := newStartableG8eoService(t, func(cfg *config.Config) {
		cfg.ProvenanceOperator = config.ProvenanceOperatorConfig{Enabled: true, OperatorID: "provenance-under-test"}
	})

	err := startWithTimeout(t, service)

	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
	assert.Contains(t, err.Error(), "model provenance attestor")
	assert.False(t, service.running)
}

func TestG8eoService_Start_LatticeConfigurationIsValidatedBeforeDialing(t *testing.T) {
	validLattice := func() *latticeconfig.LatticeConfig {
		return &latticeconfig.LatticeConfig{
			Enabled:      true,
			Endpoint:     "127.0.0.1:1",
			ClientID:     "client",
			ClientSecret: "secret",
			Entity:       latticeconfig.EntityConfig{Name: "entity-under-test"},
		}
	}
	tests := []struct {
		name    string
		mutate  func(cfg *config.Config)
		wantErr error
		wantMsg string
	}{
		{
			name: "missing endpoint",
			mutate: func(cfg *config.Config) {
				l := validLattice()
				l.Endpoint = ""
				cfg.Lattice = l
			},
			wantErr: constants.ErrLatticeEndpointRequired,
			wantMsg: "lattice: config validation",
		},
		{
			name: "missing client secret",
			mutate: func(cfg *config.Config) {
				l := validLattice()
				l.ClientSecret = ""
				cfg.Lattice = l
			},
			wantErr: constants.ErrLatticeClientSecretRequired,
			wantMsg: "lattice: config validation",
		},
		{
			name: "heartbeat interval too long for Lattice entity expiry",
			mutate: func(cfg *config.Config) {
				cfg.Lattice = validLattice()
				cfg.HeartbeatInterval = 10 * time.Minute
			},
			wantErr: constants.ErrLatticeHeartbeatIntervalInvalid,
			wantMsg: "lattice: heartbeat interval",
		},
		{
			name: "heartbeat scheduler disabled",
			mutate: func(cfg *config.Config) {
				cfg.Lattice = validLattice()
				cfg.HeartbeatInterval = 0
			},
			wantErr: constants.ErrLatticeHeartbeatIntervalInvalid,
			wantMsg: "lattice: heartbeat interval",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newStartableG8eoService(t, tt.mutate)

			err := startWithTimeout(t, service)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Contains(t, err.Error(), tt.wantMsg)
			assert.False(t, service.running, "a misconfigured Lattice adapter must not leave the operator running")
			assert.Nil(t, service.latticeAdapter)
		})
	}
}
