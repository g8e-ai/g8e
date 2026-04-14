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

	"github.com/g8e-ai/g8e/components/g8eo/services/auth"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewG8eoService_InitialState(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	before := time.Now().UTC()
	service, err := NewG8eoService(cfg, logger)
	after := time.Now().UTC()

	require.NoError(t, err)
	require.NotNil(t, service)

	assert.Equal(t, cfg, service.config)
	assert.Equal(t, logger, service.logger)
	assert.False(t, service.running)

	assert.True(t, !service.startTime.Before(before), "startTime should be >= before")
	assert.True(t, !service.startTime.After(after), "startTime should be <= after")
	assert.Equal(t, time.UTC, service.startTime.Location())

	assert.NotNil(t, service.bootstrap)
	assert.IsType(t, &auth.BootstrapService{}, service.bootstrap)

	assert.Nil(t, service.execution)
	assert.Nil(t, service.fileEdit)
	assert.Nil(t, service.pubSubCommands)
	assert.Nil(t, service.pubSubResults)
	assert.Nil(t, service.pubSubClient)
	assert.Nil(t, service.localStore)
	assert.Nil(t, service.rawVault)
	assert.Nil(t, service.auditVault)
	assert.Nil(t, service.ledger)
	assert.Nil(t, service.historyHandler)
	assert.Nil(t, service.sentinel)
}

func TestNewG8eoService_PreservesConfig(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.MaxConcurrentTasks = 42
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger)
	require.NoError(t, err)

	assert.Equal(t, 42, service.config.MaxConcurrentTasks)
	assert.Equal(t, cfg.OperatorID, service.config.OperatorID)
	assert.Equal(t, cfg.OperatorSessionId, service.config.OperatorSessionId)
}

func TestNewG8eoService_IndependentInstances(t *testing.T) {
	cfg1 := testutil.NewTestConfig(t)
	cfg2 := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	svc1, err := NewG8eoService(cfg1, logger)
	require.NoError(t, err)

	svc2, err := NewG8eoService(cfg2, logger)
	require.NoError(t, err)

	assert.NotEqual(t, svc1.config.OperatorID, svc2.config.OperatorID)
	assert.NotEqual(t, svc1.config.OperatorSessionId, svc2.config.OperatorSessionId)
}

func TestG8eoService_Start_AlreadyRunning(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger)
	require.NoError(t, err)

	service.mu.Lock()
	service.running = true
	service.mu.Unlock()

	err = service.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestG8eoService_ProductionSentinelConfig(t *testing.T) {
	cfg := productionSentinelConfig()

	assert.True(t, cfg.Enabled, "production sentinel must be enabled")
	assert.True(t, cfg.StrictMode, "production sentinel must use strict mode")
	assert.True(t, cfg.ThreatDetectionEnabled, "production sentinel must have threat detection enabled")
	assert.Equal(t, 4096, cfg.MaxOutputLength, "production sentinel max output length must be 4096")
}

func TestG8eoService_Stop(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger)
	require.NoError(t, err)

	// Manually set running to true to test Stop()
	service.mu.Lock()
	service.running = true
	service.mu.Unlock()

	err = service.Stop(context.Background())
	require.NoError(t, err)

	service.mu.RLock()
	running := service.running
	service.mu.RUnlock()
	assert.False(t, running)
}

func TestG8eoService_ConcurrentStateAccess(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger)
	require.NoError(t, err)

	done := make(chan struct{}, 20)

	for i := 0; i < 10; i++ {
		go func() {
			service.mu.RLock()
			_ = service.running
			_ = service.config
			_ = service.bootstrap
			service.mu.RUnlock()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		go func(val bool) {
			service.mu.Lock()
			service.running = val
			service.mu.Unlock()
			done <- struct{}{}
		}(i%2 == 0)
	}

	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent state access timed out")
		}
	}
}
