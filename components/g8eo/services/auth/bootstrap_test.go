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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/httpclient"
	system "github.com/g8e-ai/g8e/components/g8eo/services/system"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBootstrapService(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger)

	require.NoError(t, err)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.httpClient)
}

func TestNewBootstrapService_TLSPinning(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger)
	require.NoError(t, err)

	transport, ok := svc.httpClient.Transport.(*http.Transport)
	require.True(t, ok, "expected *http.Transport")
	require.NotNil(t, transport.TLSClientConfig)
	assert.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
	assert.Empty(t, transport.TLSClientConfig.Certificates)
}

func TestNewBootstrapService_HasTimeout(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger)
	require.NoError(t, err)

	assert.NotZero(t, svc.httpClient.Timeout)
}

// newTestBootstrapService creates a BootstrapService pointed at the given
// TLS test server. requestHTTPAuth builds "https://host:port/..." so the
// server must speak TLS; use httptest.NewTLSServer and pass its Client()
// which already trusts the test CA.
func newTestBootstrapService(t *testing.T, server *httptest.Server) *BootstrapService {
	t.Helper()
	hostport := strings.TrimPrefix(server.URL, "https://")
	host, portStr, err := net.SplitHostPort(hostport)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	cfg := testutil.NewTestConfig(t)
	cfg.Endpoint = host
	cfg.HTTPPort = port
	logger := testutil.NewTestLogger()
	svc, err := NewBootstrapService(cfg, logger)
	require.NoError(t, err)
	svc.httpClient = server.Client()
	return svc
}

func TestRequestHTTPAuth_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/api/auth/operator", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get(constants.HeaderContentType))
		require.NotEmpty(t, r.Header.Get(constants.HeaderAuthorization))

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
		require.NoError(t, json.NewEncoder(w).Encode(resp))
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
		require.NoError(t, json.NewEncoder(w).Encode(resp))
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
		require.NoError(t, json.NewEncoder(w).Encode(resp))
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
		require.NoError(t, json.NewEncoder(w).Encode(resp))
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
		require.NoError(t, json.NewEncoder(w).Encode(resp))
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
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		resp := AuthServicesResponse{
			Success:           true,
			OperatorSessionId: "sess-rc",
			OperatorID:        "op-rc",
			Config:            &BootstrapConfig{},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	hostport := strings.TrimPrefix(server.URL, "https://")
	host, portStr, err := net.SplitHostPort(hostport)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	cfg := testutil.NewTestConfig(t)
	cfg.Endpoint = host
	cfg.HTTPPort = port
	cfg.CloudMode = true
	cfg.CloudProvider = "aws"
	cfg.LocalStoreEnabled = true
	cfg.NoGit = false
	cfg.LogLevel = "debug"
	cfg.WSSPort = 443
	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger)
	require.NoError(t, err)
	svc.httpClient = server.Client()

	_, err = svc.RequestBootstrapConfig(context.Background())
	require.NoError(t, err)

	require.NotNil(t, capturedBody.RuntimeConfig)
	assert.True(t, capturedBody.RuntimeConfig.CloudMode)
	assert.Equal(t, "aws", capturedBody.RuntimeConfig.CloudProvider)
	assert.True(t, capturedBody.RuntimeConfig.LocalStorageEnabled)
	assert.False(t, capturedBody.RuntimeConfig.NoGit)
	assert.Equal(t, "debug", capturedBody.RuntimeConfig.LogLevel)
	assert.Equal(t, 443, capturedBody.RuntimeConfig.WSSPort)
}

func TestRequestHTTPAuth_SessionAuthMode(t *testing.T) {
	var capturedBody operatorAuthRequest

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		resp := AuthServicesResponse{
			Success:           true,
			OperatorSessionId: "sess-device",
			OperatorID:        "op-device",
			Config:            &BootstrapConfig{},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	hostport := strings.TrimPrefix(server.URL, "https://")
	host, portStr, err := net.SplitHostPort(hostport)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	cfg := testutil.NewTestConfig(t)
	cfg.Endpoint = host
	cfg.HTTPPort = port
	cfg.AuthMode = constants.Status.AuthMode.OperatorSession
	cfg.OperatorSessionId = "pre-auth-session-id"
	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger)
	require.NoError(t, err)
	svc.httpClient = server.Client()

	_, err = svc.RequestBootstrapConfig(context.Background())
	require.NoError(t, err)

	assert.Equal(t, constants.Status.AuthMode.OperatorSession, capturedBody.AuthMode)
	assert.Equal(t, "pre-auth-session-id", capturedBody.OperatorSessionID)
}

func TestApplyBootstrapConfig_AppliesAllFields(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger)
	require.NoError(t, err)

	bootCfg := &BootstrapConfig{
		MaxConcurrentTasks:       50,
		MaxMemoryMB:              4096,
		HeartbeatIntervalSeconds: 60,
		OperatorID:               "op-applied",
		OperatorSessionId:        "sess-applied",
	}

	err = svc.ApplyBootstrapConfig(bootCfg)
	require.NoError(t, err)

	assert.Equal(t, 50, cfg.MaxConcurrentTasks)
	assert.Equal(t, 4096, cfg.MaxMemoryMB)
	assert.Equal(t, 60, int(cfg.HeartbeatInterval.Seconds()))
	assert.Equal(t, "op-applied", cfg.OperatorID)
	assert.Equal(t, "sess-applied", cfg.OperatorSessionId)
}

func TestApplyBootstrapConfig_ZeroValuesNotOverridden(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	originalTasks := cfg.MaxConcurrentTasks
	originalMemory := cfg.MaxMemoryMB
	originalInterval := cfg.HeartbeatInterval
	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger)
	require.NoError(t, err)

	bootCfg := &BootstrapConfig{
		MaxConcurrentTasks:       0,
		MaxMemoryMB:              0,
		HeartbeatIntervalSeconds: 0,
		OperatorID:               "op-partial",
		OperatorSessionId:        "sess-partial",
	}

	err = svc.ApplyBootstrapConfig(bootCfg)
	require.NoError(t, err)

	assert.Equal(t, originalTasks, cfg.MaxConcurrentTasks)
	assert.Equal(t, originalMemory, cfg.MaxMemoryMB)
	assert.Equal(t, originalInterval, cfg.HeartbeatInterval)
	assert.Equal(t, "op-partial", cfg.OperatorID)
	assert.Equal(t, "sess-partial", cfg.OperatorSessionId)
}

func TestApplyBootstrapConfig_InvalidCertIgnored(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	svc, err := NewBootstrapService(cfg, logger)
	require.NoError(t, err)

	bootCfg := &BootstrapConfig{
		OperatorID:        "op-badcert",
		OperatorSessionId: "sess-badcert",
		OperatorCert:      "not-a-real-cert",
		OperatorCertKey:   "not-a-real-key",
	}

	err = svc.ApplyBootstrapConfig(bootCfg)
	require.NoError(t, err)
	assert.Equal(t, "op-badcert", cfg.OperatorID)
}

func TestAuthServicesResponse_JSONParsing(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		jsonData := `{
			"success": true,
			"operator_session_id": "session-123",
			"operator_id": "op-456",
			"user_id": "user-789",
			"config": {
				"max_concurrent_tasks": 25,
				"max_memory_mb": 2048,
				"heartbeat_interval_seconds": 30
			}
		}`

		var resp AuthServicesResponse
		err := json.Unmarshal([]byte(jsonData), &resp)

		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, "session-123", resp.OperatorSessionId)
		assert.Equal(t, "op-456", resp.OperatorID)
		require.NotNil(t, resp.Config)
		assert.Equal(t, 25, resp.Config.MaxConcurrentTasks)
		assert.Equal(t, 2048, resp.Config.MaxMemoryMB)
		assert.Equal(t, 30, resp.Config.HeartbeatIntervalSeconds)
	})

	t.Run("error response (bare string)", func(t *testing.T) {
		jsonData := `{"success": false, "error": "invalid api key"}`

		var resp AuthServicesResponse
		err := json.Unmarshal([]byte(jsonData), &resp)

		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid api key", httpclient.ExtractErrorMessage(resp.Error))
	})

	t.Run("error response (g8ed error envelope object)", func(t *testing.T) {
		// Regression: the server actually returns the object envelope.
		jsonData := `{"success": false, "error": {"code": "G8E-1800", "message": "already registered", "category": "auth"}}`

		var resp AuthServicesResponse
		err := json.Unmarshal([]byte(jsonData), &resp)

		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, "G8E-1800: already registered", httpclient.ExtractErrorMessage(resp.Error))
	})

	t.Run("cert fields present", func(t *testing.T) {
		jsonData := `{
			"success": true,
			"operator_session_id": "s",
			"operator_id": "o",
			"config": {},
			"operator_cert": "CERT",
			"operator_cert_key": "KEY"
		}`

		var resp AuthServicesResponse
		err := json.Unmarshal([]byte(jsonData), &resp)

		require.NoError(t, err)
		assert.Equal(t, "CERT", resp.OperatorCert)
		assert.Equal(t, "KEY", resp.OperatorCertKey)
	})
}

func TestHashAPIKey(t *testing.T) {
	t.Run("produces consistent hash", func(t *testing.T) {
		hash1 := HashAPIKey("test-api-key-12345")
		hash2 := HashAPIKey("test-api-key-12345")

		assert.Equal(t, hash1, hash2)
		assert.Len(t, hash1, 64)
	})

	t.Run("different keys produce different hashes", func(t *testing.T) {
		assert.NotEqual(t, HashAPIKey("key-one"), HashAPIKey("key-two"))
	})

	t.Run("empty string produces valid hash", func(t *testing.T) {
		assert.Len(t, HashAPIKey(""), 64)
	})
}

func TestSanitizeURL(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		contains string
		excludes string
	}{
		{"wss with password", "wss://:secretpassword@" + constants.DefaultEndpoint + ":443", constants.DefaultEndpoint, "secretpassword"},
		{"wss without password", "wss://" + constants.DefaultEndpoint + ":443", constants.DefaultEndpoint, ""},
		{"ws with password", "ws://:password@" + constants.DefaultEndpoint + ":443", constants.DefaultEndpoint, "password"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeURL(tc.input)
			assert.Contains(t, result, tc.contains)
			if tc.excludes != "" {
				assert.NotContains(t, result, tc.excludes)
			}
		})
	}

	t.Run("empty URL returns empty", func(t *testing.T) {
		assert.Empty(t, SanitizeURL(""))
	})

	t.Run("invalid URL returns invalid-url", func(t *testing.T) {
		assert.Equal(t, "invalid-url", SanitizeURL("://bad url"))
	})
}

func TestSystemInfoTools(t *testing.T) {
	t.Run("system.GetHostname", func(t *testing.T) {
		assert.NotEmpty(t, system.GetHostname())
	})

	t.Run("system.GetOSName", func(t *testing.T) {
		osName := system.GetOSName()
		assert.NotEmpty(t, osName)
		assert.Contains(t, []string{"linux", "darwin"}, osName)
	})

	t.Run("system.GetArchitecture", func(t *testing.T) {
		assert.NotEmpty(t, system.GetArchitecture())
	})

	t.Run("system.GetNumCPU", func(t *testing.T) {
		assert.Greater(t, system.GetNumCPU(), 0)
	})

	t.Run("system.GetNetworkInterfaces returns non-nil", func(t *testing.T) {
		assert.NotNil(t, system.GetNetworkInterfaces())
	})

	t.Run("system.GetLocalIP returns non-empty", func(t *testing.T) {
		assert.NotEmpty(t, system.GetLocalIP(""))
	})

	t.Run("system.GetCurrentUser returns non-empty", func(t *testing.T) {
		assert.NotEmpty(t, system.GetCurrentUser())
	})
}

func TestPerformanceMetrics(t *testing.T) {
	t.Run("system.GetCPUPercent in range", func(t *testing.T) {
		v := system.GetCPUPercent()
		assert.GreaterOrEqual(t, v, float64(0))
		assert.LessOrEqual(t, v, float64(100))
	})

	t.Run("system.GetMemoryPercent in range", func(t *testing.T) {
		v := system.GetMemoryPercent()
		assert.GreaterOrEqual(t, v, float64(0))
		assert.LessOrEqual(t, v, float64(100))
	})

	t.Run("system.GetMemoryMB non-negative", func(t *testing.T) {
		assert.GreaterOrEqual(t, system.GetMemoryMB(), 0)
	})

	t.Run("system.GetNetworkLatency non-negative", func(t *testing.T) {
		assert.GreaterOrEqual(t, system.GetNetworkLatency(), float64(0))
	})

	t.Run("system.GetUptime non-empty", func(t *testing.T) {
		assert.NotEmpty(t, system.GetUptime())
	})

	t.Run("system.GetUptimeSeconds positive", func(t *testing.T) {
		assert.Greater(t, system.GetUptimeSeconds(), int64(0))
	})

	t.Run("system.GetConnectivityStatus non-nil", func(t *testing.T) {
		assert.NotNil(t, system.GetConnectivityStatus())
	})
}
