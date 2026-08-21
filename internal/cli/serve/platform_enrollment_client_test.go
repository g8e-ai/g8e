// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package serve

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"

	"google.golang.org/protobuf/proto"
)

// --- Transcript construction parity ---

// TestBuildOperatorCompletionTranscript_MatchesGatewayProto verifies
// that the operator client's transcript construction produces
// byte-identical output to the gateway's deterministic protobuf
// encoding. This is the Go parity vector for the operator component
// (operator + CLI fingerprints).
func TestBuildOperatorCompletionTranscript_MatchesGatewayProto(t *testing.T) {
	requestID := "req-abc-123"
	tokenHash := "deadbeef" + repeatChar("0", 56)
	instanceID := "operator-test-host"
	operatorFP := "aaaa" + repeatChar("1", 60)
	cliFP := "bbbb" + repeatChar("2", 60)

	// Build via the client function.
	clientTranscript, err := buildOperatorCompletionTranscript(requestID, tokenHash, instanceID, operatorFP, cliFP)
	require.NoError(t, err)

	// Build via the gateway's exact proto construction (mirrors
	// platform_enrollment_validation.go).
	expectedMessage := &commonv1.PlatformEnrollmentCompletionTranscript{
		ProtocolVersion: constants.PlatformEnrollmentProtocolVersion,
		RequestId:       requestID,
		TokenHash:       tokenHash,
		ComponentKind:   commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR,
		InstanceId:      instanceID,
		Fingerprints: &commonv1.PlatformEnrollmentFingerprints{
			Operator: operatorFP,
			Cli:      cliFP,
		},
	}
	expected, err := (proto.MarshalOptions{Deterministic: true}).Marshal(expectedMessage)
	require.NoError(t, err)

	assert.Equal(t, expected, clientTranscript, "operator transcript must be byte-identical to gateway proto encoding")
}

// TestBuildOperatorCompletionTranscript_ComponentKindIsOperator
// verifies the enum value is 3 (OPERATOR), not 1 (DASHBOARD) or 2
// (ENSEMBLE). A wrong component kind would cause proof verification
// to fail on the gateway.
func TestBuildOperatorCompletionTranscript_ComponentKindIsOperator(t *testing.T) {
	transcript, err := buildOperatorCompletionTranscript("req", "hash", "instance", "opfp", "clifp")
	require.NoError(t, err)

	// Decode the protobuf and check the component_kind field.
	msg := &commonv1.PlatformEnrollmentCompletionTranscript{}
	err = proto.Unmarshal(transcript, msg)
	require.NoError(t, err)
	assert.Equal(t, commonv1.PlatformComponentKind_PLATFORM_COMPONENT_KIND_OPERATOR, msg.GetComponentKind())
}

// TestBuildOperatorCompletionTranscript_IncludesBothFingerprints
// verifies the operator transcript includes both operator and CLI
// fingerprints (unlike dashboard/ensemble which only include the app
// fingerprint). A missing fingerprint would cause proof verification
// to fail.
func TestBuildOperatorCompletionTranscript_IncludesBothFingerprints(t *testing.T) {
	operatorFP := "op-fp-value"
	cliFP := "cli-fp-value"
	transcript, err := buildOperatorCompletionTranscript("req", "hash", "instance", operatorFP, cliFP)
	require.NoError(t, err)

	msg := &commonv1.PlatformEnrollmentCompletionTranscript{}
	err = proto.Unmarshal(transcript, msg)
	require.NoError(t, err)
	require.NotNil(t, msg.Fingerprints)
	assert.Equal(t, operatorFP, msg.Fingerprints.GetOperator())
	assert.Equal(t, cliFP, msg.Fingerprints.GetCli())
	assert.Empty(t, msg.Fingerprints.GetApp(), "operator transcript must not set the app fingerprint")
}

// --- CSR fingerprint ---

// TestCsrFingerprint_MatchesGatewayComputation verifies the client's
// CSR fingerprint computation matches the gateway's: SHA-256 of the
// SubjectPublicKeyInfo DER bytes, hex-encoded.
func TestCsrFingerprint_MatchesGatewayComputation(t *testing.T) {
	csrPEM, privKey, err := generateTestCSR(t, "test-fp")
	require.NoError(t, err)

	// Client computation.
	clientFP, err := csrFingerprint(csrPEM)
	require.NoError(t, err)

	// Gateway computation (mirrors parsePlatformEnrollmentCSR).
	publicDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	require.NoError(t, err)
	digest := sha256.Sum256(publicDER)
	expectedFP := hex.EncodeToString(digest[:])

	assert.Equal(t, expectedFP, clientFP)
}

// TestCsrFingerprint_RejectsNonP256Key verifies the fingerprint
// function fails closed on a non-P-256 key.
func TestCsrFingerprint_RejectsNonP256Key(t *testing.T) {
	// Generate an RSA CSR (not P-256).
	privKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	template := x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "test-p384"},
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	require.NoError(t, err)
	csrPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrBytes}))

	_, err = csrFingerprint(csrPEM)
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentUnsupportedKey)
}

// --- Token hash ---

// TestTokenHash_MatchesGatewayComputation verifies the client's token
// hash matches the gateway's: SHA-256 of the raw token, hex-encoded.
func TestTokenHash_MatchesGatewayComputation(t *testing.T) {
	token := "test-token-value-abc123"
	clientHash := tokenHash(token)
	digest := sha256.Sum256([]byte(token))
	expectedHash := hex.EncodeToString(digest[:])
	assert.Equal(t, expectedHash, clientHash)
}

// --- Transcript signing and verification ---

// TestSignTranscript_ProducesVerifiableASN1Signature verifies the
// client's signature can be verified with ecdsa.VerifyASN1 against the
// corresponding public key. This mirrors the gateway's
// verifyPlatformEnrollmentProof.
func TestSignTranscript_ProducesVerifiableASN1Signature(t *testing.T) {
	csrPEM, privateKey, err := generateTestCSR(t, "test-sign")
	require.NoError(t, err)

	// Extract the public key from the CSR.
	block, _ := pem.Decode([]byte(csrPEM))
	require.NotNil(t, block)
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	require.NoError(t, err)
	publicKey, ok := csr.PublicKey.(*ecdsa.PublicKey)
	require.True(t, ok)

	transcript := []byte("test transcript bytes for verification")
	proof, err := signTranscript(privateKey, transcript)
	require.NoError(t, err)

	// Decode the base64url signature.
	signature, err := base64.RawURLEncoding.DecodeString(proof)
	require.NoError(t, err)

	// Verify as the gateway would.
	digest := sha256.Sum256(transcript)
	assert.True(t, ecdsa.VerifyASN1(publicKey, digest[:], signature), "signature must verify against the CSR public key")
}

// --- Pending state persistence ---

// TestOperatorPendingState_PersistAndLoad verifies the pending state
// is persisted with 0600 permissions and can be loaded back.
func TestOperatorPendingState_PersistAndLoad(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	client, err := NewOperatorPlatformEnrollmentClient("http://localhost:8080", "op-instance", "op-host", fileSvc, testLogger())
	require.NoError(t, err)

	state := &operatorPendingState{
		RequestID:           "req-123",
		Token:               "secret-token",
		OperatorFingerprint: "op-fp",
		CLIFingerprint:      "cli-fp",
		OperatorKeyPEM:      "-----BEGIN EC PRIVATE KEY-----\nfake\n-----END EC PRIVATE KEY-----\n",
		CLIKeyPEM:           "-----BEGIN EC PRIVATE KEY-----\nfake\n-----END EC PRIVATE KEY-----\n",
		ExpiresAt:           time.Now().Add(30 * time.Minute).UTC(),
		InstanceID:          "op-instance",
		Hostname:            "op-host",
	}

	pendingPath := client.pendingStatePath()
	err = client.persistPendingState(pendingPath, state)
	require.NoError(t, err)

	// Verify 0600 permissions.
	absPath := fileSvc.Resolve(pendingPath)
	info, err := fileSvc.Stat(context.Background(), pendingPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), info.Mode().Perm(), "pending state must be 0600 at %s", absPath)

	// Load and verify round-trip.
	loaded, err := client.loadPendingState(pendingPath)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, state.RequestID, loaded.RequestID)
	assert.Equal(t, state.Token, loaded.Token)
	assert.Equal(t, state.OperatorFingerprint, loaded.OperatorFingerprint)
	assert.Equal(t, state.CLIFingerprint, loaded.CLIFingerprint)
	assert.Equal(t, state.OperatorKeyPEM, loaded.OperatorKeyPEM)
	assert.Equal(t, state.CLIKeyPEM, loaded.CLIKeyPEM)
	assert.Equal(t, state.InstanceID, loaded.InstanceID)
	assert.Equal(t, state.Hostname, loaded.Hostname)
	assert.True(t, state.ExpiresAt.Equal(loaded.ExpiresAt))
}

// TestOperatorPendingState_LoadReturnsNilWhenNoFile verifies the load
// returns nil (not an error) when no pending state file exists.
func TestOperatorPendingState_LoadReturnsNilWhenNoFile(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	client, err := NewOperatorPlatformEnrollmentClient("http://localhost:8080", "op-instance", "op-host", fileSvc, testLogger())
	require.NoError(t, err)

	loaded, err := client.loadPendingState(client.pendingStatePath())
	require.NoError(t, err)
	assert.Nil(t, loaded)
}

// TestOperatorPendingState_RemoveIsIdempotent verifies removing a
// non-existent pending state file does not error.
func TestOperatorPendingState_RemoveIsIdempotent(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	client, err := NewOperatorPlatformEnrollmentClient("http://localhost:8080", "op-instance", "op-host", fileSvc, testLogger())
	require.NoError(t, err)

	err = client.removePendingState(client.pendingStatePath())
	assert.NoError(t, err)
}

// --- Full enroll flow against a mock gateway ---

// mockGateway is a test HTTP server that simulates the gateway's
// platform enrollment endpoints for the operator component.
type mockGateway struct {
	t               *testing.T
	server          *httptest.Server
	requestID       string
	token           string
	approveCh       chan struct{}
	operatorCertPEM string
	cliCertPEM      string
	trustBundlePEM  string
	operatorID      string
	operatorSession string
	cliSession      string
	posture         string
}

func newMockGateway(t *testing.T) *mockGateway {
	mg := &mockGateway{
		t:               t,
		requestID:       "mock-req-" + hex.EncodeToString([]byte{1, 2, 3, 4}),
		token:           "mock-token-" + hex.EncodeToString([]byte{5, 6, 7, 8}),
		approveCh:       make(chan struct{}),
		operatorID:      "op-uuid-123",
		operatorSession: "op-session-456",
		cliSession:      "cli-session-789",
		posture:         "doctrine",
	}

	// Generate self-signed certs for the response.
	mg.operatorCertPEM = generateSelfSignedCertPEM(t, "g8e-operator-test")
	mg.cliCertPEM = generateSelfSignedCertPEM(t, "g8e-cli-test")
	mg.trustBundlePEM = generateSelfSignedCertPEM(t, "g8e-ca-test")

	mux := http.NewServeMux()
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentRequest, mg.handleRequest)
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentStatus, mg.handleStatus)
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentComplete, mg.handleComplete)
	mg.server = httptest.NewServer(mux)
	t.Cleanup(mg.server.Close)

	return mg
}

func (mg *mockGateway) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.PlatformEnrollmentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.ComponentKind != models.PlatformComponentOperator {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Operator == nil || req.Operator.OperatorCSRPEM == "" || req.Operator.CLICSRPEM == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.SystemFingerprint == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	resp := models.PlatformEnrollmentCreateResponse{
		RequestID:     mg.requestID,
		Token:         mg.token,
		ComponentKind: models.PlatformComponentOperator,
		ComponentName: models.PlatformOperatorName,
		Fingerprints: models.PlatformEnrollmentCSRFingerprints{
			Operator: "op-fp-placeholder",
			CLI:      "cli-fp-placeholder",
		},
		ApprovalURL: mg.server.URL + "/console/",
		ExpiresAt:   time.Now().Add(30 * time.Minute).UTC(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (mg *mockGateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	token := r.URL.Query().Get("token")
	if token != mg.token {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// Wait for approval signal.
	select {
	case <-mg.approveCh:
		resp := models.PlatformEnrollmentStatusResponse{
			RequestID:     mg.requestID,
			ComponentKind: models.PlatformComponentOperator,
			State:         models.PlatformEnrollmentStateApproved,
			ExpiresAt:     time.Now().Add(25 * time.Minute).UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	default:
		resp := models.PlatformEnrollmentStatusResponse{
			RequestID:     mg.requestID,
			ComponentKind: models.PlatformComponentOperator,
			State:         models.PlatformEnrollmentStatePending,
			ExpiresAt:     time.Now().Add(25 * time.Minute).UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "1")
		json.NewEncoder(w).Encode(resp)
	}
}

func (mg *mockGateway) handleComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req models.PlatformEnrollmentCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Token != mg.token {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if req.Proofs.Operator == "" || req.Proofs.CLI == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	resp := models.PlatformEnrollmentCompleteResponse{
		RequestID:     mg.requestID,
		ComponentKind: models.PlatformComponentOperator,
		Operator: &models.PlatformEnrollmentOperatorCredentials{
			OperatorCert:      mg.operatorCertPEM,
			OperatorCertChain: "",
			HubTrustBundle:    mg.trustBundlePEM,
			OperatorSessionID: mg.operatorSession,
			OperatorID:        mg.operatorID,
			CLISessionID:      mg.cliSession,
			CLICert:           mg.cliCertPEM,
			CLICertChain:      "",
			Posture:           mg.posture,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// approve signals the mock gateway to return "approved" on the next
// status poll.
func (mg *mockGateway) approve() {
	close(mg.approveCh)
}

// TestOperatorEnroll_FullFlowWithApproval verifies the full nine-step
// enrollment sequence against a mock gateway: generate keys, submit
// request, persist pending state, poll until approved, sign transcript
// with both keys, submit completion, write credentials, and return the
// resolved identity.
func TestOperatorEnroll_FullFlowWithApproval(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	mg := newMockGateway(t)

	client, err := NewOperatorPlatformEnrollmentClient(mg.server.URL, "op-test-instance", "op-test-host", fileSvc, testLogger())
	require.NoError(t, err)

	// Approve after a short delay to simulate owner approval.
	go func() {
		time.Sleep(100 * time.Millisecond)
		mg.approve()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.Enroll(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the returned identity.
	assert.Equal(t, mg.operatorID, result.OperatorID)
	assert.Equal(t, mg.operatorSession, result.OperatorSessionID)
	assert.Equal(t, mg.cliSession, result.CLISessionID)
	assert.Equal(t, mg.posture, result.Posture)

	// Verify credentials were written to disk.
	assert.FileExists(t, fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)))
	assert.FileExists(t, fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey)))
	assert.FileExists(t, fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.CliCertFilename)))
	assert.FileExists(t, fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.CliKeyFilename)))
	assert.FileExists(t, fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle)))

	// Verify pending state was removed after successful enrollment.
	_, err = fileSvc.Stat(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirPendingEnroll, constants.PendingEnrollmentFileOperator))
	assert.Error(t, err, "pending state must be removed after successful enrollment")

	// Verify credential permissions are 0600.
	opCertInfo, err := fileSvc.Stat(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), opCertInfo.Mode().Perm())

	opKeyInfo, err := fileSvc.Stat(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFilePrivate), opKeyInfo.Mode().Perm())

	// Trust bundle is 0644 (public).
	bundleInfo, err := fileSvc.Stat(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(constants.PermFilePublic), bundleInfo.Mode().Perm())
}

// TestOperatorEnroll_ResumeFromPendingState verifies that when a
// pending state file exists, the client resumes the same request
// without generating new keys. This is the kill-and-restart property.
func TestOperatorEnroll_ResumeFromPendingState(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	mg := newMockGateway(t)

	// Generate keys and persist a pending state manually.
	csrPEM, opKey, err := generateTestCSR(t, "g8e-operator-resume")
	require.NoError(t, err)
	opFP, err := csrFingerprint(csrPEM)
	require.NoError(t, err)
	opKeyPEM, err := encodeECPrivateKeyPEM(opKey)
	require.NoError(t, err)

	cliCSRPEM, cliKey, err := generateTestCSR(t, "g8e-cli-resume")
	require.NoError(t, err)
	cliFP, err := csrFingerprint(cliCSRPEM)
	require.NoError(t, err)
	cliKeyPEM, err := encodeECPrivateKeyPEM(cliKey)
	require.NoError(t, err)

	originalRequestID := "preexisting-req-id"
	originalToken := "preexisting-token"
	pending := &operatorPendingState{
		RequestID:           originalRequestID,
		Token:               originalToken,
		OperatorFingerprint: opFP,
		CLIFingerprint:      cliFP,
		OperatorKeyPEM:      opKeyPEM,
		CLIKeyPEM:           cliKeyPEM,
		ExpiresAt:           time.Now().Add(30 * time.Minute).UTC(),
		InstanceID:          "op-test-instance",
		Hostname:            "op-test-host",
	}

	client, err := NewOperatorPlatformEnrollmentClient(mg.server.URL, "op-test-instance", "op-test-host", fileSvc, testLogger())
	require.NoError(t, err)

	err = client.persistPendingState(client.pendingStatePath(), pending)
	require.NoError(t, err)

	// Override the mock gateway to use the preexisting request ID and
	// token so the status and completion endpoints recognize the
	// resumed request.
	mg.requestID = originalRequestID
	mg.token = originalToken

	// Approve immediately.
	mg.approve()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := client.Enroll(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// The result should carry the mock gateway's operator ID/session,
	// proving the completion endpoint was reached with the original
	// token.
	assert.Equal(t, mg.operatorID, result.OperatorID)
	assert.Equal(t, mg.operatorSession, result.OperatorSessionID)
}

// TestOperatorEnroll_DenialFailsClosed verifies that a denied request
// causes enrollment to fail with a clear error and leaves no
// credentials on disk.
func TestOperatorEnroll_DenialFailsClosed(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	mg := newMockGateway(t)

	// Override the status handler to return "denied".
	close(mg.approveCh) // prevent approval
	mg.t = t
	mux := http.NewServeMux()
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentRequest, mg.handleRequest)
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentStatus, func(w http.ResponseWriter, r *http.Request) {
		resp := models.PlatformEnrollmentStatusResponse{
			RequestID:     mg.requestID,
			ComponentKind: models.PlatformComponentOperator,
			State:         models.PlatformEnrollmentStateDenied,
			ExpiresAt:     time.Now().Add(25 * time.Minute).UTC(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc(constants.APIPaths.AuthPlatformEnrollmentComplete, mg.handleComplete)
	mg.server.Close()
	mg.server = httptest.NewServer(mux)
	mg.t.Cleanup(mg.server.Close)

	client, err := NewOperatorPlatformEnrollmentClient(mg.server.URL, "op-test-instance", "op-test-host", fileSvc, testLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.Enroll(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")

	// No credentials should be on disk.
	assert.NoFileExists(t, fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiFileOperatorCert)))
	assert.NoFileExists(t, fileSvc.Resolve(filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey)))

	// Pending state should still exist (denial is terminal but the
	// client leaves it so the operator doesn't silently re-generate
	// keys on restart).
	exists, err := fileSvc.FileExists(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirPendingEnroll, constants.PendingEnrollmentFileOperator))
	require.NoError(t, err)
	assert.True(t, exists, "pending state should remain after denial so restart doesn't silently generate new keys")
}

// --- Helpers ---

// generateTestCSR generates a P-256 CSR and returns the PEM, the
// private key, and the public key.
func generateTestCSR(t *testing.T, commonName string) (string, *ecdsa.PrivateKey, error) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, err
	}
	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"g8e"},
		},
	}
	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &template, privKey)
	if err != nil {
		return "", nil, err
	}
	csrPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	}))
	return csrPEM, privKey, nil
}

// generateSelfSignedCertPEM generates a self-signed cert and returns
// its PEM encoding. Used for mock gateway responses.
func generateSelfSignedCertPEM(t *testing.T, commonName string) string {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}))
}

// repeatChar returns a string of n repetitions of c.
func repeatChar(c string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += c
	}
	return result
}

