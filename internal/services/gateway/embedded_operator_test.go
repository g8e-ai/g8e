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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// generateTestCLICSRPEM returns a PEM-encoded CLI CSR for bootstrap tests.
func generateTestCLICSRPEM(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "g8e-cli-test", Organization: []string{"g8e"}},
	}, priv)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
}

// postBootstrap submits a local bootstrap request carrying the given CLI
// CSR and returns the recorder for assertions.
func postBootstrap(t *testing.T, h *HTTPHandler, csrPEM []byte) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(models.BootstrapRequest{
		CLICSR:            string(csrPEM),
		SystemFingerprint: "test-fingerprint",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, constants.APIPaths.AuthBootstrap, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	h.bootstrapController.handleLocalBootstrapWithURL(rr, req)
	return rr
}

// TestRegisterPendingEmbeddedOperator_Idempotent verifies that gateway
// startup registration creates exactly one pending embedded-operator
// document, that a second registration leaves it untouched, and that
// registration never overwrites a claimed document.
func TestRegisterPendingEmbeddedOperator_Idempotent(t *testing.T) {
	infra := setupTestInfrastructure(t, false)

	require.NoError(t, registerPendingEmbeddedOperator(infra.DocStore, infra.Logger))

	op := loadEmbeddedOperatorDoc(t, infra.DocStore)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), op.ID)
	assert.Equal(t, constants.OperatorTypeEmbedded, op.OperatorType)
	assert.Equal(t, constants.ComponentNameG8EO, op.Component)
	assert.Equal(t, constants.OperatorStatusAvailable, op.Status)
	assert.False(t, op.Claimed)
	assert.False(t, op.IsSlot)
	assert.Empty(t, op.UserID)
	assert.Empty(t, op.OperatorSessionID)

	// A second registration is a no-op — exactly one operators document.
	require.NoError(t, registerPendingEmbeddedOperator(infra.DocStore, infra.Logger))
	docs, err := infra.DocStore.DocList(marshaler.CollectionName(constants.CollectionOperators))
	require.NoError(t, err)
	assert.Len(t, docs, 1)

	// A claimed document is left untouched by later registrations.
	_, sessionID, err := claimEmbeddedOperator(infra.DocStore, "user-claim", "fp", time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, registerPendingEmbeddedOperator(infra.DocStore, infra.Logger))
	claimed := loadEmbeddedOperatorDoc(t, infra.DocStore)
	assert.True(t, claimed.Claimed)
	assert.Equal(t, "user-claim", claimed.UserID)
	assert.Equal(t, sessionID, claimed.OperatorSessionID)
}

// TestBootstrap_ClaimsEmbeddedOperatorAndBindsCLISession verifies the
// bootstrap happy path when the pending document was registered at gateway
// start: the first user's bootstrap claims the pending embedded operator,
// persists its operator session, binds the new CLI session to that session,
// and returns both operator fields in the response.
func TestBootstrap_ClaimsEmbeddedOperatorAndBindsCLISession(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)
	require.NoError(t, registerPendingEmbeddedOperator(infra.DocStore, infra.Logger))

	rr := postBootstrap(t, h, generateTestCLICSRPEM(t))
	require.Equal(t, http.StatusCreated, rr.Code, "bootstrap failed: %s", rr.Body.String())

	var resp models.BootstrapResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), resp.OperatorID)
	assert.NotEmpty(t, resp.OperatorSessionID)
	assert.NotEmpty(t, resp.CLISessionID)
	assert.NotEqual(t, resp.OperatorSessionID, resp.CLISessionID)
	require.NotNil(t, resp.User)

	// The claim persisted the pending document into the claimed state.
	op := loadEmbeddedOperatorDoc(t, infra.DocStore)
	assert.True(t, op.Claimed)
	assert.Equal(t, resp.User.ID, op.UserID)
	assert.Equal(t, resp.OperatorSessionID, op.OperatorSessionID)
	assert.Equal(t, constants.OperatorTypeEmbedded, op.OperatorType)

	// The operator session document exists for the minted session.
	opSession, err := infra.OperatorSessionSvc.GetActiveSessionForUser(resp.User.ID)
	require.NoError(t, err)
	require.NotNil(t, opSession)
	assert.Equal(t, resp.OperatorSessionID, opSession.ID)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), opSession.OperatorID)

	// The CLI session is bound to the minted operator session.
	cliSession, err := infra.CLISessionSvc.loadCLISession(resp.CLISessionID)
	require.NoError(t, err)
	assert.Equal(t, resp.OperatorSessionID, cliSession.OperatorSessionID)
}

// TestBootstrap_MissingPendingDoc_ClaimsAnyway verifies the self-healing
// path: when the pending embedded-operator document is absent (e.g., a
// gateway that lost its pending record), bootstrap creates it and claims
// it rather than failing.
func TestBootstrap_MissingPendingDoc_ClaimsAnyway(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)

	doc, err := infra.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	require.NoError(t, err)
	require.Nil(t, doc, "no pending document is seeded")

	rr := postBootstrap(t, h, generateTestCLICSRPEM(t))
	require.Equal(t, http.StatusCreated, rr.Code, "bootstrap must self-heal a missing pending document: %s", rr.Body.String())

	var resp models.BootstrapResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), resp.OperatorID)
	assert.NotEmpty(t, resp.OperatorSessionID)

	op := loadEmbeddedOperatorDoc(t, infra.DocStore)
	assert.True(t, op.Claimed)
	assert.Equal(t, resp.User.ID, op.UserID)
	assert.Equal(t, resp.OperatorSessionID, op.OperatorSessionID)
}

// TestClaimEmbeddedOperator_SameUserReclaimIsIdempotent verifies that a
// second claim by the same user returns the document's existing operator
// session without minting a new one.
func TestClaimEmbeddedOperator_SameUserReclaimIsIdempotent(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	now := time.Now().UTC()

	operatorID, sessionID, err := claimEmbeddedOperator(infra.DocStore, "user-same", "fp", now)
	require.NoError(t, err)
	assert.Equal(t, string(constants.DocIDEmbeddedOperator), operatorID)
	assert.NotEmpty(t, sessionID)

	operatorID2, sessionID2, err := claimEmbeddedOperator(infra.DocStore, "user-same", "fp", now)
	require.NoError(t, err)
	assert.Equal(t, operatorID, operatorID2)
	assert.Equal(t, sessionID, sessionID2, "same-user re-claim returns the existing session without rotation")
}

// TestClaimEmbeddedOperator_DifferentUserRejected verifies that the
// embedded operator belongs to exactly one user for its lifetime: a claim
// by a different user fails with ErrEmbeddedOperatorClaimed.
func TestClaimEmbeddedOperator_DifferentUserRejected(t *testing.T) {
	infra := setupTestInfrastructure(t, false)
	now := time.Now().UTC()

	_, _, err := claimEmbeddedOperator(infra.DocStore, "user-owner", "fp", now)
	require.NoError(t, err)

	_, _, err = claimEmbeddedOperator(infra.DocStore, "user-other", "fp", now)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEmbeddedOperatorClaimed)
}
