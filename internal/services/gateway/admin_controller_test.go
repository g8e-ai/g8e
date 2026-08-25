// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func setupTestAdminController(t *testing.T) *AdminController {
	t.Helper()
	infra := setupTestInfrastructure(t, false)
	return newAdminController(AdminControllerDeps{Cfg: infra.Cfg, Logger: infra.Logger, DocStore: infra.Stores.DocStore, SignerStore: infra.Stores.SignerStore, ConsensusStore: infra.Stores.ConsensusStore, UserSvc: infra.UserSvc, Responder: infra.Responder})
}

func TestAdminControllerHandleAppPolicySigner(t *testing.T) {
	adminController := setupTestAdminController(t)

	// The first user created is the gateway admin (IsFirstUser).
	adminUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, adminUser)
	t.Cleanup(func() { adminController.docStore.DocDelete("users", adminUser.ID) })

	// A second user is a non-admin regular user.
	regularUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { adminController.docStore.DocDelete("users", regularUser.ID) })

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
	err = adminController.docStore.DocSet("app_policies", "test-app-id", policyDoc)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.docStore.DocDelete("app_policies", "test-app-id") })

	// Generate a valid Ed25519 public key for testing
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	validPubKeyHex := hex.EncodeToString(pubKey)

	t.Run("MethodNotAllowed - GET", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/app-policies/test-app-id/signers", nil)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("BadRequest - missing app_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies//signers", nil)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Unauthorized - no user context", func(t *testing.T) {
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Unauthorized - empty user_id", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "")
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Forbidden - non-admin (non-first) user", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, regularUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("Forbidden - app policy not found", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/nonexistent-app/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("BadRequest - invalid JSON", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{invalid json}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - missing public_key_hex", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - empty public_key_hex", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{"public_key_hex": ""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - invalid hex public key", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{"public_key_hex": "not-hex"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - invalid public key size", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		shortKey := hex.EncodeToString([]byte{0x01, 0x02})
		body := []byte(`{"public_key_hex": "` + shortKey + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Success - valid signer registration", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{"public_key_hex": "` + validPubKeyHex + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/app-policies/test-app-id/signers", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleAppPolicySigner(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)

		// Verify the signer was added to the database
		signerDoc, err := adminController.docStore.DocGet("trusted_signers", "test-app-id")
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

func TestAdminControllerHandleConsensus(t *testing.T) {
	adminController := setupTestAdminController(t)

	// The first user created is the gateway admin (IsFirstUser).
	adminUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, adminUser)
	t.Cleanup(func() { adminController.docStore.DocDelete("users", adminUser.ID) })

	// A second user is a non-admin regular user.
	regularUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { adminController.docStore.DocDelete("users", regularUser.ID) })

	// Create test signers
	pubKey1, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pubKey2, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	signer1 := models.TrustedSigner{
		ID:        "consensus-member-1",
		PublicKey: hex.EncodeToString(pubKey1),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	signer2 := models.TrustedSigner{
		ID:        "consensus-member-2",
		PublicKey: hex.EncodeToString(pubKey2),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}

	err = adminController.signerStore.AddTrustedSigner(signer1)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.signerStore.DeleteTrustedSigner("consensus-member-1") })

	err = adminController.signerStore.AddTrustedSigner(signer2)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.signerStore.DeleteTrustedSigner("consensus-member-2") })

	t.Run("POST - unauthorized (no user)", func(t *testing.T) {
		ctx := context.Background()
		body := []byte(`{"id":"test-consensus","member_app_ids":["consensus-member-1"],"quorum":1,"require_distinct":true,"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/consensus", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("POST - forbidden (non-admin user)", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, regularUser.ID)
		body := []byte(`{"id":"test-consensus","member_app_ids":["consensus-member-1"],"quorum":1,"require_distinct":true,"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/consensus", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("POST - invalid JSON", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{invalid json}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/consensus", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("POST - validation failure (quorum > member count)", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{"id":"test-consensus","member_app_ids":["consensus-member-1"],"quorum":2,"require_distinct":true,"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/consensus", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("POST - success", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		body := []byte(`{"id":"test-consensus","member_app_ids":["consensus-member-1","consensus-member-2"],"quorum":2,"require_distinct":true,"enabled":true}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/consensus", bytes.NewReader(body)).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)

		// Verify the consensus was created
		consensus, err := adminController.consensusStore.GetConsensus("test-consensus")
		require.NoError(t, err)
		require.NotNil(t, consensus)
		assert.Equal(t, "test-consensus", consensus.ID)
		assert.Equal(t, 2, consensus.Quorum)
		t.Cleanup(func() { adminController.consensusStore.DeleteConsensus("test-consensus") })
	})

	t.Run("GET - unauthorized (no user)", func(t *testing.T) {
		ctx := context.Background()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("GET - forbidden (non-admin user)", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, regularUser.ID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("GET - success", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("MethodNotAllowed - PUT", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("MethodNotAllowed - DELETE", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleConsensus(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestAdminControllerHandleDeleteConsensus(t *testing.T) {
	adminController := setupTestAdminController(t)

	// The first user created is the gateway admin (IsFirstUser).
	adminUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, adminUser)
	t.Cleanup(func() { adminController.docStore.DocDelete("users", adminUser.ID) })

	// A second user is a non-admin regular user.
	regularUser, err := adminController.userSvc.CreateUser()
	require.NoError(t, err)
	require.NotNil(t, regularUser)
	t.Cleanup(func() { adminController.docStore.DocDelete("users", regularUser.ID) })

	// Create a test signer
	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := models.TrustedSigner{
		ID:        "delete-consensus-member",
		PublicKey: hex.EncodeToString(pubKey),
		AddedAt:   time.Now().UTC(),
		Enabled:   true,
	}
	err = adminController.signerStore.AddTrustedSigner(signer)
	require.NoError(t, err)
	t.Cleanup(func() { adminController.signerStore.DeleteTrustedSigner("delete-consensus-member") })

	// Create a test consensus
	policy := models.ConsensusPolicy{
		ID:              "delete-test-consensus",
		MemberAppIDs:    []string{"delete-consensus-member"},
		Quorum:          1,
		RequireDistinct: true,
		Enabled:         true,
	}
	err = adminController.consensusStore.AddConsensus(policy)
	require.NoError(t, err)

	t.Run("DELETE - unauthorized (no user)", func(t *testing.T) {
		ctx := context.Background()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/consensus/delete-test-consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteConsensus(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("DELETE - forbidden (non-admin user)", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, regularUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/consensus/delete-test-consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteConsensus(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("DELETE - unauthorized + missing consensus ID (authz precedence)", func(t *testing.T) {
		ctx := context.Background()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/consensus/", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteConsensus(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code, "authz should take precedence over path validation")
	})

	t.Run("DELETE - missing consensus ID", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/consensus/", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteConsensus(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("DELETE - success", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/consensus/delete-test-consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteConsensus(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		// Verify the consensus was deleted
		consensus, err := adminController.consensusStore.GetConsensus("delete-test-consensus")
		require.NoError(t, err)
		assert.Nil(t, consensus)
	})

	t.Run("DELETE - non-existent consensus", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/consensus/non-existent", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteConsensus(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("MethodNotAllowed - GET", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/consensus/delete-test-consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteConsensus(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("MethodNotAllowed - POST", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, adminUser.ID)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/consensus/delete-test-consensus", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		adminController.handleDeleteConsensus(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}
