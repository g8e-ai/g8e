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
	"encoding/json"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyService_IssueDownloadKey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	userID := "user-123"
	orgID := "org-456"

	key, err := svc.IssueDownloadKey(userID, orgID)
	require.NoError(t, err)
	assert.NotEmpty(t, key)
	assert.True(t, strings.HasPrefix(key, apiKeyPrefix))
	assert.Len(t, key, len(apiKeyPrefix)+apiKeyLength*2) // prefix + hex encoded 32 bytes

	// Verify the key was stored in the database
	docID := key[:20] // first 20 chars as doc ID
	doc, err := db.DocGet("api_keys", docID)
	require.NoError(t, err)
	require.NotNil(t, doc)
	var userIDStr, orgIDStr string
	json.Unmarshal(doc.Data["user_id"], &userIDStr)
	json.Unmarshal(doc.Data["organization_id"], &orgIDStr)
	assert.Equal(t, userID, userIDStr)
	assert.Equal(t, orgID, orgIDStr)
}

func TestAPIKeyService_IssueDownloadKey_UpdatesUserG8EKey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	userID := "user-789"
	orgID := "org-101"

	// Create a user document first
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "testuser",
		"status":   "active",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	key, err := svc.IssueDownloadKey(userID, orgID)
	require.NoError(t, err)

	// Verify the user's g8e_key field was updated
	userDoc2, err := db.DocGet("users", userID)
	require.NoError(t, err)
	require.NotNil(t, userDoc2)
	var g8eKeyStr string
	json.Unmarshal(userDoc2.Data["g8e_key"], &g8eKeyStr)
	assert.Equal(t, key, g8eKeyStr)
}

func TestAPIKeyService_ValidateKey_ValidKey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	userID := "user-222"
	orgID := "org-333"

	key, err := svc.IssueDownloadKey(userID, orgID)
	require.NoError(t, err)

	doc, err := svc.ValidateKey(key)
	require.NoError(t, err)
	require.NotNil(t, doc)
	var userIDStr, orgIDStr string
	json.Unmarshal(doc.Data["user_id"], &userIDStr)
	json.Unmarshal(doc.Data["organization_id"], &orgIDStr)
	assert.Equal(t, userID, userIDStr)
	assert.Equal(t, orgID, orgIDStr)
}

func TestAPIKeyService_ValidateKey_InvalidFormat(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	// Test key without prefix
	_, err := svc.ValidateKey("invalid-key-without-prefix")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key format")
}

func TestAPIKeyService_ValidateKey_KeyNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	// Test non-existent key
	fakeKey := apiKeyPrefix + strings.Repeat("a", apiKeyLength*2)
	_, err := svc.ValidateKey(fakeKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key not found")
}

func TestAPIKeyService_ValidateKey_TerminatedKey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	userID := "user-444"
	orgID := "org-555"

	key, err := svc.IssueDownloadKey(userID, orgID)
	require.NoError(t, err)

	// Manually set the key status to terminated
	docID := key[:20]
	doc, err := db.DocGet("api_keys", docID)
	require.NoError(t, err)
	doc.Data["status"] = []byte(`"terminated"`)
	docBytes, err := json.Marshal(doc.Data)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("api_keys", docID, docBytes))

	// Validation should fail for terminated keys
	_, err = svc.ValidateKey(key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key is terminated")
}

func TestAPIKeyService_RevokeKey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	userID := "user-666"
	orgID := "org-777"

	key, err := svc.IssueDownloadKey(userID, orgID)
	require.NoError(t, err)

	// Revoke the key
	err = svc.RevokeKey(key)
	require.NoError(t, err)

	// Verify the key no longer exists
	docID := key[:20]
	doc, err := db.DocGet("api_keys", docID)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestAPIKeyService_generateRawKey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	key1, err := svc.generateRawKey()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(key1, apiKeyPrefix))
	assert.Len(t, key1, len(apiKeyPrefix)+apiKeyLength*2)

	// Verify keys are unique
	key2, err := svc.generateRawKey()
	require.NoError(t, err)
	assert.NotEqual(t, key1, key2)
}

func TestAPIKeyService_makeDocID(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewAPIKeyService(db, logger)

	key := apiKeyPrefix + strings.Repeat("a", apiKeyLength*2)
	docID := svc.makeDocID(key)

	// Doc ID should be the first 20 characters
	assert.Equal(t, key[:20], docID)
	assert.Len(t, docID, 20)
}
