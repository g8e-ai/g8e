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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticateWithDeviceTokenUsingClient(t *testing.T) {
	logger := testutil.NewTestLogger()
	token := "dlk_abcdefghijklmnopqrstuvwxyz012345"

	t.Run("successful authentication", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Contains(t, r.URL.Path, "/auth/link/"+token+"/register")

			var info DeviceInfo
			err := json.NewDecoder(r.Body).Decode(&info)
			require.NoError(t, err)
			assert.NotEmpty(t, info.SystemFingerprint)
			assert.NotEmpty(t, info.Hostname)

			resp := deviceRegisterResponse{
				Success:           true,
				OperatorSessionID: "sess-123",
				OperatorID:        "op-456",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		// Use the test server's client which trusts its own CA
		result, err := authenticateWithDeviceTokenUsingClient(token, server.Listener.Addr().String(), logger, server.Client(), "testuser")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "sess-123", result.OperatorSessionID)
		assert.Equal(t, "op-456", result.OperatorID)
	})

	t.Run("invalid token format", func(t *testing.T) {
		result, err := authenticateWithDeviceTokenUsingClient("invalid", "localhost", logger, http.DefaultClient, "")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid device token format")
	})

	t.Run("server error response", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := deviceRegisterResponse{
				Success: false,
				Error:   "invalid token",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		result, err := authenticateWithDeviceTokenUsingClient(token, server.Listener.Addr().String(), logger, server.Client(), "")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "device registration failed: invalid token")
	})

	t.Run("server returns 500 without error field", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"success": false}`)
		}))
		defer server.Close()

		result, err := authenticateWithDeviceTokenUsingClient(token, server.Listener.Addr().String(), logger, server.Client(), "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "registration failed with status 500")
		assert.Nil(t, result)
	})

	t.Run("missing operator session ID", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := deviceRegisterResponse{
				Success: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		result, err := authenticateWithDeviceTokenUsingClient(token, server.Listener.Addr().String(), logger, server.Client(), "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no operator session ID returned")
		assert.Nil(t, result)
	})
}

func TestValidateDeviceToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "valid token with lowercase",
			token: "dlk_abcdefghijklmnopqrstuvwxyz012345",
			want:  true,
		},
		{
			name:  "valid token with uppercase",
			token: "dlk_ABCDEFGHIJKLMNOPQRSTUVWXYZ012345",
			want:  true,
		},
		{
			name:  "valid token with mixed case and numbers",
			token: "dlk_AbCdEfGhIjKlMnOpQrStUvWxYz123456",
			want:  true,
		},
		{
			name:  "valid token with underscores and hyphens",
			token: "dlk_abc-def_ghi-jkl_mno-pqr_stu-vw12",
			want:  true,
		},
		{
			name:  "empty string",
			token: "",
			want:  false,
		},
		{
			name:  "wrong prefix",
			token: "fdl_abcdefghijklmnopqrstuvwxyz012345",
			want:  false,
		},
		{
			name:  "no prefix",
			token: "abcdefghijklmnopqrstuvwxyz0123456789",
			want:  false,
		},
		{
			name:  "too short",
			token: "dlk_abc123",
			want:  false,
		},
		{
			name:  "too long",
			token: "dlk_abcdefghijklmnopqrstuvwxyz0123456789extra",
			want:  false,
		},
		{
			name:  "invalid characters",
			token: "dlk_abcdefghijklmnopqrstuvwx!@#$%^",
			want:  false,
		},
		{
			name:  "spaces",
			token: "dlk_abcdefghijklmnopqrstuvwxyz 12345",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateDeviceToken(tt.token); got != tt.want {
				t.Errorf("ValidateDeviceToken(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}
