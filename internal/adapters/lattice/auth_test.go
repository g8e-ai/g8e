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

package lattice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestOAuthServer returns an httptest.Server that simulates the Lattice
// OAuth2 token endpoint. The handler records the number of token requests
// and inspects the sandbox header.
func newTestOAuthServer(t *testing.T, token string, expiresIn int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		t.Logf("OAuth request body: %s", string(body))

		resp := map[string]interface{}{
			"access_token": token,
			"expires_in":   expiresIn,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	return httptest.NewServer(mux)
}

// newInspectOAuthServer returns a server that records request headers for inspection.
type oauthRequestInspector struct {
	sandboxHeader  string
	authHeader     string
	contentType    string
	body           string
	requestCount   atomic.Int32
}

func newInspectOAuthServer(t *testing.T, token string, expiresIn int) (*httptest.Server, *oauthRequestInspector) {
	t.Helper()
	insp := &oauthRequestInspector{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		insp.requestCount.Add(1)
		insp.sandboxHeader = r.Header.Get("Anduril-Sandbox-Authorization")
		insp.contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		insp.body = string(body)

		resp := map[string]interface{}{
			"access_token": token,
			"expires_in":   expiresIn,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	return httptest.NewServer(mux), insp
}

func TestGetRequestMetadata_ReturnsAuthorizationBearerToken(t *testing.T) {
	t.Parallel()

	srv := newTestOAuthServer(t, "test-token-123", 3600)
	defer srv.Close()

	auth := NewClientCredentialsAuth("client-id", "client-secret", "", srv.URL+"/oauth/token")
	md, err := auth.GetRequestMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-token-123", md["authorization"])
}

func TestGetRequestMetadata_IncludesSandboxHeaderWhenSandboxesTokenSet(t *testing.T) {
	t.Parallel()

	srv, insp := newInspectOAuthServer(t, "test-token-456", 3600)
	defer srv.Close()

	auth := NewClientCredentialsAuth("client-id", "client-secret", "sandbox-token-value", srv.URL+"/oauth/token")
	md, err := auth.GetRequestMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-token-456", md["authorization"])
	assert.Equal(t, "Bearer sandbox-token-value", md["anduril-sandbox-authorization"])

	assert.Equal(t, "Bearer sandbox-token-value", insp.sandboxHeader)
}

func TestGetRequestMetadata_OmitsSandboxHeaderWhenSandboxesTokenEmpty(t *testing.T) {
	t.Parallel()

	srv, insp := newInspectOAuthServer(t, "test-token-789", 3600)
	defer srv.Close()

	auth := NewClientCredentialsAuth("client-id", "client-secret", "", srv.URL+"/oauth/token")
	md, err := auth.GetRequestMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-token-789", md["authorization"])
	_, hasSandbox := md["anduril-sandbox-authorization"]
	assert.False(t, hasSandbox)

	assert.Empty(t, insp.sandboxHeader)
}

func TestGetRequestMetadata_TokenIsReusedAcrossCallsWithinValidityWindow(t *testing.T) {
	t.Parallel()

	srv, insp := newInspectOAuthServer(t, "shared-token", 3600)
	defer srv.Close()

	auth := NewClientCredentialsAuth("client-id", "client-secret", "", srv.URL+"/oauth/token")

	for i := 0; i < 3; i++ {
		md, err := auth.GetRequestMetadata(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "Bearer shared-token", md["authorization"])
	}

	assert.Equal(t, int32(1), insp.requestCount.Load())
}

func TestForceRefresh_ClearsCachedTokenForcingReacquisition(t *testing.T) {
	t.Parallel()

	srv, insp := newInspectOAuthServer(t, "refreshed-token", 3600)
	defer srv.Close()

	auth := NewClientCredentialsAuth("client-id", "client-secret", "", srv.URL+"/oauth/token")

	md, err := auth.GetRequestMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer refreshed-token", md["authorization"])
	assert.Equal(t, int32(1), insp.requestCount.Load())

	auth.ForceRefresh()

	md2, err := auth.GetRequestMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer refreshed-token", md2["authorization"])
	assert.Equal(t, int32(2), insp.requestCount.Load())
}

func TestRequireTransportSecurity_ReturnsTrue(t *testing.T) {
	t.Parallel()

	auth := NewClientCredentialsAuth("id", "secret", "", "https://example.com")
	assert.True(t, auth.RequireTransportSecurity())
}

func TestAcquireToken_ReturnsErrLatticeTokenAcquireFailedOnNon200(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	auth := NewClientCredentialsAuth("bad-id", "bad-secret", "", srv.URL+"/oauth/token")
	_, err := auth.GetRequestMetadata(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticeTokenAcquireFailed)
}

func TestAcquireToken_SendsFormEncodedClientCredentials(t *testing.T) {
	t.Parallel()

	srv, insp := newInspectOAuthServer(t, "form-token", 3600)
	defer srv.Close()

	auth := NewClientCredentialsAuth("form-client-id", "form-client-secret", "", srv.URL+"/oauth/token")
	_, err := auth.GetRequestMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "application/x-www-form-urlencoded", insp.contentType)
	assert.Contains(t, insp.body, "grant_type=client_credentials")
	assert.Contains(t, insp.body, "client_id=form-client-id")
	assert.Contains(t, insp.body, "client_secret=form-client-secret")
}

func TestAcquireToken_ReturnsErrLatticeTokenAcquireFailedOnEmptyAccessToken(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "",
			"expires_in":   3600,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	auth := NewClientCredentialsAuth("id", "secret", "", srv.URL+"/oauth/token")
	_, err := auth.GetRequestMetadata(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticeTokenAcquireFailed)
}

func TestNewClientCredentialsAuth_SetsHttpClientTimeout(t *testing.T) {
	t.Parallel()

	auth := NewClientCredentialsAuth("id", "secret", "", "https://example.com")
	require.NotNil(t, auth.httpClient)
	assert.Equal(t, 10*time.Second, auth.httpClient.Timeout)
}
