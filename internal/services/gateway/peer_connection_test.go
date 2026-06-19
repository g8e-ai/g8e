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

//go:build integration

package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeerConnectionManager_Standalone(t *testing.T) {
	infra := setupTestInfrastructure(t, true)

	// Ensure FederationSeedURL is empty
	infra.Cfg.Gateway.FederationSeedURL = ""

	pcm := NewPeerConnectionManager(infra.Cfg, infra.Logger, infra.DB, infra.PKI)
	err := pcm.Start(context.Background())
	require.NoError(t, err)

	assert.False(t, pcm.IsConnected())
	pcm.Stop()
}

func TestPeerConnectionManager_InvalidURL(t *testing.T) {
	infra := setupTestInfrastructure(t, true)

	// Set invalid URL (non-HTTPS)
	infra.Cfg.Gateway.FederationSeedURL = "http://localhost:8080"

	pcm := NewPeerConnectionManager(infra.Cfg, infra.Logger, infra.DB, infra.PKI)
	err := pcm.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "seed URL must use HTTPS scheme")
}

func TestPeerConnectionManager_GatewayID(t *testing.T) {
	baseDir := t.TempDir()
	err := constants.InitPathsWithBase(baseDir)
	require.NoError(t, err)

	infra := setupTestInfrastructure(t, true)

	// Create data dir if it doesn't exist (InitPathsWithBase doesn't create them)
	err = os.MkdirAll(constants.Paths.Infra.DataDir, 0755)
	require.NoError(t, err)

	pcm := NewPeerConnectionManager(infra.Cfg, infra.Logger, infra.DB, infra.PKI)

	// Test generation
	id1, err := pcm.generateAndStoreGatewayID()
	require.NoError(t, err)
	assert.NotEmpty(t, id1)
	assert.Contains(t, id1, "gw-")

	// Verify file exists
	data, err := os.ReadFile(constants.GatewayIDPath)
	require.NoError(t, err)
	assert.Equal(t, id1, string(data))

	// Test loading
	id2, err := pcm.loadGatewayID()
	require.NoError(t, err)
	assert.Equal(t, id1, id2)
}

func TestPeerConnectionManager_StartEnrollment(t *testing.T) {
	baseDir := t.TempDir()
	err := constants.InitPathsWithBase(baseDir)
	require.NoError(t, err)

	infra := setupTestInfrastructure(t, true)

	// Set valid seed URL (using HTTPS)
	infra.Cfg.Gateway.FederationSeedURL = "https://seed.g8e.local"

	// Ensure data and pki dirs exist
	err = os.MkdirAll(constants.Paths.Infra.DataDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(constants.Paths.Infra.PkiDir, "peer"), 0755)
	require.NoError(t, err)

	pcm := NewPeerConnectionManager(infra.Cfg, infra.Logger, infra.DB, infra.PKI)

	// Start should trigger enrollment
	// Note: pcm.Start starts a background goroutine for connectionLoop.
	// We use a context with timeout to prevent tests from hanging.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = pcm.Start(ctx)
	require.NoError(t, err)

	// Check if gateway ID was generated
	assert.NotEmpty(t, pcm.gatewayID)

	// Check if peer cert files were created
	assert.FileExists(t, constants.PeerCertPath)
	assert.FileExists(t, constants.PeerKeyPath)
	assert.FileExists(t, constants.PeerChainPath)

	pcm.Stop()
}

func TestPeerConnectionManager_Backoff(t *testing.T) {
	current := time.Second
	max := 5 * time.Second

	// First step
	next := calculateBackoff(current, max)
	assert.Equal(t, 2*time.Second, next)

	// Second step
	next = calculateBackoff(next, max)
	assert.Equal(t, 4*time.Second, next)

	// Third step (capped at max)
	next = calculateBackoff(next, max)
	assert.Equal(t, 5*time.Second, next)
}

func TestPeerConnectionManager_HealthCheck(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	pcm := NewPeerConnectionManager(infra.Cfg, infra.Logger, infra.DB, infra.PKI)

	// Test with nil client
	pcm.mu.Lock()
	pcm.client = nil
	pcm.mu.Unlock()
	err := pcm.healthCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "client not initialized")

	// Test with successful health check
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/g8e/federation/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	pcm.seedURL = server.URL
	pcm.mu.Lock()
	pcm.client = server.Client()
	pcm.mu.Unlock()

	err = pcm.healthCheck()
	assert.NoError(t, err)

	// Test with failed health check (500 status)
	server2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/g8e/federation/health" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server2.Close()

	pcm.seedURL = server2.URL
	pcm.mu.Lock()
	pcm.client = server2.Client()
	pcm.mu.Unlock()

	err = pcm.healthCheck()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code")
}

func TestPeerConnectionManager_Connect(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	pcm := NewPeerConnectionManager(infra.Cfg, infra.Logger, infra.DB, infra.PKI)

	// Test with empty peerChainPEM (TLS pool build will fail)
	pcm.peerChainPEM = ""
	err := pcm.connect()
	assert.Error(t, err)

	// Test with invalid peerChainPEM
	pcm.peerChainPEM = "invalid PEM data"
	err = pcm.connect()
	assert.Error(t, err)
}

func TestPeerConnectionManager_ConnectionLoop_ContextCancel(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	pcm := NewPeerConnectionManager(infra.Cfg, infra.Logger, infra.DB, infra.PKI)

	// Test context cancellation - should return promptly
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	pcm.wg.Add(1)
	pcm.connectionLoop(ctx) // should return via ctx.Done() select case
}

func TestPeerConnectionManager_ConnectionLoop_Timeout(t *testing.T) {
	infra := setupTestInfrastructure(t, true)
	pcm := NewPeerConnectionManager(infra.Cfg, infra.Logger, infra.DB, infra.PKI)

	// Test connect-fail → backoff path with timeout
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/g8e/federation/health" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	pcm.seedURL = server.URL
	pcm.mu.Lock()
	pcm.client = server.Client()
	pcm.mu.Unlock()

	// Set a short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// The loop will attempt connect, succeed, then health check
	// Since we have a valid client, it should connect successfully
	pcm.wg.Add(1)
	pcm.connectionLoop(ctx)
}
