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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/response"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// newTestPasskeyController constructs a PasskeyController backed by a real
// PasskeyHandler whose PasskeyService has a nil docStore. Construction succeeds
// because the service only stores the docStore pointer; it is never reached on
// the method-guard / auth-guard early-return paths exercised below. This keeps
// the test a fast unit test with no DB.
func newTestPasskeyController(t *testing.T) *PasskeyController {
	t.Helper()
	logger := testutil.NewTestLogger()
	resp := response.NewWriter(logger)
	svc, err := NewPasskeyService(nil, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	require.NoError(t, err)
	h := NewPasskeyHandler(PasskeyHandlerDeps{Service: svc, Responder: resp})
	return newPasskeyController(PasskeyControllerDeps{Handler: h})
}

func TestNewPasskeyController_Wiring(t *testing.T) {
	c := newTestPasskeyController(t)

	assert.NotNil(t, c.PasskeyHandler(), "PasskeyHandler accessor should return the wrapped handler")
}

func TestNewPasskeyController_NilDeps(t *testing.T) {
	c := newPasskeyController(PasskeyControllerDeps{})
	require.NotNil(t, c)
	assert.Nil(t, c.PasskeyHandler(), "PasskeyHandler accessor should be nil when no handler is wired")
}

// TestPasskeyController_CfgReturningMethods_Delegate verifies the four
// cfg-returning methods return a non-nil http.HandlerFunc that delegates to
// the underlying PasskeyHandler. A GET request hits the underlying handler's
// POST-only method guard and returns 405, proving the call reached it.
func TestPasskeyController_CfgReturningMethods_Delegate(t *testing.T) {
	c := newTestPasskeyController(t)
	cfg := passkeyHandlerConfig{}

	cases := []struct {
		name string
		fn   func(cfg passkeyHandlerConfig) http.HandlerFunc
	}{
		{"registerChallenge", c.registerChallenge},
		{"registerVerify", c.registerVerify},
		{"authenticateChallenge", c.authenticateChallenge},
		{"authenticateVerify", c.authenticateVerify},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hf := tc.fn(cfg)
			require.NotNil(t, hf, "cfg-returning method should produce a non-nil handler")

			req := httptest.NewRequest(http.MethodGet, "/auth/passkeys", nil)
			rr := httptest.NewRecorder()
			hf(rr, req)

			assert.Equal(t, http.StatusMethodNotAllowed, rr.Code, "GET should hit the POST-only guard in the underlying handler")
			assert.Contains(t, rr.Body.String(), "method not allowed")
		})
	}
}

// TestPasskeyController_DirectHandlerMethods_Delegate verifies the eight
// direct-delegation methods route through to the underlying PasskeyHandler.
// Each is exercised via an early-return guard (wrong method or missing user
// context) so no DB access is needed.
func TestPasskeyController_DirectHandlerMethods_Delegate(t *testing.T) {
	c := newTestPasskeyController(t)

	t.Run("cliStatus/POST->405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/passkeys/cli/status", nil)
		rr := httptest.NewRecorder()
		c.cliStatus(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("handleApprovalPage/POST->405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/approve/abc", nil)
		rr := httptest.NewRecorder()
		c.handleApprovalPage(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("handleApprovalAction/no-user->401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/abc/challenge", nil)
		rr := httptest.NewRecorder()
		c.handleApprovalAction(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("handleCLIApprovalStatus/POST->405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/approvals/cli/abc/status", nil)
		rr := httptest.NewRecorder()
		c.handleCLIApprovalStatus(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("handleCLIListSuspended/POST->405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/approvals/cli", nil)
		rr := httptest.NewRecorder()
		c.handleCLIListSuspended(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("handleListSuspendedTransactions/POST->405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals", nil)
		rr := httptest.NewRecorder()
		c.handleListSuspendedTransactions(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("listCredentials/POST->405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys", nil)
		rr := httptest.NewRecorder()
		c.listCredentials(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("revokeCredential/POST->405", func(t *testing.T) {
		// RevokeCredential guards on DELETE; POST hits the 405 path.
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkeys/cred1", nil)
		rr := httptest.NewRecorder()
		c.revokeCredential(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}
