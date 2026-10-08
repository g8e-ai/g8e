// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package serve

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/testutil"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestOperatorEnroll_FullFlowWithApproval verifies the full nine-step
// enrollment sequence against a mock gateway: generate keys, submit
// request, persist pending state, wait for approval, sign transcript
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
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, opCertInfo.IsDir()), opCertInfo.Mode().Perm())

	opKeyInfo, err := fileSvc.Stat(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiFileOperatorKey))
	require.NoError(t, err)
	assert.Equal(t, testutil.FileMode(constants.PermFilePrivate, opKeyInfo.IsDir()), opKeyInfo.Mode().Perm())

	// Trust bundle is 0644 (public).
	bundleInfo, err := fileSvc.Stat(context.Background(), filepath.Join(constants.PkiDirname, constants.PkiSubdirTrust, constants.PkiFileGatewayBundle))
	require.NoError(t, err)
	assert.Equal(t, testutil.FileMode(constants.PermFilePublic, bundleInfo.IsDir()), bundleInfo.Mode().Perm())
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
	recorder, err := NewOperatorDeploymentRecorder(fileSvc, "resume-launch")
	require.NoError(t, err)
	client.SetDeploymentRecorder(recorder)

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
	data, err := fileSvc.ReadFile(ctx, operatorDeploymentStateRelPath())
	require.NoError(t, err)
	var progress models.OperatorDeploymentState
	require.NoError(t, json.Unmarshal(data, &progress))
	assert.Equal(t, "resume-launch", progress.LaunchID)
	assert.Equal(t, originalRequestID, progress.RequestID)
	assert.Equal(t, mg.operatorSession, progress.OperatorSessionID)
	assert.Equal(t, models.OperatorDeploymentPhaseEnrolled, progress.Phase)
	assert.NotContains(t, string(data), originalToken)
	assert.NotContains(t, string(data), opKeyPEM)
	assert.NotContains(t, string(data), cliKeyPEM)
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
