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
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionService_PersistSessions_Success(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewSessionService(db, logger)

	cliSessionID := "cli-session-123"
	operatorSessionID := "op-session-456"
	userID := "user-789"
	orgID := "org-101"
	operatorID := "operator-202"
	systemFingerprint := "fp-abc123"
	certFingerprint := "cert-fp-456"
	certSerial := "serial-789"
	loginMethod := "passkey"

	err := svc.PersistSessions(cliSessionID, operatorSessionID, userID, orgID, operatorID, systemFingerprint, certFingerprint, certSerial, loginMethod)
	require.NoError(t, err)

	// Verify CLI session was persisted
	cliDoc, err := db.DocGet("cli_sessions", cliSessionID)
	require.NoError(t, err)
	require.NotNil(t, cliDoc)
	var cliUserIDStr, operatorSessionIDStr, systemFingerprintStr, certFingerprintStr, certSerialStr, cliLoginMethodStr string
	json.Unmarshal(cliDoc.Data["user_id"], &cliUserIDStr)
	json.Unmarshal(cliDoc.Data["operator_session_id"], &operatorSessionIDStr)
	json.Unmarshal(cliDoc.Data["system_fingerprint"], &systemFingerprintStr)
	json.Unmarshal(cliDoc.Data["cert_fingerprint"], &certFingerprintStr)
	json.Unmarshal(cliDoc.Data["cert_serial"], &certSerialStr)
	json.Unmarshal(cliDoc.Data["login_method"], &cliLoginMethodStr)
	assert.Equal(t, userID, cliUserIDStr)
	assert.Equal(t, operatorSessionID, operatorSessionIDStr)
	assert.Equal(t, systemFingerprint, systemFingerprintStr)
	assert.Equal(t, certFingerprint, certFingerprintStr)
	assert.Equal(t, certSerial, certSerialStr)
	assert.Equal(t, loginMethod, cliLoginMethodStr)

	// Verify Operator session was persisted
	opDoc, err := db.DocGet("operator_sessions", operatorSessionID)
	require.NoError(t, err)
	require.NotNil(t, opDoc)
	var opUserIDStr, orgIDStr, operatorIDStr, opLoginMethodStr string
	json.Unmarshal(opDoc.Data["user_id"], &opUserIDStr)
	json.Unmarshal(opDoc.Data["organization_id"], &orgIDStr)
	json.Unmarshal(opDoc.Data["operator_id"], &operatorIDStr)
	json.Unmarshal(opDoc.Data["login_method"], &opLoginMethodStr)
	assert.Equal(t, userID, opUserIDStr)
	assert.Equal(t, orgID, orgIDStr)
	assert.Equal(t, operatorID, operatorIDStr)
	assert.Equal(t, loginMethod, opLoginMethodStr)
}

func TestSessionService_PersistSessions_EmptyFields(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewSessionService(db, logger)

	cliSessionID := "cli-session-empty"
	operatorSessionID := "op-session-empty"
	userID := "user-empty"
	orgID := ""
	operatorID := ""
	systemFingerprint := ""
	certFingerprint := ""
	certSerial := ""
	loginMethod := "csr"

	err := svc.PersistSessions(cliSessionID, operatorSessionID, userID, orgID, operatorID, systemFingerprint, certFingerprint, certSerial, loginMethod)
	require.NoError(t, err)

	// Verify sessions were persisted even with empty optional fields
	cliDoc, err := db.DocGet("cli_sessions", cliSessionID)
	require.NoError(t, err)
	require.NotNil(t, cliDoc)
	var userIDStr string
	json.Unmarshal(cliDoc.Data["user_id"], &userIDStr)
	assert.Equal(t, userID, userIDStr)

	opDoc, err := db.DocGet("operator_sessions", operatorSessionID)
	require.NoError(t, err)
	require.NotNil(t, opDoc)
	var opUserIDStr string
	json.Unmarshal(opDoc.Data["user_id"], &opUserIDStr)
	assert.Equal(t, userID, opUserIDStr)
}

func TestSessionService_PersistSessions_OverwritesExisting(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewSessionService(db, logger)

	cliSessionID := "cli-session-overwrite"
	operatorSessionID := "op-session-overwrite"
	userID := "user-overwrite"
	orgID := "org-overwrite"
	operatorID := "operator-overwrite"
	systemFingerprint := "fp-original"
	certFingerprint := "cert-fp-original"
	certSerial := "serial-original"
	loginMethod := "passkey"

	// Create initial sessions
	err := svc.PersistSessions(cliSessionID, operatorSessionID, userID, orgID, operatorID, systemFingerprint, certFingerprint, certSerial, loginMethod)
	require.NoError(t, err)

	// Update with new values
	newSystemFingerprint := "fp-updated"
	newCertFingerprint := "cert-fp-updated"
	newCertSerial := "serial-updated"
	newLoginMethod := "csr"

	err = svc.PersistSessions(cliSessionID, operatorSessionID, userID, orgID, operatorID, newSystemFingerprint, newCertFingerprint, newCertSerial, newLoginMethod)
	require.NoError(t, err)

	// Verify sessions were updated
	cliDoc, err := db.DocGet("cli_sessions", cliSessionID)
	require.NoError(t, err)
	require.NotNil(t, cliDoc)
	var systemFingerprintStr, certFingerprintStr, certSerialStr, loginMethodStr string
	json.Unmarshal(cliDoc.Data["system_fingerprint"], &systemFingerprintStr)
	json.Unmarshal(cliDoc.Data["cert_fingerprint"], &certFingerprintStr)
	json.Unmarshal(cliDoc.Data["cert_serial"], &certSerialStr)
	json.Unmarshal(cliDoc.Data["login_method"], &loginMethodStr)
	assert.Equal(t, newSystemFingerprint, systemFingerprintStr)
	assert.Equal(t, newCertFingerprint, certFingerprintStr)
	assert.Equal(t, newCertSerial, certSerialStr)
	assert.Equal(t, newLoginMethod, loginMethodStr)
}

func TestSessionService_PersistSessions_CLIOnly(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewSessionService(db, logger)

	cliSessionID := "cli-session-only"
	operatorSessionID := "" // Empty for CLI-only enrollment
	userID := "user-cli-only"
	orgID := ""
	operatorID := ""
	systemFingerprint := "fp-cli-only"
	certFingerprint := "cert-fp-cli-only"
	certSerial := "serial-cli-only"
	loginMethod := "bootstrap"

	err := svc.PersistSessions(cliSessionID, operatorSessionID, userID, orgID, operatorID, systemFingerprint, certFingerprint, certSerial, loginMethod)
	require.NoError(t, err)

	// Verify CLI session was persisted
	cliDoc, err := db.DocGet("cli_sessions", cliSessionID)
	require.NoError(t, err)
	require.NotNil(t, cliDoc)
	var cliUserIDStr, operatorSessionIDStr, systemFingerprintStr, certFingerprintStr, certSerialStr, cliLoginMethodStr string
	json.Unmarshal(cliDoc.Data["user_id"], &cliUserIDStr)
	json.Unmarshal(cliDoc.Data["operator_session_id"], &operatorSessionIDStr)
	json.Unmarshal(cliDoc.Data["system_fingerprint"], &systemFingerprintStr)
	json.Unmarshal(cliDoc.Data["cert_fingerprint"], &certFingerprintStr)
	json.Unmarshal(cliDoc.Data["cert_serial"], &certSerialStr)
	json.Unmarshal(cliDoc.Data["login_method"], &cliLoginMethodStr)
	assert.Equal(t, userID, cliUserIDStr)
	assert.Equal(t, "", operatorSessionIDStr, "operator_session_id should be empty for CLI-only enrollment")
	assert.Equal(t, systemFingerprint, systemFingerprintStr)
	assert.Equal(t, certFingerprint, certFingerprintStr)
	assert.Equal(t, certSerial, certSerialStr)
	assert.Equal(t, loginMethod, cliLoginMethodStr)

	// Verify Operator session was NOT persisted (CLI-only enrollment)
	opDoc, err := db.DocGet("operator_sessions", cliSessionID)
	require.NoError(t, err)
	assert.Nil(t, opDoc, "operator session should not be created for CLI-only enrollment")
}

func TestSessionService_PersistSessions_OperatorOnly(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	svc := NewSessionService(db, logger)

	cliSessionID := "" // Empty for operator-only enrollment
	operatorSessionID := "op-session-only"
	userID := "user-op-only"
	orgID := "org-op-only"
	operatorID := "operator-op-only"
	systemFingerprint := ""
	certFingerprint := ""
	certSerial := ""
	loginMethod := "csr"

	err := svc.PersistSessions(cliSessionID, operatorSessionID, userID, orgID, operatorID, systemFingerprint, certFingerprint, certSerial, loginMethod)
	require.NoError(t, err)

	// Verify CLI session was NOT persisted (operator-only enrollment)
	cliDoc, err := db.DocGet("cli_sessions", operatorSessionID)
	require.NoError(t, err)
	assert.Nil(t, cliDoc, "CLI session should not be created for operator-only enrollment")

	// Verify Operator session was persisted
	opDoc, err := db.DocGet("operator_sessions", operatorSessionID)
	require.NoError(t, err)
	require.NotNil(t, opDoc)
	var opUserIDStr, orgIDStr, operatorIDStr, opLoginMethodStr string
	json.Unmarshal(opDoc.Data["user_id"], &opUserIDStr)
	json.Unmarshal(opDoc.Data["organization_id"], &orgIDStr)
	json.Unmarshal(opDoc.Data["operator_id"], &operatorIDStr)
	json.Unmarshal(opDoc.Data["login_method"], &opLoginMethodStr)
	assert.Equal(t, userID, opUserIDStr)
	assert.Equal(t, orgID, orgIDStr)
	assert.Equal(t, operatorID, operatorIDStr)
	assert.Equal(t, loginMethod, opLoginMethodStr)
}

func TestNewSessionService(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()

	svc := NewSessionService(db, logger)
	require.NotNil(t, svc)
	assert.Equal(t, db, svc.db)
	assert.Equal(t, logger, svc.logger)
}
