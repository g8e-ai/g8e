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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthService_ValidateOperatorSession_MissingSessionID(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "")

	_, err := auth.ValidateOperatorSession("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing operator_session_id")
}

func TestAuthService_ValidateOperatorSession_SessionNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "")

	_, err := auth.ValidateOperatorSession("nonexistent-session")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired operator session")
}

func TestAuthService_ValidateOperatorSession_TerminatedStatus(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "")

	// Create an operator session with terminated status
	sessionID := "terminated-session"
	opDoc := map[string]interface{}{
		"id":                  "op-123",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusTerminated),
		"user_id":             "user-123",
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("operators", "op-123", opBytes))

	_, err = auth.ValidateOperatorSession(sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operator identity disabled")
}

func TestAuthService_ValidateOperatorSession_SessionExpired(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "")

	// Create an active user
	userID := "user-456"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "expired-user",
		"status":   "active",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Create an operator session with old timestamp using the test hook
	sessionID := "expired-session"
	oldTime := time.Now().UTC().Add(-25 * time.Hour)
	opDoc := map[string]interface{}{
		"id":                  "op-456",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusActive),
		"user_id":             userID,
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSetWithTimestamps("operators", "op-456", opBytes, oldTime, oldTime))

	_, err = auth.ValidateOperatorSession(sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operator session expired")
}

func TestAuthService_ValidateOperatorSession_UserInactive(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "")

	// Create an inactive user
	userID := "inactive-user"
	userDoc := map[string]interface{}{
		"id":       userID,
		"username": "inactive",
		"status":   "inactive",
	}
	userBytes, err := json.Marshal(userDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("users", userID, userBytes))

	// Create an operator session linked to the inactive user
	sessionID := "session-with-inactive-user"
	opDoc := map[string]interface{}{
		"id":                  "op-789",
		"operator_session_id": sessionID,
		"status":              marshaler.OperatorStatus(constants.OperatorStatusActive),
		"user_id":             userID,
		"created_at":          time.Now().Format(time.RFC3339),
	}
	opBytes, err := json.Marshal(opDoc)
	require.NoError(t, err)
	require.NoError(t, db.DocSet("operators", "op-789", opBytes))

	_, err = auth.ValidateOperatorSession(sessionID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "identity disabled")
}

func TestAuthService_ExtractOperatorSessionID_BearerToken(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "")

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer test-token-123")

	sessionID := auth.ExtractOperatorSessionID(req)
	assert.Equal(t, "test-token-123", sessionID)
}

func TestAuthService_ExtractOperatorSessionID_NoBearer(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "")

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic dGVzdDp0ZXN0")

	sessionID := auth.ExtractOperatorSessionID(req)
	assert.Empty(t, sessionID)
}

func TestAuthService_ExtractOperatorSessionID_NoHeader(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	res := responder.New(logger)
	auth := NewAuthService(db, nil, logger, userSvc, personaSvc, res, "", nil, "")

	req := httptest.NewRequest("GET", "/", nil)

	sessionID := auth.ExtractOperatorSessionID(req)
	assert.Empty(t, sessionID)
}

func TestAuthError_Error(t *testing.T) {
	err := &AuthError{
		Message: "test error",
		Reason:  "test reason",
		Status:  http.StatusUnauthorized,
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "test error")
	assert.Contains(t, errStr, "test reason")
}

func TestAuthError_Is(t *testing.T) {
	err := &AuthError{
		Message: "test error",
		Status:  http.StatusUnauthorized,
	}

	target := &AuthError{}
	assert.True(t, err.Is(target))

	otherErr := &AuthError{}
	assert.True(t, otherErr.Is(err))
}
