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
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGatewayService(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Ensure directories are set for tests to avoid SQLITE_CANTOPEN
	cfg.Gateway.DataDir = t.TempDir()
	cfg.Gateway.PKIDir = t.TempDir()
	cfg.Gateway.SecretsDir = t.TempDir()

	t.Run("Default configuration with self-signed certs", func(t *testing.T) {
		t.Parallel()
		db, err := OpenGatewayDBService(cfg.Gateway.DataDir, cfg.Gateway.SecretsDir, logger, true)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		pubsub := NewPubSubBroker(logger)
		t.Cleanup(func() { pubsub.Close() })

		cfg.Gateway.PKIDir = t.TempDir()
		cfg.Gateway.SecretsDir = t.TempDir()
		cfg.Gateway.BootstrapPort = constants.Ports.OperatorBootstrapHttps

		ls, err := newGatewayServiceFromComponents(cfg, logger, db, pubsub)
		require.NoError(t, err)
		assert.NotNil(t, ls)
		assert.NotNil(t, ls.server)
		assert.NotNil(t, ls.pki)
		assert.False(t, ls.running)
	})
}

func TestGatewayService_StateManagement(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	cfg.Gateway.DataDir = t.TempDir()
	cfg.Gateway.PKIDir = t.TempDir()
	cfg.Gateway.SecretsDir = t.TempDir()

	db, err := OpenGatewayDBService(cfg.Gateway.DataDir, cfg.Gateway.SecretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewPubSubBroker(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.BootstrapPort = constants.Ports.OperatorBootstrapHttps

	ls, err := newGatewayServiceFromComponents(cfg, logger, db, pubsub)
	require.NoError(t, err)

	t.Run("Initial state", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ls.IsRunning())
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

func TestNewGatewayServiceFromComponents(t *testing.T) {
	t.Parallel()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewPubSubBroker(logger)
	t.Cleanup(func() { pubsub.Close() })

	cfg.Gateway.PKIDir = pkiDir
	cfg.Gateway.SecretsDir = secretsDir
	cfg.Gateway.BootstrapPort = constants.Ports.OperatorBootstrapHttps

	ls, err := newGatewayServiceFromComponents(cfg, logger, db, pubsub)
	require.NoError(t, err)
	assert.NotNil(t, ls)
	assert.Equal(t, db, ls.db)
	assert.Equal(t, pubsub, ls.pubsub)
	assert.NotNil(t, ls.server)
}

func TestAutoTLSListener(t *testing.T) {
	t.Parallel()

	// Create a simple test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create a test TLS config from PEM strings
	certPEM, keyPEM := testutil.GenerateTestCertificate(t, "test-cert")
	cert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	require.NoError(t, err)

	tlsConfig := &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return &cert, nil
		},
	}

	// Start a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })

	// Wrap with autoTLSListener
	autoLn := &autoTLSListener{
		Listener:  ln,
		tlsConfig: tlsConfig,
		logger:    testutil.NewTestLogger(),
	}

	// Start server
	server := &http.Server{
		Handler: handler,
	}
	go server.Serve(autoLn)
	t.Cleanup(func() { server.Close() })

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	addr := ln.Addr().String()

	t.Run("HTTP connection works", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Get("http://" + addr + "/")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "OK", string(body))
	})

	t.Run("HTTPS connection works", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			},
		}
		resp, err := client.Get("https://" + addr + "/")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "OK", string(body))
	})
}
