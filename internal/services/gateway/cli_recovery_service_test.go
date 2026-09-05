// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func newTestCLIRecoveryService(t *testing.T) *CLIRecoveryService {
	t.Helper()
	db := newTestDB(t)
	logger := testutil.NewTestLogger()
	return NewCLIRecoveryService(db.GetDocStore(), logger)
}

// generateTestCSR generates an ECDSA P-256 key and a PEM-encoded CSR, returning
// the PEM CSR, the private key, and the expected public-key fingerprint.
func generateTestCSR(t *testing.T, commonName string) (csrPEM string, privKey *ecdsa.PrivateKey, fingerprint string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e-test"},
		},
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, priv)
	require.NoError(t, err)
	csrPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes})

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	h := sha256.Sum256(pubDER)
	return string(csrPEMBytes), priv, hexEncode(h[:])
}

// hexEncode is a local helper to avoid importing encoding/hex just for the test.
func hexEncode(b []byte) string {
	const chars = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = chars[v>>4]
		out[i*2+1] = chars[v&0x0f]
	}
	return string(out)
}

// signProofOfPossession signs the request ID with the CSR private key using
// ECDSA ASN.1 encoding, matching what VerifyProofOfPossession expects.
func signProofOfPossession(t *testing.T, priv *ecdsa.PrivateKey, requestID string) []byte {
	t.Helper()
	msgHash := sha256.Sum256([]byte(requestID))
	sig, err := ecdsa.SignASN1(rand.Reader, priv, msgHash[:])
	require.NoError(t, err)
	return sig
}

// ---------------------------------------------------------------------------
// CreateRequest
// ---------------------------------------------------------------------------

func TestCLIRecovery_CreateRequest_Success(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, expectedFingerprint := generateTestCSR(t, "g8e-cli-recovery")

	requestID, token, expiresAt, err := svc.CreateRequest(csrPEM, "test-sys-fp", &models.LocalOSUser{Username: "bob"})
	require.NoError(t, err)
	assert.NotEmpty(t, requestID)
	assert.NotEmpty(t, token)
	assert.True(t, expiresAt.After(time.Now().UTC()))

	// Verify the request was persisted with a hashed token (not the raw token).
	tokenHash := hashToken(token)
	doc, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash)
	require.NoError(t, err)
	require.NotNil(t, doc)

	dataBytes, err := json.Marshal(doc.Data)
	require.NoError(t, err)
	var stored models.CLIRecoveryRequest
	require.NoError(t, json.Unmarshal(dataBytes, &stored))

	assert.Equal(t, requestID, stored.ID)
	assert.Equal(t, tokenHash, stored.TokenHash, "stored token_hash must be SHA-256 of the raw token")
	assert.NotEqual(t, token, stored.TokenHash, "raw token must never be stored")
	assert.Equal(t, expectedFingerprint, stored.CSRFingerprint)
	assert.Equal(t, "test-sys-fp", stored.SystemFingerprint)
	assert.Equal(t, models.CLIRecoveryStatePending, stored.State)
	assert.Equal(t, "bob", stored.LocalOSUser.Username)
}

func TestCLIRecovery_CreateRequest_InvalidCSR(t *testing.T) {
	svc := newTestCLIRecoveryService(t)

	_, _, _, err := svc.CreateRequest("not-a-csr", "", nil)
	assert.Error(t, err)
}

func TestCLIRecovery_CreateRequest_EmptyCSR(t *testing.T) {
	svc := newTestCLIRecoveryService(t)

	_, _, _, err := svc.CreateRequest("", "", nil)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// GetByToken / GetStatus
// ---------------------------------------------------------------------------

func TestCLIRecovery_GetStatus_Pending(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-status")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	state, err := svc.GetStatus(token)
	require.NoError(t, err)
	assert.Equal(t, models.CLIRecoveryStatePending, state)
}

func TestCLIRecovery_GetStatus_UnknownToken(t *testing.T) {
	svc := newTestCLIRecoveryService(t)

	_, err := svc.GetStatus("nonexistent-token-abcdef1234567890")
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestNotFound))
}

func TestCLIRecovery_GetByToken_ExpiredAutoTransitions(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-expired")

	requestID, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	// Manually backdate the request to simulate expiry.
	tokenHash := hashToken(token)
	doc, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash)
	require.NoError(t, err)
	require.NotNil(t, doc)

	pastTime := time.Now().UTC().Add(-1 * time.Minute)
	dataBytes, _ := json.Marshal(doc.Data)
	var req models.CLIRecoveryRequest
	require.NoError(t, json.Unmarshal(dataBytes, &req))
	req.ExpiresAt = pastTime
	updatedData, _ := json.Marshal(&req)
	require.NoError(t, svc.db.DocSet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash, updatedData))

	_, err = svc.GetByToken(token)
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestExpired))

	// Verify the state was atomically transitioned to expired.
	doc2, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash)
	require.NoError(t, err)
	require.NotNil(t, doc2)
	dataBytes2, _ := json.Marshal(doc2.Data)
	var req2 models.CLIRecoveryRequest
	require.NoError(t, json.Unmarshal(dataBytes2, &req2))
	assert.Equal(t, models.CLIRecoveryStateExpired, req2.State)

	_ = requestID // unused but kept for clarity
}

// ---------------------------------------------------------------------------
// Approve / Deny
// ---------------------------------------------------------------------------

func TestCLIRecovery_Approve_Success(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-approve")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	err = svc.Approve(token, "user-approver-1")
	require.NoError(t, err)

	state, err := svc.GetStatus(token)
	require.NoError(t, err)
	assert.Equal(t, models.CLIRecoveryStateApproved, state)

	// Verify the approving user was persisted.
	req, err := svc.GetByToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-approver-1", req.ApprovingUserID)
	assert.NotNil(t, req.ApprovedAt)
}

func TestCLIRecovery_Deny_Success(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-deny")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	err = svc.Deny(token, "user-denier-1")
	require.NoError(t, err)

	state, err := svc.GetStatus(token)
	require.NoError(t, err)
	assert.Equal(t, models.CLIRecoveryStateDenied, state)

	req, err := svc.GetByToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-denier-1", req.ApprovingUserID)
	assert.NotNil(t, req.DeniedAt)
	assert.Nil(t, req.ApprovedAt)
}

func TestCLIRecovery_Approve_UnknownToken(t *testing.T) {
	svc := newTestCLIRecoveryService(t)

	err := svc.Approve("nonexistent-token-abcdef1234567890", "user-1")
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestNotFound))
}

func TestCLIRecovery_Approve_AlreadyApproved(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-double-approve")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	require.NoError(t, svc.Approve(token, "user-1"))
	err = svc.Approve(token, "user-2")
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestConsumed))
}

func TestCLIRecovery_Approve_AlreadyDenied(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-approve-denied")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	require.NoError(t, svc.Deny(token, "user-1"))
	err = svc.Approve(token, "user-2")
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestDenied))
}

func TestCLIRecovery_Deny_AlreadyApproved(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-deny-approved")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	require.NoError(t, svc.Approve(token, "user-1"))
	err = svc.Deny(token, "user-2")
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestConsumed))
}

func TestCLIRecovery_Approve_ExpiredRequest(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-approve-expired")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	// Backdate expiry.
	tokenHash := hashToken(token)
	doc, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash)
	require.NoError(t, err)
	dataBytes, _ := json.Marshal(doc.Data)
	var req models.CLIRecoveryRequest
	require.NoError(t, json.Unmarshal(dataBytes, &req))
	req.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)
	updatedData, _ := json.Marshal(&req)
	require.NoError(t, svc.db.DocSet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash, updatedData))

	err = svc.Approve(token, "user-1")
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestExpired))
}

// ---------------------------------------------------------------------------
// Complete
// ---------------------------------------------------------------------------

func TestCLIRecovery_Complete_Success(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, priv, _ := generateTestCSR(t, "g8e-cli-complete")

	requestID, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	require.NoError(t, svc.Approve(token, "user-approver"))

	req, err := svc.Complete(token)
	require.NoError(t, err)
	assert.Equal(t, models.CLIRecoveryStateCompleted, req.State)
	assert.Equal(t, requestID, req.ID)
	assert.NotNil(t, req.CompletedAt)
	assert.Equal(t, "user-approver", req.ApprovingUserID)

	// Verify proof-of-possession succeeds with the correct private key.
	sig := signProofOfPossession(t, priv, requestID)
	assert.NoError(t, svc.VerifyProofOfPossession(req, sig))

	// Verify CSR match succeeds with the original CSR.
	assert.NoError(t, svc.VerifyTokenCSRMatch(req, csrPEM))
}

func TestCLIRecovery_Complete_NotApproved(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-complete-pending")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	_, err = svc.Complete(token)
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryNotApproved))
}

func TestCLIRecovery_Complete_AlreadyCompleted(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-complete-twice")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)
	require.NoError(t, svc.Approve(token, "user-1"))

	_, err = svc.Complete(token)
	require.NoError(t, err)

	_, err = svc.Complete(token)
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestConsumed))
}

func TestCLIRecovery_Complete_DeniedRequest(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-complete-denied")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)
	require.NoError(t, svc.Deny(token, "user-1"))

	_, err = svc.Complete(token)
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestDenied))
}

func TestCLIRecovery_Complete_UnknownToken(t *testing.T) {
	svc := newTestCLIRecoveryService(t)

	_, err := svc.Complete("nonexistent-token-abcdef1234567890")
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestNotFound))
}

func TestCLIRecovery_Complete_ExpiredRequest(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-complete-expired")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)
	require.NoError(t, svc.Approve(token, "user-1"))

	// Backdate expiry.
	tokenHash := hashToken(token)
	doc, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash)
	require.NoError(t, err)
	dataBytes, _ := json.Marshal(doc.Data)
	var req models.CLIRecoveryRequest
	require.NoError(t, json.Unmarshal(dataBytes, &req))
	req.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)
	updatedData, _ := json.Marshal(&req)
	require.NoError(t, svc.db.DocSet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash, updatedData))

	_, err = svc.Complete(token)
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryRequestExpired))
}

// ---------------------------------------------------------------------------
// Proof-of-possession verification
// ---------------------------------------------------------------------------

func TestCLIRecovery_VerifyProofOfPossession_ValidSignature(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, priv, _ := generateTestCSR(t, "g8e-cli-pop-valid")

	requestID, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)
	require.NoError(t, svc.Approve(token, "user-1"))

	req, err := svc.Complete(token)
	require.NoError(t, err)

	sig := signProofOfPossession(t, priv, requestID)
	assert.NoError(t, svc.VerifyProofOfPossession(req, sig))
}

func TestCLIRecovery_VerifyProofOfPossession_WrongKey(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-pop-wrongkey")

	requestID, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)
	require.NoError(t, svc.Approve(token, "user-1"))

	req, err := svc.Complete(token)
	require.NoError(t, err)

	// Sign with a different key.
	otherPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	sig := signProofOfPossession(t, otherPriv, requestID)

	err = svc.VerifyProofOfPossession(req, sig)
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryProofInvalid))
}

func TestCLIRecovery_VerifyProofOfPossession_WrongMessage(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, priv, _ := generateTestCSR(t, "g8e-cli-pop-wrongmsg")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)
	require.NoError(t, svc.Approve(token, "user-1"))

	req, err := svc.Complete(token)
	require.NoError(t, err)

	// Sign a different message (not the request ID).
	sig := signProofOfPossession(t, priv, "wrong-message")
	err = svc.VerifyProofOfPossession(req, sig)
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryProofInvalid))
}

func TestCLIRecovery_VerifyProofOfPossession_InvalidCSR(t *testing.T) {
	svc := newTestCLIRecoveryService(t)

	req := &models.CLIRecoveryRequest{
		CLICSRPEM: "not-a-csr",
	}
	err := svc.VerifyProofOfPossession(req, []byte("invalid-sig"))
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryProofInvalid))
}

// ---------------------------------------------------------------------------
// CSR match verification
// ---------------------------------------------------------------------------

func TestCLIRecovery_VerifyTokenCSRMatch_MatchingCSR(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-csrmatch-ok")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	req, err := svc.GetByToken(token)
	require.NoError(t, err)

	assert.NoError(t, svc.VerifyTokenCSRMatch(req, csrPEM))
}

func TestCLIRecovery_VerifyTokenCSRMatch_DifferentCSR(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM1, _, _ := generateTestCSR(t, "g8e-cli-csrmatch-orig")
	csrPEM2, _, _ := generateTestCSR(t, "g8e-cli-csrmatch-different")

	_, token, _, err := svc.CreateRequest(csrPEM1, "", nil)
	require.NoError(t, err)

	req, err := svc.GetByToken(token)
	require.NoError(t, err)

	err = svc.VerifyTokenCSRMatch(req, csrPEM2)
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryCSRMismatch))
}

func TestCLIRecovery_VerifyTokenCSRMatch_InvalidCSR(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-csrmatch-invalid")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	req, err := svc.GetByToken(token)
	require.NoError(t, err)

	err = svc.VerifyTokenCSRMatch(req, "not-a-csr")
	assert.True(t, errors.Is(err, constants.ErrCLIRecoveryCSRMismatch))
}

// ---------------------------------------------------------------------------
// Concurrent completion (atomicity)
// ---------------------------------------------------------------------------

func TestCLIRecovery_Complete_ConcurrentOnlyOneSucceeds(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-concurrent")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)
	require.NoError(t, svc.Approve(token, "user-1"))

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var successes, failures int
	var mu sync.Mutex

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := svc.Complete(token)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				failures++
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent completion must succeed")
	assert.Equal(t, goroutines-1, failures, "all other concurrent completions must fail")
}

func TestCLIRecovery_Approve_ConcurrentOnlyOneSucceeds(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-concurrent-approve")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var successes, failures int
	var mu sync.Mutex

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := svc.Approve(token, "user-concurrent")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else {
				failures++
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent approval must succeed")
	assert.Equal(t, goroutines-1, failures, "all other concurrent approvals must fail")
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

func TestCLIRecovery_CleanupExpired(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM1, _, _ := generateTestCSR(t, "g8e-cli-cleanup-expired")
	csrPEM2, _, _ := generateTestCSR(t, "g8e-cli-cleanup-valid")

	// Create a request and backdate it to be expired.
	_, token1, _, err := svc.CreateRequest(csrPEM1, "", nil)
	require.NoError(t, err)
	tokenHash1 := hashToken(token1)
	doc, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash1)
	require.NoError(t, err)
	dataBytes, _ := json.Marshal(doc.Data)
	var req1 models.CLIRecoveryRequest
	require.NoError(t, json.Unmarshal(dataBytes, &req1))
	req1.ExpiresAt = time.Now().UTC().Add(-5 * time.Minute)
	updatedData, _ := json.Marshal(&req1)
	require.NoError(t, svc.db.DocSet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash1, updatedData))

	// Create a valid (non-expired) request.
	_, token2, _, err := svc.CreateRequest(csrPEM2, "", nil)
	require.NoError(t, err)
	tokenHash2 := hashToken(token2)

	// Run cleanup.
	require.NoError(t, svc.CleanupExpired())

	// Expired request should be deleted.
	doc, err = svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash1)
	require.NoError(t, err)
	assert.Nil(t, doc, "expired request should be deleted")

	// Valid request should remain.
	doc, err = svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash2)
	require.NoError(t, err)
	assert.NotNil(t, doc, "valid request should remain")
}

func TestCLIRecovery_CleanupExpired_EmptyCollection(t *testing.T) {
	svc := newTestCLIRecoveryService(t)

	err := svc.CleanupExpired()
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Token hashing and storage safety
// ---------------------------------------------------------------------------

func TestCLIRecovery_TokenHashIsNotRawToken(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM, _, _ := generateTestCSR(t, "g8e-cli-hash-safety")

	_, token, _, err := svc.CreateRequest(csrPEM, "", nil)
	require.NoError(t, err)

	// The raw token must not be a document ID in the collection.
	doc, err := svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), token)
	require.NoError(t, err)
	assert.Nil(t, doc, "raw token must not be usable as a lookup key")

	// The hash of the token must be the document ID.
	tokenHash := hashToken(token)
	doc, err = svc.db.DocGet(marshaler.CollectionName(constants.CollectionCLIRecoveryRequests), tokenHash)
	require.NoError(t, err)
	assert.NotNil(t, doc, "token hash must be the lookup key")
}

func TestCLIRecovery_TwoRequestsHaveDifferentTokens(t *testing.T) {
	svc := newTestCLIRecoveryService(t)
	csrPEM1, _, _ := generateTestCSR(t, "g8e-cli-unique-1")
	csrPEM2, _, _ := generateTestCSR(t, "g8e-cli-unique-2")

	_, token1, _, err := svc.CreateRequest(csrPEM1, "", nil)
	require.NoError(t, err)
	_, token2, _, err := svc.CreateRequest(csrPEM2, "", nil)
	require.NoError(t, err)

	assert.NotEqual(t, token1, token2, "each request must have a unique token")
	assert.NotEqual(t, hashToken(token1), hashToken(token2), "token hashes must be unique")
}
