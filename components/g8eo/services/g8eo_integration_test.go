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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/pubsub"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestG8eoService_Start_BootstrapFailure(t *testing.T) {

	cfg := testutil.NewTestConfig(t)
	cfg.APIKey = "invalid-api-key-for-testing"
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger)
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

		cmdSvc, err := pubsub.NewPubSubCommandService(pubsub.CommandServiceConfig{
			Config:    cfg,
			Logger:    logger,
			Execution: execSvc,
			FileEdit:  fileEditSvc,
		})
		require.NoError(t, err)
		require.NotNil(t, cmdSvc)
		t.Cleanup(func() { cmdSvc.Stop() })
	})

	t.Run("pub/sub results service", func(t *testing.T) {

		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()

		client, err := pubsub.NewG8esPubSubClient(testutil.GetTestG8esDirectURL(), "", logger)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		resultsSvc, err := pubsub.NewPubSubResultsService(cfg, logger, client, nil)
		require.NoError(t, err)
		assert.NotNil(t, resultsSvc)
	})
}
