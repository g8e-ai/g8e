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
	"crypto/tls"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/services/auth"
	"github.com/g8e-ai/g8e/internal/services/fs"
	"github.com/g8e-ai/g8e/internal/services/storage"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTLSConfig(t *testing.T) *certs.TLSConfig {
	t.Helper()
	trustStore := testutil.GetTestTrustStore()
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})
	return certs.NewTLSConfig(trustStore, clientIdentity)
}

func newTestFileSvc(t *testing.T) fs.RuntimeFileService {
	t.Helper()
	fileSvc, err := fs.NewRuntimeFileService(testutil.TempDir(t), testutil.NewTestLogger())
	require.NoError(t, err)
	return fileSvc
}

func TestNewG8eoService_InitialState(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	before := time.Now().UTC()
	service, err := NewG8eoService(cfg, logger, newTestTLSConfig(t), newTestFileSvc(t))
	after := time.Now().UTC()

	require.NoError(t, err)
	require.NotNil(t, service)

	assert.Equal(t, cfg, service.config)
	assert.Equal(t, logger, service.logger)
	assert.False(t, service.running)

	assert.False(t, service.startTime.Before(before), "startTime should be >= before")
	assert.False(t, service.startTime.After(after), "startTime should be <= after")
	assert.Equal(t, time.UTC, service.startTime.Location())

	assert.NotNil(t, service.bootstrap)
	assert.IsType(t, &auth.BootstrapService{}, service.bootstrap)

	assert.Nil(t, service.execution)
	assert.Nil(t, service.fileEdit)
	assert.Nil(t, service.pubSubCommands)
	assert.Nil(t, service.pubSubResults)
	assert.Nil(t, service.pubSubClient)
	assert.Nil(t, service.executionVault)
	assert.Nil(t, service.tokenStore)
	assert.Nil(t, service.suspendedTxStore)
	assert.Nil(t, service.ledger)
	assert.Nil(t, service.historyHandler)
}

func TestNewG8eoService_PreservesConfig(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	cfg.MaxConcurrentTasks = 42
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)

	service, err := NewG8eoService(cfg, logger, newTestTLSConfig(t), fileSvc)
	require.NoError(t, err)

	assert.Equal(t, 42, service.config.MaxConcurrentTasks)
	assert.Equal(t, cfg.OperatorID, service.config.OperatorID)
	assert.Equal(t, cfg.OperatorSessionId, service.config.OperatorSessionId)
}

func TestNewG8eoService_IndependentInstances(t *testing.T) {
	t.Parallel()
	cfg1 := testutil.NewTestConfig(t)
	cfg2 := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	svc1, err := NewG8eoService(cfg1, logger, newTestTLSConfig(t), newTestFileSvc(t))
	require.NoError(t, err)

	svc2, err := NewG8eoService(cfg2, logger, newTestTLSConfig(t), newTestFileSvc(t))
	require.NoError(t, err)

	assert.NotEqual(t, svc1.config.OperatorID, svc2.config.OperatorID)
	assert.NotEqual(t, svc1.config.OperatorSessionId, svc2.config.OperatorSessionId)
}

func TestG8eoService_Start_AlreadyRunning(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger, newTestTLSConfig(t), newTestFileSvc(t))
	require.NoError(t, err)

	service.mu.Lock()
	service.running = true
	service.mu.Unlock()

	err = service.Start(context.Background())
	require.Error(t, err)
	assert.Error(t, err)
}

func TestG8eoService_Stop(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger, newTestTLSConfig(t), newTestFileSvc(t))
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

func TestG8eoService_SuspendedTxStoreSingleField(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger, newTestTLSConfig(t), newTestFileSvc(t))
	require.NoError(t, err)

	// Create a real SuspendedTransactionService with a temp DB
	suspendedTxConfig := storage.DefaultSuspendedTransactionConfig()
	suspendedTxConfig.DBPath = filepath.Join(testutil.TempDir(t), "suspended_tx.db")
	suspendedTxService, err := storage.NewSuspendedTransactionService(suspendedTxConfig, logger)
	require.NoError(t, err)

	service.suspendedTxStore = suspendedTxService

	t.Run("suspendedTxStore is non-nil", func(t *testing.T) {
		assert.NotNil(t, service.suspendedTxStore)
	})

	t.Run("suspendedTxStore satisfies SuspendedTransactionStore interface", func(t *testing.T) {
		var _ storage.SuspendedTransactionStore = service.suspendedTxStore
	})

	t.Run("Stop closes suspendedTxStore without error", func(t *testing.T) {
		service.mu.Lock()
		service.running = true
		service.mu.Unlock()

		err := service.Stop(context.Background())
		assert.NoError(t, err)
	})

	t.Run("Stop is idempotent after suspendedTxStore closed", func(t *testing.T) {
		service.mu.Lock()
		service.running = true
		service.mu.Unlock()

		err := service.Stop(context.Background())
		assert.NoError(t, err)
	})
}

func TestG8eoService_ConcurrentStateAccess(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	service, err := NewG8eoService(cfg, logger, newTestTLSConfig(t), newTestFileSvc(t))
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
