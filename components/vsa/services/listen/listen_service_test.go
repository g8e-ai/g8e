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

package listen

import (
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewListenService(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Ensure directories are set for tests to avoid SQLITE_CANTOPEN
	cfg.Listen.DataDir = t.TempDir()
	cfg.Listen.SSLDir = t.TempDir()

	t.Run("Default configuration with self-signed certs", func(t *testing.T) {
		ls, err := NewListenService(cfg, logger)
		require.NoError(t, err)
		assert.NotNil(t, ls)
		assert.NotNil(t, ls.server)
		assert.NotNil(t, ls.certs)
		assert.False(t, ls.running)

		err = ls.db.Close()
		require.NoError(t, err)
	})

	t.Run("External TLS certificates", func(t *testing.T) {
		certDir := t.TempDir()
		certPath := certDir + "/cert.pem"
		keyPath := certDir + "/key.pem"

		// Note: We're not actually creating valid cert files here,
		// just testing that it attempts to load them.
		// Since tls.LoadX509KeyPair will fail with empty files,
		// we expect an error if they don't exist.
		cfg.Listen.TLSCertPath = certPath
		cfg.Listen.TLSKeyPath = keyPath

		ls, err := NewListenService(cfg, logger)
		assert.Error(t, err)
		assert.Nil(t, ls)
	})
}

func TestListenService_StateManagement(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	cfg.Listen.DataDir = t.TempDir()
	cfg.Listen.SSLDir = t.TempDir()

	ls, err := NewListenService(cfg, logger)
	require.NoError(t, err)
	defer ls.db.Close()

	t.Run("Initial state", func(t *testing.T) {
		assert.False(t, ls.IsRunning())
		assert.False(t, ls.IsReady())
	})

	t.Run("State getters are thread-safe", func(t *testing.T) {
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

func TestNewListenServiceFromComponents(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	sslDir := t.TempDir()
	db, err := NewListenDBService(dbDir, sslDir, logger)
	require.NoError(t, err)
	defer db.Close()

	pubsub := NewPubSubBroker(logger)
	defer pubsub.Close()

	ls := newListenServiceFromComponents(cfg, logger, db, pubsub)
	assert.NotNil(t, ls)
	assert.Equal(t, db, ls.db)
	assert.Equal(t, pubsub, ls.pubsub)
	assert.NotNil(t, ls.server)
}
