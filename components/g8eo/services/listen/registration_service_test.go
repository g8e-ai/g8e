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
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
)

func TestRegistrationService_RegisterDevice(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	sslDir := t.TempDir()
	db, err := NewListenDBService(dbDir, sslDir, logger)
	require.NoError(t, err)
	defer db.Close()

	certs := newCertStore(dbDir, sslDir, logger)
	reg := NewRegistrationService(db, certs, logger)

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
		assert.NotEmpty(t, resp.APIKey)

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
