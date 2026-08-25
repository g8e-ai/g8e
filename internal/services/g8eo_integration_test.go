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
	"crypto/tls"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/services/execution"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/pubsub"
	pubsubtest "github.com/g8e-ai/g8e/v2/internal/services/pubsub/pubsubtest"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestG8eoService_Start_BootstrapFailure(t *testing.T) {

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	trustStore := testutil.GetTestTrustStore()
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})
	tlsCfg := certs.NewTLSConfig(trustStore, clientIdentity)

	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), logger)
	require.NoError(t, err)

	service, err := NewG8eoService(cfg, logger, tlsCfg, fileSvc)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = service.Start(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to authenticate")

	service.mu.RLock()
	running := service.running
	service.mu.RUnlock()
	assert.False(t, running)
}

func TestG8eoService_SubServices_Initialization(t *testing.T) {
	t.Run("execution service", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc := execution.NewExecutionService(cfg, logger)
		assert.NotNil(t, svc)
	})

	t.Run("file edit service", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		svc := execution.NewFileEditService(cfg, logger)
		assert.NotNil(t, svc)
	})

	t.Run("pub/sub command service", func(t *testing.T) {

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		cmdSvc, err := pubsub.NewOperatorPubSubService(pubsub.CommandServiceConfig{
			Config:       cfg,
			Logger:       logger,
			Execution:    execSvc,
			FileEdit:     fileEditSvc,
			PubSubClient: pubsubtest.NewMockOperatorPubSubClient(),
		}, pubsub.GovernanceDeps{
			ReplayStore:       &testutil.MockReplayStore{},
			StateRootProvider: testutil.NewMockStateRootProvider("test-state-root"),
			TransactionAudit:  &testutil.MockTransactionAudit{},
			L3Notary:          &testutil.MockL3Notary{},
		})
		require.NoError(t, err)
		require.NotNil(t, cmdSvc)
		t.Cleanup(func() { cmdSvc.Stop() })
	})

	t.Run("pub/sub results service", func(t *testing.T) {

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		subTrustStore := testutil.GetTestTrustStore()
		subClientIdentity := certs.NewClientIdentity(tls.Certificate{})
		subTLSCfg := certs.NewTLSConfig(subTrustStore, subClientIdentity)

		client, err := pubsub.NewOperatorPubSubClient(testutil.GetTestOperatorDirectURL(), "", logger, subTLSCfg)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		resultsSvc, err := pubsub.NewPubSubResultsService(cfg, logger, client)
		require.NoError(t, err)
		assert.NotNil(t, resultsSvc)
	})
}
