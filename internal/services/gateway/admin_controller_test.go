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
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/models"
)

func setupTestAdminController(t *testing.T) *AdminController {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	return newAdminController(infra.Cfg, infra.Logger, infra.DB, infra.UserSvc, infra.Responder)
}

func TestAdminControllerHandleAppPolicySigner(t *testing.T) {
	adminController := setupTestAdminController(t)

	// Create a bootstrap user for testing
	bootstrapUser, err := adminController.userSvc.CreateBootstrapUser()
	require.NoError(t, err)
	require.NotNil(t, bootstrapUser)
	t.Cleanup(func() { adminController.db.DocDelete("users", bootstrapUser.ID) })

	// Create a non-bootstrap user for testing
	regularUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { adminController.db.DocDelete("users", regularUser.ID) })

	// Create an app policy document for testing
	now := time.Now()
	appPolicy := models.AppPolicy{
		AppID:              "test-app-id",
		AllowedCollections: []string{"test-collection"},
		AllowedEventTypes:  []string{"test-event"},
		AllowedIntents:     []string{"test-intent"},
		RateLimitRPS:       100,
		MaxPayloadBytes:    1024 * 1024,
		RequireL3Approval:  false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	policyDoc, err := json.Marshal(appPolicy)
	require.NoError(t, err)
	err = adminController.db.DocSet("app_policies", "test-app-id", policyDoc)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.db.DocDelete("app_policies", "test-app-id") })

	// Generate a valid Ed25519 public key for testing
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	validPubKeyHex := hex.EncodeToString(pubKey)

	t.Run("MethodNotAllowed - GET", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/app-policies/test-app-id/signers", nil)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("BadRequest - missing app_id", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies//signers", nil)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Unauthorized - no user context", func(t *testing.T) {
		t.Parallel()
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Unauthorized - empty user_id", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, "")
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Forbidden - non-bootstrap user", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, regularUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Forbidden - app policy not found", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, bootstrapUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/nonexistent-app/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("BadRequest - invalid JSON", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, bootstrapUser.ID)
		body := []byte(`{invalid json}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - missing public_key_hex", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, bootstrapUser.ID)
		body := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - empty public_key_hex", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, bootstrapUser.ID)
		body := []byte(`{"public_key_hex": ""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - invalid hex public key", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, bootstrapUser.ID)
		body := []byte(`{"public_key_hex": "not-hex"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - invalid public key size", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, bootstrapUser.ID)
		shortKey := hex.EncodeToString([]byte{0x01, 0x02})
		body := []byte(`{"public_key_hex": "` + shortKey + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Success - valid signer registration", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), userIDKey, bootstrapUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)

		// Verify the signer was added to the database
		signerDoc, err := adminController.db.DocGet("trusted_signers", "test-app-id")
		require.NoError(t, err)
		require.NotNil(t, signerDoc)
		assert.Equal(t, "test-app-id", signerDoc.ID)

		var signer models.TrustedSigner
		signerJSON, err := json.Marshal(signerDoc.Data)
		require.NoError(t, err)
		err = json.Unmarshal(signerJSON, &signer)
		require.NoError(t, err)
		assert.Equal(t, validPubKeyHex, signer.PublicKey)
		assert.True(t, signer.Enabled)
	})
}
