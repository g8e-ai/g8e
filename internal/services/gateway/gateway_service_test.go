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

package gateway

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
)

func TestNewGatewayModeService(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Ensure directories are set for tests to avoid SQLITE_CANTOPEN
	cfg.Gateway.DataDir = tempDir(t)
	cfg.Gateway.PKIDir = tempDir(t)
	cfg.Gateway.SecretsDir = tempDir(t)

	t.Run("Default configuration with self-signed certs", func(t *testing.T) {
		t.Parallel()
		db, err := OpenCanonicalDBService(cfg.Gateway.DataDir, cfg.Gateway.SecretsDir, filepath.Join(cfg.Gateway.DataDir, "vault"), logger, true, "", false)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })

		cfg.Gateway.PKIDir = tempDir(t)
		cfg.Gateway.SecretsDir = tempDir(t)
		cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

		ls, err := newGatewayModeServiceFromComponents(cfg, logger, db, pubsub)
		require.NoError(t, err)
		assert.NotNil(t, ls)
		assert.NotNil(t, ls.server)
		assert.NotNil(t, ls.pki)
		assert.False(t, ls.running)
	})
}

func TestGatewayModeService_StateManagement(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	cfg.Gateway.DataDir = tempDir(t)
	cfg.Gateway.PKIDir = tempDir(t)
	cfg.Gateway.SecretsDir = tempDir(t)

	db, err := OpenCanonicalDBService(cfg.Gateway.DataDir, cfg.Gateway.SecretsDir, filepath.Join(cfg.Gateway.DataDir, "vault"), logger, true, "", false)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceFromComponents(cfg, logger, db, pubsub)
	require.NoError(t, err)

	t.Run("Initial state", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ls.IsRunning())
		assert.False(t, ls.IsReady())
	})

	t.Run("IsRunning returns false when not running", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ls.IsRunning())
	})

	t.Run("IsReady returns false when not ready", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ls.IsReady())
	})

	t.Run("State getters are thread-safe", func(t *testing.T) {
		t.Parallel()
		// Test that we can call state methods concurrently
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				ls.IsRunning()
				ls.IsReady()
				done <- true
			}()
		}

		// Wait for all goroutines to finish
		for i := 0; i < 10; i++ {
			select {
			case <-done:
			case <-time.After(1 * time.Second):
				t.Fatal("State methods deadlocked")
			}
		}
	})
}

func TestNewGatewayModeServiceFromComponents(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := tempDir(t)
	pkiDir := tempDir(t)
	secretsDir := tempDir(t)
	db, err := OpenCanonicalDBService(dbDir, secretsDir, filepath.Join(dbDir, "vault"), logger, true, "", false)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewGatewayWebSocketHandler(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = secretsDir
	cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls, err := newGatewayModeServiceFromComponents(cfg, logger, db, pubsub)
	require.NoError(t, err)
	assert.NotNil(t, ls)
	assert.Equal(t, db, ls.db)
	assert.Equal(t, pubsub, ls.pubsub)
	assert.NotNil(t, ls.server)
}
