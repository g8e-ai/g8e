// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// samplePendingResponse builds a PlatformEnrollmentPendingResponse with one
// operator request and one dashboard request for testing.
func samplePendingResponse() models.PlatformEnrollmentPendingResponse {
	return models.PlatformEnrollmentPendingResponse{
		Requests: []models.PlatformEnrollmentPendingRequest{
			{
				RequestID:         "req-operator-001",
				ComponentKind:     models.PlatformComponentOperator,
				ComponentName:     models.PlatformOperatorName,
				InstanceID:        "operator-host-01",
				Hostname:          "operator.example.com",
				SystemFingerprint: "sys-fp-abc123",
				Fingerprints: models.PlatformEnrollmentCSRFingerprints{
					Operator: "op-fp-sha256-aaa",
					CLI:      "cli-fp-sha256-bbb",
				},
				State:     models.PlatformEnrollmentStatePending,
				CreatedAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
				ExpiresAt: time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
			},
			{
				RequestID:     "req-dashboard-002",
				ComponentKind: models.PlatformComponentDashboard,
				ComponentName: models.PlatformDashboardName,
				InstanceID:    "dashboard-host-01",
				Hostname:      "dashboard.example.com",
				Fingerprints: models.PlatformEnrollmentCSRFingerprints{
					App: "app-fp-sha256-ccc",
				},
				State:     models.PlatformEnrollmentStatePending,
				CreatedAt: time.Date(2026, 8, 20, 10, 5, 0, 0, time.UTC),
				ExpiresAt: time.Date(2026, 8, 20, 11, 5, 0, 0, time.UTC),
			},
		},
	}
}

func TestRevokePlatformEnrollmentCmdWithConfig_PostsRequestID(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	responseBody, err := json.Marshal(models.PlatformEnrollmentRevokeResponse{
		RequestID:     "req-operator-001",
		ComponentKind: models.PlatformComponentOperator,
		State:         models.PlatformEnrollmentStateRevoked,
	})
	require.NoError(t, err)
	mockClient := &mockAPIClient{postResp: responseBody}

	cmd := revokePlatformEnrollmentCmdWithConfig(configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	cmd.Flags().Set("reason", "host retired")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.NoError(t, err)
	require.Len(t, mockClient.postCalls, 1)
	assert.Equal(t, constants.APIPaths.AuthPlatformEnrollmentRevoke, mockClient.postCalls[0].path)
	request := mockClient.postCalls[0].body.(models.PlatformEnrollmentRevokeRequest)
	assert.Equal(t, "req-operator-001", request.RequestID)
	assert.Equal(t, "host retired", request.Reason)
	assert.Contains(t, buf.String(), "revoked")
}

// --- enroll approve command tests ---

// TestApprovePlatformEnrollmentCmd_ApproveWithYes verifies that --yes skips the
// interactive prompt, posts an approve decision with the correct request ID,
// and prints the resulting state.
func TestApprovePlatformEnrollmentCmd_ApproveWithYes(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	decisionResp := models.PlatformEnrollmentDecisionResponse{
		RequestID: "req-operator-001",
		State:     models.PlatformEnrollmentStateApproved,
	}
	postResp, err := json.Marshal(decisionResp)
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody, postResp: postResp}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.NoError(t, err)

	require.Len(t, mockClient.postCalls, 1)
	assert.Equal(t, constants.APIPaths.AuthPlatformEnrollmentDecision, mockClient.postCalls[0].path)
	decisionReq := mockClient.postCalls[0].body.(models.PlatformEnrollmentDecisionRequest)
	assert.Equal(t, "req-operator-001", decisionReq.RequestID)
	assert.Equal(t, models.PlatformEnrollmentDecisionApprove, decisionReq.Decision)
	assert.Contains(t, buf.String(), "approved")
}

// TestDenyPlatformEnrollmentCmd_WithYes verifies that the deny subcommand posts a
// deny decision and prints the denied state.
func TestDenyPlatformEnrollmentCmd_WithYes(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	decisionResp := models.PlatformEnrollmentDecisionResponse{
		RequestID: "req-dashboard-002",
		State:     models.PlatformEnrollmentStateDenied,
	}
	postResp, err := json.Marshal(decisionResp)
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody, postResp: postResp}

	cmd := denyPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-dashboard-002"})
	require.NoError(t, err)

	require.Len(t, mockClient.postCalls, 1)
	decisionReq := mockClient.postCalls[0].body.(models.PlatformEnrollmentDecisionRequest)
	assert.Equal(t, models.PlatformEnrollmentDecisionDeny, decisionReq.Decision)
	assert.Contains(t, buf.String(), "denied")
}

// TestApprovePlatformEnrollmentCmd_ReasonIncludedInBody verifies that the
// --reason flag value is included in the posted decision request body.
func TestApprovePlatformEnrollmentCmd_ReasonIncludedInBody(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	postResp, err := json.Marshal(models.PlatformEnrollmentDecisionResponse{
		RequestID: "req-operator-001",
		State:     models.PlatformEnrollmentStateApproved,
	})
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody, postResp: postResp}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	cmd.Flags().Set("reason", "approved by owner via CLI")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.NoError(t, err)

	decisionReq := mockClient.postCalls[0].body.(models.PlatformEnrollmentDecisionRequest)
	assert.Equal(t, "approved by owner via CLI", decisionReq.Reason)
}

// TestApprovePlatformEnrollmentCmd_DisplaysRequestDetails verifies that the
// command output includes component kind, hostname, instance ID, CSR
// fingerprints, creation time, and expiry before posting the decision.
func TestApprovePlatformEnrollmentCmd_DisplaysRequestDetails(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	postResp, err := json.Marshal(models.PlatformEnrollmentDecisionResponse{
		RequestID: "req-operator-001",
		State:     models.PlatformEnrollmentStateApproved,
	})
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody, postResp: postResp}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "operator")
	assert.Contains(t, output, "operator.example.com")
	assert.Contains(t, output, "operator-host-01")
	assert.Contains(t, output, "op-fp-sha256-aaa")
	assert.Contains(t, output, "cli-fp-sha256-bbb")
	assert.Contains(t, output, "Created:")
	assert.Contains(t, output, "Expires:")
}

// TestApprovePlatformEnrollmentCmd_RequestNotFoundReturnsError verifies that a
// request ID not in the pending list returns ErrPlatformEnrollmentRequestNotFound.
func TestApprovePlatformEnrollmentCmd_RequestNotFoundReturnsError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"nonexistent-req"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentRequestNotFound)
	assert.Empty(t, mockClient.postCalls, "decision must not be posted for a non-existent request")
}

// TestApprovePlatformEnrollmentCmd_GetErrorReturnsWrappedError verifies that a
// pending-list fetch failure is surfaced as a wrapped error.
func TestApprovePlatformEnrollmentCmd_GetErrorReturnsWrappedError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	getErr := fmt.Errorf("network failure")
	mockClient := &mockAPIClient{getErr: getErr}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-operator-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, getErr)
	assert.Empty(t, mockClient.postCalls, "decision must not be posted when pending fetch fails")
}

// TestApprovePlatformEnrollmentCmd_PostErrorReturnsWrappedError verifies that a
// decision post failure is surfaced as a wrapped error.
func TestApprovePlatformEnrollmentCmd_PostErrorReturnsWrappedError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	postErr := fmt.Errorf("post failure")
	mockClient := &mockAPIClient{getResp: pendingBody, postErr: postErr}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, postErr)
}

// TestApprovePlatformEnrollmentCmd_RejectsMissingRequestID verifies that the
// command rejects a missing request ID argument via cobra's Args validation.
func TestApprovePlatformEnrollmentCmd_RejectsMissingRequestID(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	mockClient := &mockAPIClient{}
	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.SetArgs([]string{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Empty(t, mockClient.getCalls, "client must not be called when args validation fails")
	assert.Empty(t, mockClient.postCalls, "client must not be called when args validation fails")
}

// TestApprovePlatformEnrollmentCmd_OutputNeverContainsTokens verifies that the
// command output never includes requester tokens, token hashes, CSR PEM, or
// certificates — only request ID, component metadata, fingerprints, and
// timestamps.
func TestApprovePlatformEnrollmentCmd_OutputNeverContainsTokens(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	postResp, err := json.Marshal(models.PlatformEnrollmentDecisionResponse{
		RequestID: "req-operator-001",
		State:     models.PlatformEnrollmentStateApproved,
	})
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody, postResp: postResp}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.NoError(t, err)

	output := buf.String()
	for _, forbidden := range []string{"token", "Token", "BEGIN CERTIFICATE", "BEGIN CERTIFICATE REQUEST", "csr_pem"} {
		assert.NotContains(t, output, forbidden, "output must not contain %q", forbidden)
	}
}

// TestApprovePlatformEnrollmentCmd_InteractiveAbort verifies that answering
// "n" to the interactive prompt aborts the decision without posting.
func TestApprovePlatformEnrollmentCmd_InteractiveAbort(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody}

	// Replace stdin with a pipe that sends "n\n".
	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = origStdin; r.Close(); w.Close() })
	go func() { _, _ = w.WriteString("n\n"); _ = w.Close() }()

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Aborted")
	assert.Empty(t, mockClient.postCalls, "decision must not be posted on abort")
}

// TestApprovePlatformEnrollmentCmd_ReasonTooLongReturnsError verifies that a
// --reason value exceeding PlatformEnrollmentMaxReasonBytes returns a
// validation error before posting.
func TestApprovePlatformEnrollmentCmd_ReasonTooLongReturnsError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	longReason := strings.Repeat("x", constants.PlatformEnrollmentMaxReasonBytes+1)
	cmd.Flags().Set("reason", longReason)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPlatformEnrollmentReasonTooLong)
	assert.Empty(t, mockClient.postCalls, "decision must not be posted when reason validation fails")
}

// --- pending command tests ---

// TestPendingPlatformEnrollmentCmd_ListsRequests verifies that the command
// fetches the pending list and prints request metadata for each entry.
func TestPendingPlatformEnrollmentCmd_ListsRequests(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody}

	cmd := pendingPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, nil)
	require.NoError(t, err)

	require.Len(t, mockClient.getCalls, 1)
	assert.Equal(t, constants.APIPaths.AuthPlatformEnrollmentPending, mockClient.getCalls[0])

	output := buf.String()
	assert.Contains(t, output, "req-operator-001")
	assert.Contains(t, output, "operator")
	assert.Contains(t, output, "operator.example.com")
	assert.Contains(t, output, "operator-host-01")
	assert.Contains(t, output, "req-dashboard-002")
	assert.Contains(t, output, "dashboard")
	assert.Contains(t, output, "dashboard.example.com")
}

// TestPendingPlatformEnrollmentCmd_EmptyListPrintsMessage verifies that an
// empty pending list prints a clear "no pending" message.
func TestPendingPlatformEnrollmentCmd_EmptyListPrintsMessage(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(models.PlatformEnrollmentPendingResponse{Requests: nil})
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody}

	cmd := pendingPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No pending platform enrollment requests")
}

// TestPendingPlatformEnrollmentCmd_GetErrorReturnsWrappedError verifies that a
// fetch failure is surfaced as a wrapped error.
func TestPendingPlatformEnrollmentCmd_GetErrorReturnsWrappedError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	getErr := fmt.Errorf("network failure")
	mockClient := &mockAPIClient{getErr: getErr}

	cmd := pendingPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, getErr)
}

// TestPendingPlatformEnrollmentCmd_OutputNeverContainsTokens verifies that the
// command output never includes requester tokens, token hashes, CSR PEM, or
// certificates.
func TestPendingPlatformEnrollmentCmd_OutputNeverContainsTokens(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody}

	cmd := pendingPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	for _, forbidden := range []string{"token", "Token", "BEGIN CERTIFICATE", "BEGIN CERTIFICATE REQUEST", "csr_pem"} {
		assert.NotContains(t, output, forbidden, "output must not contain %q", forbidden)
	}
}

// TestPendingPlatformEnrollmentCmd_InvalidJSONReturnsError verifies that a
// malformed response body returns a wrapped parse error.
func TestPendingPlatformEnrollmentCmd_InvalidJSONReturnsError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	mockClient := &mockAPIClient{getResp: []byte("not json {{{")}

	cmd := pendingPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

// TestPendingPlatformEnrollmentCmd_ClientFactoryError verifies that a client
// factory failure is surfaced as a wrapped error and no GET is attempted.
func TestPendingPlatformEnrollmentCmd_ClientFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	clientErr := fmt.Errorf("client factory boom")
	factory := func(_ fs.RuntimeFileService, _ *config.Config) (apiClient, error) {
		return nil, clientErr
	}

	cmd := pendingPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), factory, fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, clientErr)
}

// TestApprovePlatformEnrollmentCmd_ClientFactoryError verifies that a client
// factory failure is surfaced as a wrapped error and no GET is attempted.
func TestApprovePlatformEnrollmentCmd_ClientFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	clientErr := fmt.Errorf("client factory boom")
	factory := func(_ fs.RuntimeFileService, _ *config.Config) (apiClient, error) {
		return nil, clientErr
	}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), factory, fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-operator-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, clientErr)
}

// TestApprovePlatformEnrollmentCmd_InvalidPendingJSONReturnsError verifies
// that a malformed pending-list response returns a wrapped parse error.
func TestApprovePlatformEnrollmentCmd_InvalidPendingJSONReturnsError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	mockClient := &mockAPIClient{getResp: []byte("not json {{{")}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-operator-001"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse pending list")
}

// TestApprovePlatformEnrollmentCmd_InvalidDecisionJSONReturnsError verifies
// that a malformed decision response returns a wrapped parse error.
func TestApprovePlatformEnrollmentCmd_InvalidDecisionJSONReturnsError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody, postResp: []byte("not json {{{")}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse response")
}

// TestApprovePlatformEnrollmentCmd_ConfigLoaderError verifies that a config
// load failure is returned directly.
func TestApprovePlatformEnrollmentCmd_ConfigLoaderError(t *testing.T) {
	cfgErr := fmt.Errorf("config load error")
	loader := func(string) (*config.Config, error) { return nil, cfgErr }

	cmd := approvePlatformEnrollmentCmdWithConfig(
		loader, panickingClientFactory(), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, cfgErr)
}

// TestPendingPlatformEnrollmentCmd_ConfigLoaderError verifies that a config
// load failure is returned directly.
func TestPendingPlatformEnrollmentCmd_ConfigLoaderError(t *testing.T) {
	cfgErr := fmt.Errorf("config load error")
	loader := func(string) (*config.Config, error) { return nil, cfgErr }

	cmd := pendingPlatformEnrollmentCmdWithConfig(
		loader, panickingClientFactory(), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, cfgErr)
}

// TestApprovePlatformEnrollmentCmd_DisplaysSystemFingerprint verifies that the
// system fingerprint is shown for operator requests (which carry it) but not
// for dashboard requests (which do not).
func TestApprovePlatformEnrollmentCmd_DisplaysSystemFingerprint(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	postResp, err := json.Marshal(models.PlatformEnrollmentDecisionResponse{
		RequestID: "req-operator-001",
		State:     models.PlatformEnrollmentStateApproved,
	})
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody, postResp: postResp}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "sys-fp-abc123")
}

// TestApprovePlatformEnrollmentCmd_CommandStructure verifies the command's
// Use, flags, and Args validation.
func TestApprovePlatformEnrollmentCmd_CommandStructure(t *testing.T) {
	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(nil), panickingClientFactory(), fileSvcFactoryFor(nil))
	assert.Equal(t, "approve <request-id>", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.Nil(t, cmd.Flags().Lookup("deny"))
	assert.NotNil(t, cmd.Flags().Lookup("reason"))
	assert.NotNil(t, cmd.Flags().Lookup("yes"))
	assert.Equal(t, "false", cmd.Flags().Lookup("yes").DefValue)
}

// TestDenyPlatformEnrollmentCmd_CommandStructure verifies the deny command's
// Use, flags, and Args validation.
func TestDenyPlatformEnrollmentCmd_CommandStructure(t *testing.T) {
	cmd := denyPlatformEnrollmentCmdWithConfig(
		configLoaderFor(nil), panickingClientFactory(), fileSvcFactoryFor(nil))
	assert.Equal(t, "deny <request-id>", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.Nil(t, cmd.Flags().Lookup("deny"))
	assert.NotNil(t, cmd.Flags().Lookup("reason"))
	assert.NotNil(t, cmd.Flags().Lookup("yes"))
}

// TestPendingPlatformEnrollmentCmd_CommandStructure verifies the command's
// Use and Args validation.
func TestPendingPlatformEnrollmentCmd_CommandStructure(t *testing.T) {
	cmd := pendingPlatformEnrollmentCmdWithConfig(
		configLoaderFor(nil), panickingClientFactory(), fileSvcFactoryFor(nil))
	assert.Equal(t, "pending", cmd.Use)
	assert.NotNil(t, cmd.RunE)
}

// TestApprovePlatformEnrollmentCmd_ApproveWithYes_FetchesPendingFirst verifies
// that the command GETs the pending list before POSTing the decision, proving
// the display step happens before the decision.
func TestApprovePlatformEnrollmentCmd_ApproveWithYes_FetchesPendingFirst(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	pendingBody, err := json.Marshal(samplePendingResponse())
	require.NoError(t, err)

	postResp, err := json.Marshal(models.PlatformEnrollmentDecisionResponse{
		RequestID: "req-operator-001",
		State:     models.PlatformEnrollmentStateApproved,
	})
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: pendingBody, postResp: postResp}

	cmd := approvePlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	cmd.Flags().Set("yes", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, []string{"req-operator-001"})
	require.NoError(t, err)

	require.Len(t, mockClient.getCalls, 1, "must GET the pending list before deciding")
	assert.Equal(t, constants.APIPaths.AuthPlatformEnrollmentPending, mockClient.getCalls[0])
	require.Len(t, mockClient.postCalls, 1, "must POST exactly one decision")
}

func sampleEnrolledResponse() models.PlatformEnrollmentEnrolledResponse {
	completedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	return models.PlatformEnrollmentEnrolledResponse{
		Enrollments: []models.PlatformEnrollmentEnrolledRequest{
			{
				RequestID:         "req-operator-completed-001",
				ComponentKind:     models.PlatformComponentOperator,
				ComponentName:     models.PlatformOperatorName,
				InstanceID:        "operator-host-01",
				Hostname:          "operator.example.com",
				State:             models.PlatformEnrollmentStateCompleted,
				OperatorID:        "59182945-2709-47f6-b573-049b9c92a19e",
				OperatorSessionID: "3b750b7a-6d88-493a-80b5-fe715a9b8525",
				CLISessionID:      "cli-session-001",
				CompletedAt:       &completedAt,
			},
			{
				RequestID:     "req-dashboard-revoked-002",
				ComponentKind: models.PlatformComponentDashboard,
				ComponentName: models.PlatformDashboardName,
				InstanceID:    "dashboard-host-01",
				Hostname:      "dashboard.example.com",
				State:         models.PlatformEnrollmentStateRevoked,
				PolicyID:      "spiffe://g8e.local/app/g8ed",
				CompletedAt:   &completedAt,
				RevokedAt:     &completedAt,
			},
		},
	}
}

func TestListPlatformEnrollmentCmd_ListsEnrollments(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	enrolledBody, err := json.Marshal(sampleEnrolledResponse())
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: enrolledBody}

	cmd := listPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, nil)
	require.NoError(t, err)

	require.Len(t, mockClient.getCalls, 1)
	assert.Equal(t, constants.APIPaths.AuthPlatformEnrollmentEnrolled, mockClient.getCalls[0])

	output := buf.String()
	assert.Contains(t, output, "req-operator-completed-001")
	assert.Contains(t, output, "req-dashboard-revoked-002")
	assert.Contains(t, output, "operator.example.com")
	assert.Contains(t, output, "59182945-2709-47f6-b573-049b9c92a19e")
	assert.NotContains(t, output, "3b750b7a-6d88-493a-80b5-fe715a9b8525")
}

func TestListPlatformEnrollmentCmd_EmptyListPrintsMessage(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	enrolledBody, err := json.Marshal(models.PlatformEnrollmentEnrolledResponse{Enrollments: nil})
	require.NoError(t, err)

	mockClient := &mockAPIClient{getResp: enrolledBody}

	cmd := listPlatformEnrollmentCmdWithConfig(
		configLoaderFor(cfg), mockClientFactory(mockClient), fileSvcFactoryFor(nil))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err = cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No completed platform enrollment requests")
}

func TestListPlatformEnrollmentCmd_CommandStructure(t *testing.T) {
	cmd := listPlatformEnrollmentCmdWithConfig(
		configLoaderFor(nil), panickingClientFactory(), fileSvcFactoryFor(nil))
	assert.Equal(t, "list", cmd.Use)
	assert.NotNil(t, cmd.RunE)
}
