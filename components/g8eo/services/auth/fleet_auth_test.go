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

// Tests for device link authentication covering multi-use (mass deployment) scenarios.
// Fleet device links have been unified into the device link model: all tokens use
// the dlk_ prefix and register at /auth/link/:token/register regardless of max_uses.

package auth

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDeviceServiceMultiUse creates an AuthenticateWithDeviceToken-compatible
// test helper that points at the given TLS test server.
func newTestDeviceServiceFromServer(t *testing.T, server *httptest.Server) (endpoint string, client *http.Client) {
	t.Helper()
	hostport := strings.TrimPrefix(server.URL, "https://")
	host, portStr, err := net.SplitHostPort(hostport)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	return fmt.Sprintf("%s:%d", host, port), server.Client()
}

func TestAuthenticateWithDeviceToken_MultiUse_InvalidFormat(t *testing.T) {
	logger := testutil.NewTestLogger()
	_, err := AuthenticateWithDeviceToken("bad-token", "localhost", logger, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid device token format")
}

func TestAuthenticateWithDeviceToken_MultiUse_FdlPrefixRejected(t *testing.T) {
	logger := testutil.NewTestLogger()
	_, err := AuthenticateWithDeviceToken("fdl_abcdefghijklmnopqrstuvwxyz123456", "localhost", logger, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid device token format")
}

func TestAuthenticateWithDeviceToken_MultiUse_Success(t *testing.T) {
	const validToken = "dlk_abcdefghijklmnopqrstuvwxyz123456"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, fmt.Sprintf("/auth/link/%s/register", validToken), r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get(constants.HeaderContentType))

		var body DeviceInfo
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.NotEmpty(t, body.SystemFingerprint)
		assert.NotEmpty(t, body.Hostname)
		assert.NotEmpty(t, body.OS)
		assert.NotEmpty(t, body.Arch)

		resp := deviceRegisterResponse{
			Success:           true,
			OperatorSessionID: "sess-dlk-123",
			OperatorID:        "op-dlk-456",
		}
		w.Header().Set(constants.HeaderContentType, "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	endpoint, httpClient := newTestDeviceServiceFromServer(t, server)
	logger := testutil.NewTestLogger()

	result, err := authenticateWithDeviceTokenUsingClient(validToken, endpoint, logger, httpClient, "testuser")

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "sess-dlk-123", result.OperatorSessionID)
	assert.Equal(t, "op-dlk-456", result.OperatorID)
}

func TestAuthenticateWithDeviceToken_MultiUse_RegistrationFailure(t *testing.T) {
	const validToken = "dlk_abcdefghijklmnopqrstuvwxyz123456"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := deviceRegisterResponse{
			Success: false,
			Error:   json.RawMessage(`"max uses exhausted"`),
		}
		w.Header().Set(constants.HeaderContentType, "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	endpoint, httpClient := newTestDeviceServiceFromServer(t, server)
	logger := testutil.NewTestLogger()

	_, err := authenticateWithDeviceTokenUsingClient(validToken, endpoint, logger, httpClient, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max uses exhausted")
}

func TestAuthenticateWithDeviceToken_MultiUse_RegistrationFailureNoMessage(t *testing.T) {
	const validToken = "dlk_abcdefghijklmnopqrstuvwxyz123456"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		resp := deviceRegisterResponse{Success: false}
		w.Header().Set(constants.HeaderContentType, "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	endpoint, httpClient := newTestDeviceServiceFromServer(t, server)
	logger := testutil.NewTestLogger()

	_, err := authenticateWithDeviceTokenUsingClient(validToken, endpoint, logger, httpClient, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "device registration failed")
}

func TestAuthenticateWithDeviceToken_MultiUse_MissingSessionID(t *testing.T) {
	const validToken = "dlk_abcdefghijklmnopqrstuvwxyz123456"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := deviceRegisterResponse{
			Success:           true,
			OperatorSessionID: "",
			OperatorID:        "op-dlk-456",
		}
		w.Header().Set(constants.HeaderContentType, "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	endpoint, httpClient := newTestDeviceServiceFromServer(t, server)
	logger := testutil.NewTestLogger()

	_, err := authenticateWithDeviceTokenUsingClient(validToken, endpoint, logger, httpClient, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no operator session ID")
}

func TestAuthenticateWithDeviceToken_MultiUse_InvalidJSON(t *testing.T) {
	const validToken = "dlk_abcdefghijklmnopqrstuvwxyz123456"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(constants.HeaderContentType, "application/json")
		fmt.Fprint(w, "not-json{{{")
	}))
	defer server.Close()

	endpoint, httpClient := newTestDeviceServiceFromServer(t, server)
	logger := testutil.NewTestLogger()

	_, err := authenticateWithDeviceTokenUsingClient(validToken, endpoint, logger, httpClient, "")

	require.Error(t, err)
}

func TestAuthenticateWithDeviceToken_MultiUse_RegistersAtLinkEndpoint(t *testing.T) {
	const validToken = "dlk_MultiUseLinkAbCdEfGhIjKlMnOpQrSt"
	var capturedPath string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		resp := deviceRegisterResponse{
			Success:           true,
			OperatorSessionID: "sess-multi-789",
			OperatorID:        "op-multi-012",
		}
		w.Header().Set(constants.HeaderContentType, "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	endpoint, httpClient := newTestDeviceServiceFromServer(t, server)
	logger := testutil.NewTestLogger()

	_, err := authenticateWithDeviceTokenUsingClient(validToken, endpoint, logger, httpClient, "")

	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("/auth/link/%s/register", validToken), capturedPath,
		"multi-use device links must register at /auth/link/:token/register")
}
