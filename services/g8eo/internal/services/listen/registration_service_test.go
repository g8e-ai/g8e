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

package listen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/marshaler"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
)

func generateTestCSR(t *testing.T) string {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: "test-operator",
		},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	require.NoError(t, err)

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})
	return string(csrPEM)
}

func TestRegistrationService_RegisterDevice(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	defer db.Close()

	pki := newPKIAuthority(dbDir, secretsDir, db, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc)

	token := "dlk_test_token_12345678901234567890"
	userID := "user-1"
	orgID := "org-1"

	t.Run("Success - Multi-use link, new slot", func(t *testing.T) {
		// Setup link in KV
		linkData := models.DeviceLinkData{
			Token:          token,
			UserID:         userID,
			OrganizationID: orgID,
			MaxUses:        5,
			Status:         "active",
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		err := db.KVSet("g8e:device-link:"+token, string(linkBytes), 3600)
		require.NoError(t, err)

		req := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fingerprint-1",
			Hostname:          "host-1",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bob",
		}

		resp, err := reg.RegisterDevice(token, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.OperatorID)
		assert.NotEmpty(t, resp.OperatorSessionID)

		// Verify operator doc was created
		doc, err := db.DocGet("operators", resp.OperatorID)
		require.NoError(t, err)
		require.NotNil(t, doc)
		assert.Equal(t, userID, docFieldString(t, doc, "user_id"))
		assert.Equal(t, constants.Status.OperatorStatus.Active, docFieldString(t, doc, "status"))
	})

	t.Run("Success - Single-operator link", func(t *testing.T) {
		opID := "op-single-1"
		// Create operator slot first
		op := &models.OperatorDocumentGo{
			ID:             opID,
			UserID:         userID,
			OrganizationID: orgID,
			Component:      "g8eo",
			Status:         constants.Status.OperatorStatus.Offline,
			OperatorType:   constants.Status.OperatorType.System,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		opBytes, _ := json.Marshal(op)
		err := db.DocSet("operators", opID, opBytes)
		require.NoError(t, err)

		token2 := "dlk_single_token_123"
		linkData := models.DeviceLinkData{
			Token:          token2,
			UserID:         userID,
			OrganizationID: orgID,
			OperatorID:     opID,
			MaxUses:        1,
			Status:         "active",
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		err = db.KVSet("g8e:device-link:"+token2, string(linkBytes), 3600)
		require.NoError(t, err)

		req := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fingerprint-2",
			Hostname:          "host-2",
		}

		resp, err := reg.RegisterDevice(token2, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, opID, resp.OperatorID)

		// Verify status update
		doc, _ := db.DocGet("operators", opID)
		assert.Equal(t, constants.Status.OperatorStatus.Active, docFieldString(t, doc, "status"))
	})

	t.Run("Failure - Link not found", func(t *testing.T) {
		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "fp",
		}
		resp, err := reg.RegisterDevice("nonexistent", req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Failure - Link expired", func(t *testing.T) {
		token3 := "dlk_expired"
		linkData := models.DeviceLinkData{
			Token:     token3,
			UserID:    userID,
			Status:    "active",
			ExpiresAt: time.Now().Add(-1 * time.Minute),
		}
		linkBytes, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token3, string(linkBytes), 3600)

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "fp",
		}
		resp, err := reg.RegisterDevice(token3, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "expired")
	})

	t.Run("Failure - Link revoked", func(t *testing.T) {
		token4 := "dlk_revoked"
		linkData := models.DeviceLinkData{
			Token:     token4,
			UserID:    userID,
			Status:    deviceLinkStatusRevoked,
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token4, string(linkBytes), 3600)

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "fp",
		}
		resp, err := reg.RegisterDevice(token4, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "revoked")
	})

	t.Run("Failure - Link exhausted", func(t *testing.T) {
		token5 := "dlk_exhausted"
		linkData := models.DeviceLinkData{
			Token:     token5,
			UserID:    userID,
			Status:    deviceLinkStatusExhausted,
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token5, string(linkBytes), 3600)

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "fp",
		}
		resp, err := reg.RegisterDevice(token5, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "exhausted")
	})

	t.Run("Failure - Missing system fingerprint", func(t *testing.T) {
		token6 := "dlk_no_fp"
		linkData := models.DeviceLinkData{
			Token:     token6,
			UserID:    userID,
			Status:    "active",
			ExpiresAt: time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token6, string(linkBytes), 3600)

		req := models.OperatorRegistrationRequest{
			CSR: generateTestCSR(t),
		}
		resp, err := reg.RegisterDevice(token6, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "system_fingerprint is required")
	})

	t.Run("Failure - Missing CSR", func(t *testing.T) {
		token7 := "dlk_no_csr"
		linkData := models.DeviceLinkData{
			Token:          token7,
			UserID:         userID,
			OrganizationID: orgID,
			MaxUses:        5,
			Status:         "active",
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		db.KVSet("g8e:device-link:"+token7, string(linkBytes), 3600)

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "fp-1",
		}
		resp, err := reg.RegisterDevice(token7, req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "CSR required")
	})

	t.Run("Success - Multi-use link with claim tracking", func(t *testing.T) {
		token8 := "dlk_multi_claim"
		linkData := models.DeviceLinkData{
			Token:          token8,
			UserID:         userID,
			OrganizationID: orgID,
			MaxUses:        3,
			Status:         "active",
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		err := db.KVSet("g8e:device-link:"+token8, string(linkBytes), 3600)
		require.NoError(t, err)

		req := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-multi-1",
			Hostname:          "host-multi-1",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bob",
		}

		resp, err := reg.RegisterDevice(token8, req)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		// Verify claim was added to linkData
		raw, found := db.KVGet("g8e:device-link:" + token8)
		require.True(t, found)
		var updatedLink models.DeviceLinkData
		require.NoError(t, json.Unmarshal([]byte(raw), &updatedLink))
		assert.Len(t, updatedLink.Claims, 1)
		assert.Equal(t, "fp-multi-1", updatedLink.Claims[0].SystemFingerprint)
		assert.Equal(t, resp.OperatorID, updatedLink.Claims[0].OperatorID)
		assert.Equal(t, 1, updatedLink.Uses)
	})

	t.Run("Success - Device re-registration reuses claim", func(t *testing.T) {
		token9 := "dlk_reuse"
		linkData := models.DeviceLinkData{
			Token:          token9,
			UserID:         userID,
			OrganizationID: orgID,
			MaxUses:        3,
			Status:         "active",
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		err := db.KVSet("g8e:device-link:"+token9, string(linkBytes), 3600)
		require.NoError(t, err)

		req := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-reuse",
			Hostname:          "host-reuse",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bob",
		}

		// First registration
		resp1, err := reg.RegisterDevice(token9, req)
		require.NoError(t, err)
		assert.True(t, resp1.Success)

		// Second registration with same fingerprint - should reuse same operator
		req2 := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-reuse",
			Hostname:          "host-reuse",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bob",
		}
		resp2, err := reg.RegisterDevice(token9, req2)
		require.NoError(t, err)
		assert.True(t, resp2.Success)
		assert.Equal(t, resp1.OperatorID, resp2.OperatorID)

		// Verify only 1 claim in linkData
		raw, found := db.KVGet("g8e:device-link:" + token9)
		require.True(t, found)
		var updatedLink models.DeviceLinkData
		require.NoError(t, json.Unmarshal([]byte(raw), &updatedLink))
		assert.Len(t, updatedLink.Claims, 1)
	})

	t.Run("Failure - Fingerprint dedup prevents double registration", func(t *testing.T) {
		token10 := "dlk_dedup"
		linkData := models.DeviceLinkData{
			Token:          token10,
			UserID:         userID,
			OrganizationID: orgID,
			MaxUses:        2,
			Status:         "active",
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		err := db.KVSet("g8e:device-link:"+token10, string(linkBytes), 3600)
		require.NoError(t, err)

		req := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-dedup",
			Hostname:          "host-dedup",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bob",
		}

		// First registration succeeds
		resp1, err := reg.RegisterDevice(token10, req)
		require.NoError(t, err)
		assert.True(t, resp1.Success)

		// Manually remove claim to test fingerprint set dedup
		raw, found := db.KVGet("g8e:device-link:" + token10)
		require.True(t, found)
		var link models.DeviceLinkData
		require.NoError(t, json.Unmarshal([]byte(raw), &link))
		link.Claims = []models.DeviceLinkClaim{}
		link.Uses = 0
		linkBytes2, _ := json.Marshal(link)
		db.KVSet("g8e:device-link:"+token10, string(linkBytes2), 3600)

		// Second registration with same fingerprint should fail due to fingerprint set
		req2 := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-dedup",
			Hostname:          "host-dedup-2",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bob",
		}
		resp2, err := reg.RegisterDevice(token10, req2)
		assert.Error(t, err)
		assert.Nil(t, resp2)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("Failure - Max uses enforced", func(t *testing.T) {
		token11 := "dlk_maxuses"
		linkData := models.DeviceLinkData{
			Token:          token11,
			UserID:         userID,
			OrganizationID: orgID,
			MaxUses:        1,
			Status:         "active",
			CreatedAt:      time.Now(),
			ExpiresAt:      time.Now().Add(1 * time.Hour),
		}
		linkBytes, _ := json.Marshal(linkData)
		err := db.KVSet("g8e:device-link:"+token11, string(linkBytes), 3600)
		require.NoError(t, err)

		req := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-max1",
			Hostname:          "host-max1",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bob",
		}

		// First registration succeeds
		resp1, err := reg.RegisterDevice(token11, req)
		require.NoError(t, err)
		assert.True(t, resp1.Success)

		// Second registration with different fingerprint should fail due to max uses
		req2 := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-max2",
			Hostname:          "host-max2",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bob",
		}
		resp2, err := reg.RegisterDevice(token11, req2)
		assert.Error(t, err)
		assert.Nil(t, resp2)
		assert.Contains(t, err.Error(), "exhausted")
	})
}

func TestRegistrationService_DeviceLinks(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	defer db.Close()

	pki := newPKIAuthority(dbDir, secretsDir, db, logger)
	userSvc := NewUserService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc)

	resp, err := reg.CreateDeviceLink(models.CreateDeviceLinkRequest{
		UserID:         "user-1",
		OrganizationID: "org-1",
		Name:           "fleet",
		MaxUses:        2,
		TTLSeconds:     3600,
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.True(t, isValidDeviceLinkToken(resp.Token))
	assert.Equal(t, "g8e.operator --device-token "+resp.Token, resp.OperatorCommand)

	links, err := reg.ListDeviceLinks("user-1")
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, resp.Token, links[0].Token)
	assert.Equal(t, "fleet", links[0].Name)
	assert.Equal(t, deviceLinkStatusActive, links[0].Status)

	require.NoError(t, reg.DeleteDeviceLink(resp.Token, "user-1"))
	raw, found := db.KVGet(deviceLinkKey(resp.Token))
	require.True(t, found)
	var link models.DeviceLinkData
	require.NoError(t, json.Unmarshal([]byte(raw), &link))
	assert.Equal(t, deviceLinkStatusRevoked, link.Status)
	assert.NotNil(t, link.RevokedAt)
}

func TestRegistrationService_CreateDeviceLinkRejectsWrongOperatorOwner(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	defer db.Close()

	pki := newPKIAuthority(dbDir, secretsDir, db, logger)
	userSvc := NewUserService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc)
	op := &models.OperatorDocumentGo{
		ID:        "op-1",
		UserID:    "other-user",
		Component: "g8eo",
		Status:    constants.Status.OperatorStatus.Offline,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	opBytes, _ := json.Marshal(op)
	require.NoError(t, db.DocSet("operators", op.ID, opBytes))

	resp, err := reg.CreateDeviceLink(models.CreateDeviceLinkRequest{
		UserID:     "user-1",
		OperatorID: op.ID,
	})
	require.Error(t, err)
	require.Nil(t, resp)
	assert.Contains(t, err.Error(), "does not belong to user")
}

// mustDocJSON marshals a map to json.RawMessage for use with DocSet/DocUpdate.
// Moved to listen_db_test.go as a shared package-level helper for the listen package.
// func mustDocJSON(t *testing.T, v interface{}) json.RawMessage { ... }

func docFieldString(t *testing.T, doc *models.Document, field string) string {
	t.Helper()
	raw, ok := doc.Data[field]
	if !ok {
		return ""
	}
	var v string
	require.NoError(t, json.Unmarshal(raw, &v))
	return v
}

func TestRegistrationService_RotateOperatorAPIKey(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	defer db.Close()

	pki := newPKIAuthority(dbDir, secretsDir, db, logger)
	userSvc := NewUserService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc)

	userID := "user-1"
	opID := "op-1"
	oldKey := "g8e_old_key_123"

	// Create operator slot
	op := &models.OperatorDocumentGo{
		ID:             opID,
		UserID:         userID,
		OperatorAPIKey: oldKey,
		Status:         constants.Status.OperatorStatus.Offline,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	opBytes, _ := json.Marshal(op)
	require.NoError(t, db.DocSet("operators", opID, opBytes))

	t.Run("Success", func(t *testing.T) {
		require.NoError(t, reg.RotateOperatorAPIKey(opID, userID))

		doc, err := db.DocGet("operators", opID)
		require.NoError(t, err)
		newKey := docFieldString(t, doc, "operator_api_key")
		assert.NotEmpty(t, newKey)
		assert.NotEqual(t, oldKey, newKey)
		assert.Contains(t, newKey, "g8e_op-1_")
	})

	t.Run("Failure - Wrong user", func(t *testing.T) {
		err := reg.RotateOperatorAPIKey(opID, "wrong-user")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not belong to user")
	})

	t.Run("Failure - Not found", func(t *testing.T) {
		err := reg.RotateOperatorAPIKey("nonexistent", userID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRegistrationService_ListOperatorSlots(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	defer db.Close()

	pki := newPKIAuthority(dbDir, secretsDir, db, logger)
	userSvc := NewUserService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc)

	userID := "user-1"

	// Create 2 slots and 1 non-slot
	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("op-%d", i)
		op := &models.OperatorDocumentGo{
			ID:         id,
			UserID:     userID,
			IsSlot:     true,
			SlotNumber: i,
			Status:     constants.Status.OperatorStatus.Offline,
		}
		opBytes, _ := json.Marshal(op)
		db.DocSet("operators", id, opBytes)
	}

	// Non-slot
	db.DocSet("operators", "non-slot", json.RawMessage(`{"user_id": "user-1", "is_slot": false}`))

	t.Run("Success", func(t *testing.T) {
		slots, err := reg.ListOperatorSlots(userID)
		require.NoError(t, err)
		assert.Len(t, slots, 2)
		assert.Equal(t, 1, slots[0].SlotNumber)
		assert.Equal(t, 2, slots[1].SlotNumber)
	})

	t.Run("Empty for other user", func(t *testing.T) {
		slots, err := reg.ListOperatorSlots("other-user")
		require.NoError(t, err)
		assert.Empty(t, slots)
	})
}

func TestRegistrationService_TerminateOperator(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	defer db.Close()

	pki := newPKIAuthority(dbDir, secretsDir, db, logger)
	userSvc := NewUserService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc)

	userID := "user-1"
	opID := "op-terminate-1"

	// Create operator slot
	op := &models.OperatorDocumentGo{
		ID:             opID,
		UserID:         userID,
		OperatorAPIKey: "g8e_old_key",
		Status:         constants.Status.OperatorStatus.Active,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	opBytes, _ := json.Marshal(op)
	require.NoError(t, db.DocSet("operators", opID, opBytes))

	t.Run("Success", func(t *testing.T) {
		err := reg.TerminateOperator(opID, userID, "test termination")
		require.NoError(t, err)

		// Verify status updated
		doc, err := db.DocGet("operators", opID)
		require.NoError(t, err)
		assert.Equal(t, constants.Status.OperatorStatus.Terminated, docFieldString(t, doc, "status"))
		assert.Equal(t, "test termination", docFieldString(t, doc, "termination_reason"))
	})

	t.Run("Success - Already terminated", func(t *testing.T) {
		// First termination
		err := reg.TerminateOperator(opID, userID, "first termination")
		require.NoError(t, err)

		// Second termination should be idempotent
		err = reg.TerminateOperator(opID, userID, "second termination")
		require.NoError(t, err)

		doc, _ := db.DocGet("operators", opID)
		assert.Equal(t, constants.Status.OperatorStatus.Terminated, docFieldString(t, doc, "status"))
	})

	t.Run("Failure - Wrong user", func(t *testing.T) {
		// Reset operator to active for this test
		update := map[string]interface{}{"status": constants.Status.OperatorStatus.Active}
		updateBytes, _ := json.Marshal(update)
		db.DocUpdate("operators", opID, updateBytes)

		err := reg.TerminateOperator(opID, "wrong-user", "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not belong to user")
	})

	t.Run("Failure - Not found", func(t *testing.T) {
		err := reg.TerminateOperator("nonexistent", userID, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Failure - Missing operator_id", func(t *testing.T) {
		err := reg.TerminateOperator("", userID, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "operator_id is required")
	})

	t.Run("Failure - Missing user_id", func(t *testing.T) {
		err := reg.TerminateOperator(opID, "", "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user_id is required")
	})
}

func TestRegistrationService_Binding(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := NewListenDBService(dbDir, secretsDir, logger)
	require.NoError(t, err)
	defer db.Close()

	pki := newPKIAuthority(dbDir, secretsDir, db, logger)
	userSvc := NewUserService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc)

	userID := "user-1"
	sessionID := "sess-1"
	opID := "op-bind-1"
	opSessID := "op-sess-1"

	// Create operator slot with active session
	op := &models.OperatorDocumentGo{
		ID:                opID,
		UserID:            userID,
		OperatorSessionID: opSessID,
		Status:            constants.Status.OperatorStatus.Active,
	}
	opBytes, _ := json.Marshal(op)
	require.NoError(t, db.DocSet("operators", opID, opBytes))

	t.Run("BindOperators", func(t *testing.T) {
		req := models.BindOperatorsRequest{
			OperatorIDs:  []string{opID},
			UserID:       userID,
			WebSessionID: sessionID,
		}
		resp, err := reg.BindOperators(req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, 1, resp.BoundCount)
		assert.Equal(t, opID, resp.BoundOperatorIDs[0])

		// Verify KV binding
		val, found := db.KVGet(sessionOperatorBindKey(opSessID))
		assert.True(t, found)
		assert.Equal(t, sessionID, val)

		// Verify Web bind (SET)
		val, found = db.KVGet(sessionWebBindKey(sessionID))
		assert.True(t, found)
		var sids []string
		json.Unmarshal([]byte(val), &sids)
		assert.Contains(t, sids, opSessID)

		// Verify durability document
		doc, err := db.DocGet(marshaler.CollectionName(constants.CollectionBoundSessions), sessionID)
		require.NoError(t, err)
		require.NotNil(t, doc)
		assert.Equal(t, sessionID, docFieldString(t, doc, "web_session_id"))

		// Verify operator document updated
		opDoc, _ := db.DocGet("operators", opID)
		assert.Equal(t, sessionID, docFieldString(t, opDoc, "bound_web_session_id"))
	})

	t.Run("SetTargetContext", func(t *testing.T) {
		req := models.SetTargetContextRequest{
			OperatorID:   opID,
			UserID:       userID,
			WebSessionID: sessionID,
		}
		resp, err := reg.SetTargetContext(req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, opID, resp.OperatorID)
	})

	t.Run("UnbindOperators", func(t *testing.T) {
		req := models.UnbindOperatorsRequest{
			OperatorIDs:  []string{opID},
			UserID:       userID,
			WebSessionID: sessionID,
		}
		resp, err := reg.UnbindOperators(req)
		require.NoError(t, err)
		assert.True(t, resp.Success)
		assert.Equal(t, 1, resp.UnboundCount)

		// Verify KV unbinding
		_, found := db.KVGet(sessionOperatorBindKey(opSessID))
		assert.False(t, found)

		_, found = db.KVGet(sessionWebBindKey(sessionID))
		assert.False(t, found)

		// Verify operator document updated
		opDoc, _ := db.DocGet("operators", opID)
		assert.Equal(t, "", docFieldString(t, opDoc, "bound_web_session_id"))
	})
}
