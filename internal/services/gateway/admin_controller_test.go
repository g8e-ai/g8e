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

	"github.com/g8e-ai/g8e/internal/constants"
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
	t.Cleanup(func() { adminController.db.DocStore.DocDelete("users", bootstrapUser.ID) })

	// Create a non-bootstrap user for testing
	regularUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { adminController.db.DocStore.DocDelete("users", regularUser.ID) })

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
	err = adminController.db.DocStore.DocSet("app_policies", "test-app-id", policyDoc)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.db.DocStore.DocDelete("app_policies", "test-app-id") })

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
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "")
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Forbidden - non-bootstrap user", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, regularUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Forbidden - app policy not found", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/nonexistent-app/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("BadRequest - invalid JSON", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{invalid json}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - missing public_key_hex", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - empty public_key_hex", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{"public_key_hex": ""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - invalid hex public key", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{"public_key_hex": "not-hex"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - invalid public key size", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		shortKey := hex.EncodeToString([]byte{0x01, 0x02})
		body := []byte(`{"public_key_hex": "` + shortKey + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Success - valid signer registration", func(t *testing.T) {
		t.Parallel()
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)

		// Verify the signer was added to the database
		signerDoc, err := adminController.db.DocStore.DocGet("trusted_signers", "test-app-id")
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

func TestAdminControllerHandleTribunals(t *testing.T) {
	adminController := setupTestAdminController(t)

	// Create a bootstrap user for testing
	bootstrapUser, err := adminController.userSvc.CreateBootstrapUser()
	require.NoError(t, err)
	require.NotNil(t, bootstrapUser)
	t.Cleanup(func() { adminController.db.DocStore.DocDelete("users", bootstrapUser.ID) })

	// Create a non-bootstrap user for testing
	regularUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { adminController.db.DocStore.DocDelete("users", regularUser.ID) })

	// Create test signers
	pubKey1, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pubKey2, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signer1 := models.TrustedSigner{
		ID:        "tribunal-member-1",
		PublicKey: hex.EncodeToString(pubKey1),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	signer2 := models.TrustedSigner{
		ID:        "tribunal-member-2",
		PublicKey: hex.EncodeToString(pubKey2),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}

	err = adminController.db.SignerStore.AddTrustedSigner(signer1)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.db.SignerStore.DeleteTrustedSigner("tribunal-member-1") })

	err = adminController.db.SignerStore.AddTrustedSigner(signer2)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.db.SignerStore.DeleteTrustedSigner("tribunal-member-2") })

	t.Run("POST - unauthorized (no user)", func(t *testing.T) {
		ctx := context.Background()
		body := []byte(`{"id":"test-tribunal","member_app_ids":["tribunal-member-1"],"quorum":1,"require_distinct":true,"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tribunals", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("POST - forbidden (non-bootstrap user)", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, regularUser.ID)
		body := []byte(`{"id":"test-tribunal","member_app_ids":["tribunal-member-1"],"quorum":1,"require_distinct":true,"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tribunals", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("POST - invalid JSON", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{invalid json}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tribunals", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("POST - validation failure (quorum > member count)", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{"id":"test-tribunal","member_app_ids":["tribunal-member-1"],"quorum":2,"require_distinct":true,"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tribunals", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("POST - success", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		body := []byte(`{"id":"test-tribunal","member_app_ids":["tribunal-member-1","tribunal-member-2"],"quorum":2,"require_distinct":true,"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tribunals", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)

		// Verify the tribunal was created
		tribunal, err := adminController.db.TribunalStore.GetTribunal("test-tribunal")
		require.NoError(t, err)
		require.NotNil(t, tribunal)
		assert.Equal(t, "test-tribunal", tribunal.ID)
		assert.Equal(t, 2, tribunal.Quorum)
		t.Cleanup(func() { adminController.db.TribunalStore.DeleteTribunal("test-tribunal") })
	})

	t.Run("GET - unauthorized (no user)", func(t *testing.T) {
		ctx := context.Background()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tribunals", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("GET - forbidden (non-bootstrap user)", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, regularUser.ID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tribunals", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("GET - success", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tribunals", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("MethodNotAllowed - PUT", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/tribunals", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("MethodNotAllowed - DELETE", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tribunals", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleTribunals(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestAdminControllerHandleDeleteTribunal(t *testing.T) {
	adminController := setupTestAdminController(t)

	// Create a bootstrap user for testing
	bootstrapUser, err := adminController.userSvc.CreateBootstrapUser()
	require.NoError(t, err)
	require.NotNil(t, bootstrapUser)
	t.Cleanup(func() { adminController.db.DocStore.DocDelete("users", bootstrapUser.ID) })

	// Create a non-bootstrap user for testing
	regularUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { adminController.db.DocStore.DocDelete("users", regularUser.ID) })

	// Create a test signer
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "delete-tribunal-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = adminController.db.SignerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.db.SignerStore.DeleteTrustedSigner("delete-tribunal-member") })

	// Create a test tribunal
	policy := models.TribunalPolicy{
		ID:              "delete-test-tribunal",
		MemberAppIDs:    []string{"delete-tribunal-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = adminController.db.TribunalStore.AddTribunal(policy)
	require.NoError(t, err)

	t.Run("DELETE - unauthorized (no user)", func(t *testing.T) {
		ctx := context.Background()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tribunals/delete-test-tribunal", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteTribunal(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("DELETE - forbidden (non-bootstrap user)", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, regularUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tribunals/delete-test-tribunal", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteTribunal(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("DELETE - unauthorized + missing tribunal ID (authz precedence)", func(t *testing.T) {
		ctx := context.Background()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tribunals/", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteTribunal(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code, "authz should take precedence over path validation")
	})

	t.Run("DELETE - missing tribunal ID", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tribunals/", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteTribunal(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("DELETE - success", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tribunals/delete-test-tribunal", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteTribunal(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		// Verify the tribunal was deleted
		tribunal, err := adminController.db.TribunalStore.GetTribunal("delete-test-tribunal")
		require.NoError(t, err)
		assert.Nil(t, tribunal)
	})

	t.Run("DELETE - non-existent tribunal", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tribunals/non-existent", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteTribunal(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("MethodNotAllowed - GET", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tribunals/delete-test-tribunal", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteTribunal(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("MethodNotAllowed - POST", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, bootstrapUser.ID)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tribunals/delete-test-tribunal", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteTribunal(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}
