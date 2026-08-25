// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/test/fixtures"
)

// TestCLIRefresh_ExpiredSessionRecoverable verifies the full Tier 2 refresh
// flow: enroll a CLI identity, expire the session server-side, call the
// refresh endpoint with the still-valid cert, and verify the new session
// works for a downstream mTLS call.
//
// This is the integration-test assertion of the E.3 completion criterion:
// "An expired CLI session on a running stack is recoverable without nuking
// the gateway volume."
func TestCLIRefresh_ExpiredSessionRecoverable(t *testing.T) {
	fixture := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		AllowTestPortZero: true,
	})
	fixture.WaitForReady(t)

	// Enroll a CLI identity with a real cert and session.
	identity := fixtures.EnrollClientIdentity(t, fixture, "user-refresh-int", "org-refresh-int", "sys-fp-int", "host-int")

	// Verify the enrolled CLI session works before expiry.
	cliClient := fixtures.CreateCLIMTLSClient(t, fixture, identity)
	mtlsURL := network.LocalhostHTTPSURL(fixture.Service.GetHTTPSPort())

	// 1. Expire the CLI session server-side by overwriting the session
	//    document with an ExpiresAt in the past. This simulates the TTL
	//    elapsing without waiting for the real 7-day TTL.
	cliSessionID := identity.CLISessionID
	expiredSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            identity.UserID,
		OperatorSessionID: identity.OperatorSessionID,
		SystemFingerprint: "sys-fp-int",
		CertFingerprint:   "cert-fp-int",
		CertSerial:        "serial-int",
		CreatedAt:         time.Now().Add(-2 * time.Hour),
		ExpiresAt:         time.Now().Add(-1 * time.Hour), // expired
		AbsoluteExpiresAt: time.Now().Add(-1 * time.Hour),
		IdleExpiresAt:     time.Now().Add(-1 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "mTLS",
	}
	expiredBytes, err := json.Marshal(expiredSession)
	require.NoError(t, err)
	require.NoError(t, fixture.Service.GetDocStore().DocSet(
		marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, expiredBytes,
	))

	// 2. Verify the expired session is rejected on a non-refresh endpoint
	//    (fail-closed). Use the audit receipts endpoint as a stand-in.
	preRefreshReq, _ := http.NewRequest(http.MethodGet, mtlsURL+constants.APIPaths.AuditReceipts, nil)
	preRefreshResp, err := cliClient.Do(preRefreshReq)
	require.NoError(t, err)
	_, _ = io.ReadAll(preRefreshResp.Body)
	preRefreshResp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, preRefreshResp.StatusCode,
		"expired session must be rejected on non-refresh endpoints (fail-closed)")

	// 3. Call the refresh endpoint with the still-valid cert. The auth
	//    middleware admits the expired session to the refresh controller,
	//    which issues a new session. Use a fresh HTTP client for the
	//    refresh call so the connection pool does not hand out a
	//    connection the gateway may have closed after the 401 above.
	refreshCliClient := fixtures.CreateCLIMTLSClient(t, fixture, identity)
	refreshBody, _ := json.Marshal(models.CLIRefreshRequest{})
	refreshReq, _ := http.NewRequest(http.MethodPost, mtlsURL+constants.APIPaths.AuthCLIRefresh, bytes.NewReader(refreshBody))
	refreshResp, err := refreshCliClient.Do(refreshReq)
	require.NoError(t, err)
	defer refreshResp.Body.Close()
	refreshRespBytes, err := io.ReadAll(refreshResp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusCreated, refreshResp.StatusCode,
		"refresh should succeed with a valid cert and expired session, body: %s", string(refreshRespBytes))

	var refreshResult models.CLIRefreshResponse
	require.NoError(t, json.Unmarshal(refreshRespBytes, &refreshResult))
	require.True(t, refreshResult.Success)
	require.NotEmpty(t, refreshResult.CLISessionID)
	require.NotEqual(t, cliSessionID, refreshResult.CLISessionID,
		"refresh must issue a new session ID distinct from the expired one")
	assert.Equal(t, identity.UserID, refreshResult.UserID)

	// 4. Verify the old session is deactivated.
	oldDoc, err := fixture.Service.GetDocStore().DocGet(
		marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID)
	require.NoError(t, err)
	require.NotNil(t, oldDoc)
	var oldSession models.CLISession
	oldDataBytes, err := json.Marshal(oldDoc.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(oldDataBytes, &oldSession))
	assert.False(t, oldSession.IsActive, "old session must be deactivated after refresh")

	// 5. Verify the new session is active and persisted.
	newDoc, err := fixture.Service.GetDocStore().DocGet(
		marshaler.CollectionName(constants.CollectionCLISessions), refreshResult.CLISessionID)
	require.NoError(t, err)
	require.NotNil(t, newDoc)
	var newSession models.CLISession
	newDataBytes, err := json.Marshal(newDoc.Data)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(newDataBytes, &newSession))
	assert.True(t, newSession.IsActive, "new session must be active")
	assert.Equal(t, identity.UserID, newSession.UserID)
	assert.True(t, newSession.ExpiresAt.After(time.Now()),
		"new session must have a future expiry (TTL refreshed)")

	// 6. Verify the new session works for a downstream mTLS call by
	//    building a CLI client with the new session ID header.
	identity.CLISessionID = refreshResult.CLISessionID
	newCliClient := fixtures.CreateCLIMTLSClient(t, fixture, identity)
	postRefreshReq, _ := http.NewRequest(http.MethodGet, mtlsURL+constants.APIPaths.AuditReceipts, nil)
	postRefreshResp, err := newCliClient.Do(postRefreshReq)
	require.NoError(t, err)
	_, _ = io.ReadAll(postRefreshResp.Body)
	postRefreshResp.Body.Close()
	assert.NotEqual(t, http.StatusUnauthorized, postRefreshResp.StatusCode,
		"new session must be accepted on downstream endpoints (no longer expired)")
}
