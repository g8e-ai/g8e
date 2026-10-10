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
	"log/slog"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore"
	"github.com/g8e-ai/g8e/v2/internal/services/keystore/keystoretest"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	pubsubtest "github.com/g8e-ai/g8e/v2/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enrolledTestConfig is a config carrying the in-memory identity StartOperator
// applies from the bootstrap bundle before the service starts.
func enrolledTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	cfg.OperatorID = "test-op-1"
	cfg.OperatorSessionId = "test-sess-1"
	return cfg
}

func TestG8eoService_Start_SuccessFlow(t *testing.T) {
	cfg := enrolledTestConfig(t)
	cfg.PubSubURL = "wss://127.0.0.1:0" // dummy
	cfg.NoGit = true

	// Initialize paths with test directory
	require.NoError(t, paths.InitWithBase(cfg.WorkDir))

	fileSvc, err := fs.NewRuntimeFileService(cfg.WorkDir, testutil.NewVerboseTestLogger(t))
	require.NoError(t, err)
	require.NoError(t, fileSvc.CreateRuntimeTree(context.Background()))

	// Initialize keystore with in-memory keyring for the master key (required for gateway database)
	testBackend := keystoretest.NewMemoryKeyring()
	ks, err := keystore.NewWithKeyringAndFS(testutil.NewVerboseTestLogger(t), testBackend, fileSvc)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnforcePermissions())

	tlsConfig := newTestTLSConfig(t)
	factoryCalls := 0
	factory := func(baseURL, serverName string, logger *slog.Logger, receivedTLSConfig *certs.TLSConfig) (pubsub.PubSubClient, error) {
		factoryCalls++
		assert.Equal(t, cfg.PubSubURL, baseURL)
		assert.Equal(t, cfg.TLSServerName, serverName)
		assert.NotNil(t, logger)
		assert.Same(t, tlsConfig, receivedTLSConfig)
		return pubsubtest.NewMockOperatorPubSubClient(), nil
	}
	service, err := NewG8eoService(cfg, testutil.NewVerboseTestLogger(t), tlsConfig, fileSvc, factory)
	require.NoError(t, err)

	// Inject test keystore (bypasses OS keychain for cross-platform CI)
	service.mu.Lock()
	service.keystore = ks
	service.mu.Unlock()

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
	assert.Equal(t, 1, factoryCalls)

	service.pubSubCommands.ShutdownChan <- "test shutdown request"
	select {
	case <-service.Done():
	case <-ctx.Done():
		t.Fatal("Timed out waiting for the Operator service to observe shutdown")
	}
	assert.ErrorIs(t, service.ctx.Err(), context.Canceled)

	// Clean up to avoid background goroutines logging after test completion
	require.NoError(t, service.Stop(context.Background()))
}

func TestG8eoService_Start_FactoryFailureStopsBeforeDependents(t *testing.T) {
	cfg := enrolledTestConfig(t)

	factoryErr := fmt.Errorf("pub/sub client factory unavailable")
	factoryCalls := 0
	service, err := NewG8eoService(cfg, testutil.NewVerboseTestLogger(t), newTestTLSConfig(t), newTestFileSvc(t), func(string, string, *slog.Logger, *certs.TLSConfig) (pubsub.PubSubClient, error) {
		factoryCalls++
		return nil, factoryErr
	})
	require.NoError(t, err)

	err = service.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, factoryErr)
	assert.Equal(t, 1, factoryCalls)
	assert.Nil(t, service.pubSubClient)
	assert.Nil(t, service.pubSubResults)
	assert.Nil(t, service.pubSubCommands)
	assert.Nil(t, service.execution)
	service.cancel()
}
