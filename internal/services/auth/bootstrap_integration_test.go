// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestHTTPAuth_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/operators/reauth", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get(constants.HeaderContentType))
		// mTLS authentication - no Authorization header expected
		assert.Empty(t, r.Header.Get(constants.HeaderAuthorization))

		resp := AuthServicesResponse{
			Success:           true,
			OperatorSessionId: "sess-abc",
			OperatorID:        "op-xyz",
			UserID:            "user-1",
			Config: &BootstrapConfig{
				MaxConcurrentTasks:       10,
				MaxMemoryMB:              1024,
				HeartbeatIntervalSeconds: 30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	svc := newTestBootstrapService(t, server)
	bootCfg, err := svc.RequestBootstrapConfig(context.Background())

	require.NoError(t, err)
	require.NotNil(t, bootCfg)
	assert.Equal(t, "sess-abc", bootCfg.OperatorSessionId)
	assert.Equal(t, "op-xyz", bootCfg.OperatorID)
	assert.Equal(t, 10, bootCfg.MaxConcurrentTasks)
	assert.Equal(t, 1024, bootCfg.MaxMemoryMB)
}

func TestRequestHTTPAuth_PropagatesCerts(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := AuthServicesResponse{
			Success:           true,
			OperatorSessionId: "sess-cert",
			OperatorID:        "op-cert",
			Config:            &BootstrapConfig{},
			OperatorCert:      "cert-pem-data",
			OperatorCertKey:   "key-pem-data",
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	svc := newTestBootstrapService(t, server)
	bootCfg, err := svc.RequestBootstrapConfig(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "cert-pem-data", bootCfg.OperatorCert)
	assert.Equal(t, "key-pem-data", bootCfg.OperatorCertKey)
}

func TestRequestHTTPAuth_Failure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := AuthServicesResponse{
			Success: false,
			Error:   json.RawMessage(`"invalid api key"`),
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	svc := newTestBootstrapService(t, server)
	_, err := svc.RequestBootstrapConfig(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid api key")
}

func TestRequestHTTPAuth_MissingSessionID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := AuthServicesResponse{
			Success:           true,
			OperatorSessionId: "",
			Config:            &BootstrapConfig{},
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	svc := newTestBootstrapService(t, server)
	_, err := svc.RequestBootstrapConfig(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator_session_id")
}

func TestRequestHTTPAuth_MissingConfig(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := AuthServicesResponse{
			Success:           true,
			OperatorSessionId: "sess-ok",
			Config:            nil,
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	svc := newTestBootstrapService(t, server)
	_, err := svc.RequestBootstrapConfig(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no configuration")
}

func TestRequestHTTPAuth_InvalidJSON(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not-json{{{")
	}))
	defer server.Close()

	svc := newTestBootstrapService(t, server)
	_, err := svc.RequestBootstrapConfig(context.Background())

	require.Error(t, err)
}

func TestRequestHTTPAuth_RuntimeConfigSent(t *testing.T) {
	var capturedBody operatorAuthRequest

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		resp := AuthServicesResponse{
			Success:           true,
			OperatorSessionId: "sess-rc",
			OperatorID:        "op-rc",
			Config:            &BootstrapConfig{},
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	hostport := strings.TrimPrefix(server.URL, "https://")
	host, portStr, err := net.SplitHostPort(hostport)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	cfg := testutil.NewTestConfig(t)
	cfg.Endpoint = host
	cfg.HTTPSPort = port
	cfg.CloudMode = true
	cfg.CloudProvider = "aws"
	cfg.ExecutionVaultEnabled = true
	cfg.NoGit = false
	cfg.LogLevel = "debug"

	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger, newTestTLSConfig(t))
	require.NoError(t, err)
	svc.httpClient = server.Client()

	_, err = svc.RequestBootstrapConfig(context.Background())
	require.NoError(t, err)

	require.NotNil(t, capturedBody.RuntimeConfig)
	runtimeCfg, err := models.UnmarshalOperatorRuntimeConfig(capturedBody.RuntimeConfig)
	require.NoError(t, err)
	assert.True(t, runtimeCfg.CloudMode)
	assert.Equal(t, "aws", runtimeCfg.CloudProvider)
	assert.True(t, runtimeCfg.LocalStorageEnabled)
	assert.False(t, runtimeCfg.NoGit)
	assert.Equal(t, "debug", runtimeCfg.LogLevel)

}

func TestRequestHTTPAuth_ContextCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer server.Close()

	svc := newTestBootstrapService(t, server)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.requestHTTPAuth(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}
