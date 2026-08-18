// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistrationService(t *testing.T) {

	docStore := &DocumentStoreService{}
	kvStore := &KVStoreService{}
	pki := &PKIAuthority{}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	userSvc := &UserService{}
	cliSessionSvc := &CLISessionService{}
	operatorSessionSvc := &OperatorSessionService{}
	cfg := &config.GatewayConfig{}

	service := NewRegistrationService(docStore, kvStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

	assert.NotNil(t, service)
	assert.Equal(t, docStore, service.docStore)
	assert.Equal(t, kvStore, service.kvStore)
	assert.Equal(t, pki, service.pki)
	assert.Equal(t, logger, service.logger)
	assert.Equal(t, userSvc, service.userSvc)
	assert.Equal(t, cliSessionSvc, service.cliSessionSvc)
	assert.Equal(t, operatorSessionSvc, service.operatorSessionSvc)
	assert.Equal(t, cfg, service.cfg)
}

func TestPKIPhase2_CalculateSerialFromPEM(t *testing.T) {

	// Test that calculateSerialFromPEM correctly extracts serial from a certificate
	dataDir := testutil.TempDir(t)
	logger := testutil.NewTestLogger()
	fileSvc := newTestFileSvc(t)
	db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	sm := newTestSecretManager(t, db.db, fileSvc)

	pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
	err = pki.InitializePKI(nil)
	require.NoError(t, err)

	// Generate a CSR and sign it
	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
	require.NoError(t, err)

	// Extract serial using the helper function
	serial := calculateSerialFromPEM(certPEM)
	assert.NotEmpty(t, serial, "serial should be extracted from issued cert")

	// Verify serial matches the actual certificate serial
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	assert.Equal(t, cert.SerialNumber.String(), serial, "extracted serial should match certificate serial")
}

func TestSessionWebBindKey(t *testing.T) {

	tests := []struct {
		name      string
		sessionID string
		expected  string
	}{
		{"Valid session ID", "web-session-123", "g8e:sessions:web:web-session-123:bind"},
		{"Empty session ID", "", "g8e:sessions:web::bind"},
		{"Session with special chars", "session-abc-123", "g8e:sessions:web:session-abc-123:bind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sessionWebBindKey(tt.sessionID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSessionOperatorBindKey(t *testing.T) {

	tests := []struct {
		name      string
		sessionID string
		expected  string
	}{
		{"Valid session ID", "op-session-456", "g8e:sessions:operator:op-session-456:bind"},
		{"Empty session ID", "", "g8e:sessions:operator::bind"},
		{"Session with special chars", "operator-xyz-789", "g8e:sessions:operator:operator-xyz-789:bind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sessionOperatorBindKey(tt.sessionID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPKIPhase3_CLI_CSR_Mandatory verifies that enrollment without CLI CSR is rejected
// This is the fix for C5 (SPIFFE drift fallback) in the PKI cleanup plan.
// See: .local.dev/docs/plans/pki_cleanup.md C5
// Updated: CLI CSR is now optional for operator-only enrollment
func TestPKIPhase3_CLI_CSR_Optional(t *testing.T) {
	t.Run("RegisterDeviceCSR accepts enrollment without CLI CSR (operator-only)", func(t *testing.T) {

		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(stores.DocStore, logger)
		cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
		operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

		// Generate only Operator CSR, no CLI CSR
		opCSR := testutil.GenerateTestCSRP256(t, "test-operator")

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "test-fingerprint",
			Hostname:          "test-host",
			CSR:               opCSR,
			CLICSR:            "", // Empty CLI CSR is now allowed for operator-only enrollment
		}

		resp, err := regSvc.RegisterDeviceCSR("user-123", "org-123", req)
		require.NoError(t, err, "enrollment without CLI CSR should succeed for operator-only")
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.OperatorID)
		assert.NotEmpty(t, resp.OperatorSessionID)
		assert.Empty(t, resp.CLISessionID, "CLI session ID should be empty for operator-only enrollment")
		assert.Empty(t, resp.CLICert, "CLI cert should be empty for operator-only enrollment")
	})
}

func TestRegistrationService_ListOperatorSlots(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	regSvc := infra.Reg

	t.Run("Empty user_id returns error", func(t *testing.T) {
		slots, err := regSvc.ListOperatorSlots("")
		assert.Error(t, err)
		assert.Nil(t, slots)
		assert.Contains(t, err.Error(), "user_id is required")
	})

	t.Run("Returns empty list for user with no slots", func(t *testing.T) {
		slots, err := regSvc.ListOperatorSlots("nonexistent-user")
		require.NoError(t, err)
		assert.NotNil(t, slots)
		assert.Empty(t, slots)
	})

	t.Run("Returns slots filtered by user_id and is_slot", func(t *testing.T) {
		// Create a slot for the user
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)
		require.NotNil(t, slot)

		slots, err := regSvc.ListOperatorSlots("user-123")
		require.NoError(t, err)
		assert.NotEmpty(t, slots)
		// Find the slot we just created (may not be first due to ordering)
		found := false
		for _, s := range slots {
			if s.ID == slot.ID {
				found = true
				assert.Equal(t, "user-123", s.UserID)
				assert.True(t, s.IsSlot)
				break
			}
		}
		assert.True(t, found, "created slot should be in results")
	})
}

func TestRegistrationService_TerminateOperator(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	regSvc := infra.Reg

	t.Run("Missing operator_id returns error", func(t *testing.T) {
		err := regSvc.TerminateOperator("", "user-123", "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operator_id is required")
	})

	t.Run("Missing user_id returns error", func(t *testing.T) {
		err := regSvc.TerminateOperator("op-123", "", "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user_id is required")
	})

	t.Run("Non-existent operator returns error", func(t *testing.T) {
		err := regSvc.TerminateOperator("nonexistent-op", "user-123", "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operator not found")
	})

	t.Run("Wrong owner returns error", func(t *testing.T) {
		// Create an operator for user-123
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		// Try to terminate with different user_id
		err = regSvc.TerminateOperator(slot.ID, "user-456", "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not belong to user")
	})

	t.Run("Already terminated operator returns nil", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		// First termination
		err = regSvc.TerminateOperator(slot.ID, "user-123", "test")
		require.NoError(t, err)

		// Second termination should be no-op
		err = regSvc.TerminateOperator(slot.ID, "user-123", "test")
		assert.NoError(t, err)
	})

	t.Run("Happy path terminates operator", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		err = regSvc.TerminateOperator(slot.ID, "user-123", "test reason")
		require.NoError(t, err)

		// Verify status was updated
		doc, err := infra.Stores.DocStore.DocGet("operators", slot.ID)
		require.NoError(t, err)
		require.NotNil(t, doc)

		var op models.OperatorDocumentGo
		b, _ := json.Marshal(doc.ForWire())
		_ = json.Unmarshal(b, &op)
		assert.Equal(t, constants.OperatorStatusTerminated, op.Status)
	})
}

func TestRegistrationService_ToOperatorDoc(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	regSvc := infra.Reg

	t.Run("Valid doc round-trips correctly", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		doc, err := infra.Stores.DocStore.DocGet("operators", slot.ID)
		require.NoError(t, err)

		op, err := regSvc.toOperatorDoc(doc)
		require.NoError(t, err)
		assert.Equal(t, slot.ID, op.ID)
		assert.Equal(t, "user-123", op.UserID)
	})

	t.Run("Malformed doc returns error", func(t *testing.T) {
		// Create a document with invalid JSON in a required field
		malformedDoc := &models.Document{
			ID: "malformed",
			Data: map[string]json.RawMessage{
				"id":      json.RawMessage(`"malformed"`),
				"user_id": json.RawMessage(`"user-123"`),
				"status":  json.RawMessage(`invalid-json`), // Invalid JSON
			},
		}

		_, err := regSvc.toOperatorDoc(malformedDoc)
		assert.Error(t, err)
	})
}

func TestRegistrationService_BindOperators(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	regSvc := infra.Reg

	t.Run("Missing web_session_id returns error", func(t *testing.T) {
		req := models.BindOperatorsRequest{
			UserID:      "user-123",
			OperatorIDs: []string{"op-123"},
		}
		resp, err := regSvc.BindOperators(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "web_session_id is required")
	})

	t.Run("Missing user_id returns error", func(t *testing.T) {
		req := models.BindOperatorsRequest{
			WebSessionID: "web-123",
			OperatorIDs:  []string{"op-123"},
		}
		resp, err := regSvc.BindOperators(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "user_id is required")
	})

	t.Run("Empty operator_ids returns error", func(t *testing.T) {
		req := models.BindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorIDs:  []string{},
		}
		resp, err := regSvc.BindOperators(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "operator_ids required")
	})

	t.Run("Non-existent operator fails", func(t *testing.T) {
		req := models.BindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorIDs:  []string{"nonexistent-op"},
		}
		resp, err := regSvc.BindOperators(req)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, 0, resp.BoundCount)
		assert.Equal(t, 1, resp.FailedCount)
	})

	t.Run("Wrong owner fails", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		req := models.BindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-456", // Different owner
			OperatorIDs:  []string{slot.ID},
		}
		resp, err := regSvc.BindOperators(req)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, 0, resp.BoundCount)
		assert.Equal(t, 1, resp.FailedCount)
	})

	t.Run("Operator with no active session fails", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		req := models.BindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorIDs:  []string{slot.ID},
		}
		resp, err := regSvc.BindOperators(req)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, 0, resp.BoundCount)
		assert.Equal(t, 1, resp.FailedCount)
		assert.Contains(t, resp.Error, "no active session")
	})

	t.Run("Happy path binds operator successfully", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		// Update the slot to have an active session
		update := map[string]interface{}{
			"operator_session_id": "session-123",
			"status":              string(constants.OperatorStatusActive),
		}
		updateBytes, err := json.Marshal(update)
		require.NoError(t, err)
		_, err = infra.Stores.DocStore.DocUpdate("operators", slot.ID, updateBytes)
		require.NoError(t, err)

		req := models.BindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorIDs:  []string{slot.ID},
		}
		resp, err := regSvc.BindOperators(req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, 1, resp.BoundCount)
		assert.Equal(t, 0, resp.FailedCount)
		assert.Contains(t, resp.BoundOperatorIDs, slot.ID)
	})

	t.Run("Multiple operators with mixed success", func(t *testing.T) {
		slot1, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)
		slot2, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		// Update slot1 to have an active session
		update := map[string]interface{}{
			"operator_session_id": "session-123",
			"status":              string(constants.OperatorStatusActive),
		}
		updateBytes, err := json.Marshal(update)
		require.NoError(t, err)
		_, err = infra.Stores.DocStore.DocUpdate("operators", slot1.ID, updateBytes)
		require.NoError(t, err)

		req := models.BindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorIDs:  []string{slot1.ID, slot2.ID, "nonexistent"},
		}
		resp, err := regSvc.BindOperators(req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, 1, resp.BoundCount)
		assert.Equal(t, 2, resp.FailedCount)
	})
}

func TestRegistrationService_UnbindOperators(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	regSvc := infra.Reg

	t.Run("Missing web_session_id returns error", func(t *testing.T) {
		req := models.UnbindOperatorsRequest{
			UserID:      "user-123",
			OperatorIDs: []string{"op-123"},
		}
		resp, err := regSvc.UnbindOperators(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "web_session_id is required")
	})

	t.Run("Missing user_id returns error", func(t *testing.T) {
		req := models.UnbindOperatorsRequest{
			WebSessionID: "web-123",
			OperatorIDs:  []string{"op-123"},
		}
		resp, err := regSvc.UnbindOperators(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "user_id is required")
	})

	t.Run("Non-existent operator fails", func(t *testing.T) {
		req := models.UnbindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorIDs:  []string{"nonexistent-op"},
		}
		resp, err := regSvc.UnbindOperators(req)
		require.NoError(t, err)
		assert.False(t, resp.Success) // Failed to unbind the operator
		assert.Equal(t, 0, resp.UnboundCount)
		assert.Equal(t, 1, resp.FailedCount)
	})

	t.Run("Wrong owner fails", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		req := models.UnbindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-456", // Different owner
			OperatorIDs:  []string{slot.ID},
		}
		resp, err := regSvc.UnbindOperators(req)
		require.NoError(t, err)
		assert.False(t, resp.Success)
		assert.Equal(t, 0, resp.UnboundCount)
		assert.Equal(t, 1, resp.FailedCount)
	})

	t.Run("Happy path unbinds operator successfully", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		// First bind the operator
		update := map[string]interface{}{
			"operator_session_id":  "session-123",
			"status":               string(constants.OperatorStatusActive),
			"bound_web_session_id": "web-123",
		}
		updateBytes, err := json.Marshal(update)
		require.NoError(t, err)
		_, err = infra.Stores.DocStore.DocUpdate("operators", slot.ID, updateBytes)
		require.NoError(t, err)

		// Create bound sessions document
		boundDoc := map[string]interface{}{
			"id":                   "web-123",
			"web_session_id":       "web-123",
			"user_id":              "user-123",
			"operator_session_ids": []string{"session-123"},
			"operator_ids":         []string{slot.ID},
			"bound_at":             time.Now().UTC().Format(time.RFC3339),
			"last_updated_at":      time.Now().UTC().Format(time.RFC3339),
			"status":               string(constants.OperatorStatusActive),
		}
		boundBytes, err := json.Marshal(boundDoc)
		require.NoError(t, err)
		err = infra.Stores.DocStore.DocSet("bound_sessions", "web-123", boundBytes)
		require.NoError(t, err)

		req := models.UnbindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorIDs:  []string{slot.ID},
		}
		resp, err := regSvc.UnbindOperators(req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, 1, resp.UnboundCount)
		assert.Equal(t, 0, resp.FailedCount)
		assert.Contains(t, resp.UnboundOperatorIDs, slot.ID)
	})

	t.Run("Multiple operators with mixed success", func(t *testing.T) {
		slot1, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)
		slot2, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		// Update slot1 to have an active session
		update := map[string]interface{}{
			"operator_session_id":  "session-123",
			"status":               string(constants.OperatorStatusActive),
			"bound_web_session_id": "web-123",
		}
		updateBytes, err := json.Marshal(update)
		require.NoError(t, err)
		_, err = infra.Stores.DocStore.DocUpdate("operators", slot1.ID, updateBytes)
		require.NoError(t, err)

		req := models.UnbindOperatorsRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorIDs:  []string{slot1.ID, slot2.ID, "nonexistent"},
		}
		resp, err := regSvc.UnbindOperators(req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		// UnbindOperators is lenient - it unbinds operators even if they weren't bound
		assert.Equal(t, 2, resp.UnboundCount)
		assert.Equal(t, 1, resp.FailedCount)
	})
}

func TestRegistrationService_SetTargetContext(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	regSvc := infra.Reg

	t.Run("Missing web_session_id returns error", func(t *testing.T) {
		req := models.SetTargetContextRequest{
			UserID:     "user-123",
			OperatorID: "op-123",
		}
		resp, err := regSvc.SetTargetContext(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "web_session_id is required")
	})

	t.Run("Missing user_id returns error", func(t *testing.T) {
		req := models.SetTargetContextRequest{
			WebSessionID: "web-123",
			OperatorID:   "op-123",
		}
		resp, err := regSvc.SetTargetContext(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "user_id is required")
	})

	t.Run("Non-existent operator returns error", func(t *testing.T) {
		req := models.SetTargetContextRequest{
			WebSessionID: "web-123",
			UserID:       "user-123",
			OperatorID:   "nonexistent-op",
		}
		resp, err := regSvc.SetTargetContext(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Wrong owner returns error", func(t *testing.T) {
		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		req := models.SetTargetContextRequest{
			WebSessionID: "web-123",
			UserID:       "user-456", // Different owner
			OperatorID:   slot.ID,
		}
		resp, err := regSvc.SetTargetContext(req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "does not belong to user")
	})

	// Skip happy path test - requires creating an active operator session which is complex
	// The validation guards are already tested above
}

func TestRegistrationService_SetTargetContext_HappyPath(t *testing.T) {

	infra := setupTestInfrastructure(t, false)
	regSvc := infra.Reg

	// Create a slot
	slot, err := regSvc.createSlot("user-123", "org-123")
	require.NoError(t, err)

	// Update the slot to have an active session so it exists and can be retrieved
	update := map[string]interface{}{
		"operator_session_id": "session-123",
		"status":              string(constants.OperatorStatusActive),
		"user_id":             "user-123",
	}
	updateBytes, err := json.Marshal(update)
	require.NoError(t, err)
	_, err = infra.Stores.DocStore.DocUpdate("operators", slot.ID, updateBytes)
	require.NoError(t, err)

	req := models.SetTargetContextRequest{
		WebSessionID: "web-123",
		UserID:       "user-123",
		OperatorID:   slot.ID,
	}

	res, err := regSvc.SetTargetContext(req)
	require.NoError(t, err)
	assert.NotNil(t, res)
}

func TestRegistrationService_RegisterDeviceCSR(t *testing.T) {

	t.Run("Missing system_fingerprint returns error", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(stores.DocStore, logger)
		cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
		operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

		opCSR := testutil.GenerateTestCSRP256(t, "test-operator")
		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "", // Missing
			Hostname:          "test-host",
			CSR:               opCSR,
		}

		_, err = regSvc.RegisterDeviceCSR("user-123", "org-123", req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "system_fingerprint is required")
	})

	t.Run("Missing user_id returns error", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(stores.DocStore, logger)
		cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
		operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

		opCSR := testutil.GenerateTestCSRP256(t, "test-operator")
		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "test-fingerprint",
			Hostname:          "test-host",
			CSR:               opCSR,
		}

		_, err = regSvc.RegisterDeviceCSR("", "org-123", req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user_id is required")
	})

	t.Run("Missing CSR returns error", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(stores.DocStore, logger)
		cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
		operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "test-fingerprint",
			Hostname:          "test-host",
			CSR:               "", // Missing
		}

		_, err = regSvc.RegisterDeviceCSR("user-123", "org-123", req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operator CSR is required")
	})

	t.Run("Invalid system_fingerprint returns error", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(stores.DocStore, logger)
		cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
		operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

		opCSR := testutil.GenerateTestCSRP256(t, "test-operator")
		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "   ", // Only whitespace
			Hostname:          "test-host",
			CSR:               opCSR,
		}

		_, err = regSvc.RegisterDeviceCSR("user-123", "org-123", req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid system_fingerprint")
	})
}

func TestRegistrationService_CompleteRegistration(t *testing.T) {

	t.Run("Invalid CSR PEM format returns error", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(stores.DocStore, logger)
		cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
		operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "test-fingerprint",
			Hostname:          "test-host",
			CSR:               "invalid-pem-data",
		}

		_, err = regSvc.completeRegistration(slot, "user-123", "org-123", req, "test-fingerprint")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid CSR PEM format")
	})

	t.Run("Wrong CSR block type returns error", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(stores.DocStore, logger)
		cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
		operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		// Generate a real CSR, then change the block type to CERTIFICATE (wrong type)
		csr := testutil.GenerateTestCSRP256(t, "test-operator")
		wrongTypePEM := strings.Replace(csr, "CERTIFICATE REQUEST", "CERTIFICATE", 1)

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "test-fingerprint",
			Hostname:          "test-host",
			CSR:               wrongTypePEM, // Wrong block type
		}

		_, err = regSvc.completeRegistration(slot, "user-123", "org-123", req, "test-fingerprint")
		assert.Error(t, err)
		// The error could be either decode failure or wrong block type depending on PEM parsing
		assert.Contains(t, err.Error(), "invalid CSR PEM format")
	})

	t.Run("Missing CSR returns error", func(t *testing.T) {
		dataDir := testutil.TempDir(t)
		logger := testutil.NewTestLogger()
		fileSvc := newTestFileSvc(t)
		db, stores, err := openTestDB(t, dataDir, fileSvc, logger)
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		sm := newTestSecretManager(t, db.db, fileSvc)

		pki := newPKIAuthority(fileSvc, stores.DocStore, sm, logger)
		err = pki.InitializePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(stores.DocStore, logger)
		cliSessionSvc := NewCLISessionService(stores.DocStore, logger)
		operatorSessionSvc := NewOperatorSessionService(stores.DocStore, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(stores.DocStore, stores.KVStore, pki, logger, userSvc, cliSessionSvc, operatorSessionSvc, cfg)

		slot, err := regSvc.createSlot("user-123", "org-123")
		require.NoError(t, err)

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "test-fingerprint",
			Hostname:          "test-host",
			CSR:               "", // Empty CSR
		}

		_, err = regSvc.completeRegistration(slot, "user-123", "org-123", req, "test-fingerprint")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CSR required")
	})
}
