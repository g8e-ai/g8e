// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

func TestFindPendingRequestByComponent_ReturnsMatchingRequest(t *testing.T) {
	ensembleReq := models.PlatformEnrollmentPendingRequest{
		RequestID:     "req-ensemble",
		ComponentKind: models.PlatformComponentEnsemble,
		Hostname:      "ensemble.local",
	}
	dashboardReq := models.PlatformEnrollmentPendingRequest{
		RequestID:     "req-dashboard",
		ComponentKind: models.PlatformComponentDashboard,
		Hostname:      "dashboard.local",
	}
	operatorReq := models.PlatformEnrollmentPendingRequest{
		RequestID:     "req-operator",
		ComponentKind: models.PlatformComponentOperator,
		Hostname:      "operator.local",
	}
	requests := []models.PlatformEnrollmentPendingRequest{ensembleReq, dashboardReq, operatorReq}

	tests := []struct {
		name      string
		component models.PlatformComponentKind
		wantID    string
		wantNil   bool
	}{
		{name: "ensemble", component: models.PlatformComponentEnsemble, wantID: "req-ensemble"},
		{name: "dashboard", component: models.PlatformComponentDashboard, wantID: "req-dashboard"},
		{name: "operator", component: models.PlatformComponentOperator, wantID: "req-operator"},
		{name: "not present", component: "nonexistent", wantNil: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findPendingRequestByComponent(requests, tc.component)
			if tc.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, tc.wantID, got.RequestID)
		})
	}
}

func TestFindPendingRequestByComponent_EmptyListReturnsNil(t *testing.T) {
	got := findPendingRequestByComponent(nil, models.PlatformComponentEnsemble)
	assert.Nil(t, got)
}

func TestFindPendingRequestByComponent_ReturnsFirstMatch(t *testing.T) {
	first := models.PlatformEnrollmentPendingRequest{
		RequestID:     "req-first",
		ComponentKind: models.PlatformComponentEnsemble,
	}
	second := models.PlatformEnrollmentPendingRequest{
		RequestID:     "req-second",
		ComponentKind: models.PlatformComponentEnsemble,
	}
	got := findPendingRequestByComponent([]models.PlatformEnrollmentPendingRequest{first, second}, models.PlatformComponentEnsemble)
	require.NotNil(t, got)
	assert.Equal(t, "req-first", got.RequestID)
}

// stubWalkthroughAPIClient is a configurable apiClient stub for walkthrough tests.
type stubWalkthroughAPIClient struct {
	getResponses  map[string][]byte
	getErr        error
	postResponses map[string][]byte
	postErr       error
	getCalls      []string
	postCalls     []walkthroughPostCall
}

type walkthroughPostCall struct {
	path string
	body interface{}
}

func (c *stubWalkthroughAPIClient) Get(path string) ([]byte, error) {
	c.getCalls = append(c.getCalls, path)
	if c.getErr != nil {
		return nil, c.getErr
	}
	if resp, ok := c.getResponses[path]; ok {
		return resp, nil
	}
	return nil, errors.New("unexpected GET path: " + path)
}

func (c *stubWalkthroughAPIClient) Post(path string, body interface{}) ([]byte, error) {
	c.postCalls = append(c.postCalls, walkthroughPostCall{path: path, body: body})
	if c.postErr != nil {
		return nil, c.postErr
	}
	if resp, ok := c.postResponses[path]; ok {
		return resp, nil
	}
	return nil, errors.New("unexpected POST path: " + path)
}

func (c *stubWalkthroughAPIClient) Put(path string, body interface{}) ([]byte, error) {
	return nil, nil
}

func (c *stubWalkthroughAPIClient) Delete(path string) ([]byte, error) {
	return nil, nil
}

func TestPromptApproveComponent_NoPendingRequestSkips(t *testing.T) {
	client := &stubWalkthroughAPIClient{
		getResponses: map[string][]byte{
			constants.APIPaths.AuthPlatformEnrollmentPending: mustMarshalPendingResp(t, nil),
		},
	}

	cmd := dockerStartCmd()
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := promptApproveComponent(cmd, context.Background(), client, models.PlatformComponentEnsemble)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No pending ensemble enrollment request found")
	assert.Empty(t, client.postCalls, "no POST should be made when there is no pending request")
}

func TestPromptApproveComponent_UserDeclinesPostsNothing(t *testing.T) {
	ensembleReq := models.PlatformEnrollmentPendingRequest{
		RequestID:     "req-ensemble-1",
		ComponentKind: models.PlatformComponentEnsemble,
		ComponentName: "g8ee",
		InstanceID:    "inst-1",
		Hostname:      "ensemble.local",
		State:         models.PlatformEnrollmentStatePending,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	client := &stubWalkthroughAPIClient{
		getResponses: map[string][]byte{
			constants.APIPaths.AuthPlatformEnrollmentPending: mustMarshalPendingResp(t, []models.PlatformEnrollmentPendingRequest{ensembleReq}),
		},
	}

	cmd := dockerStartCmd()
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(bytes.NewBufferString("n\n"))

	err := promptApproveComponent(cmd, context.Background(), client, models.PlatformComponentEnsemble)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Skipped ensemble enrollment")
	assert.Empty(t, client.postCalls, "no POST should be made when user declines")
}

func TestPromptApproveComponent_UserApprovesPostsDecision(t *testing.T) {
	ensembleReq := models.PlatformEnrollmentPendingRequest{
		RequestID:     "req-ensemble-1",
		ComponentKind: models.PlatformComponentEnsemble,
		ComponentName: "g8ee",
		InstanceID:    "inst-1",
		Hostname:      "ensemble.local",
		State:         models.PlatformEnrollmentStatePending,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	decisionResp := models.PlatformEnrollmentDecisionResponse{
		RequestID: "req-ensemble-1",
		State:     models.PlatformEnrollmentStateApproved,
	}
	client := &stubWalkthroughAPIClient{
		getResponses: map[string][]byte{
			constants.APIPaths.AuthPlatformEnrollmentPending: mustMarshalPendingResp(t, []models.PlatformEnrollmentPendingRequest{ensembleReq}),
		},
		postResponses: map[string][]byte{
			constants.APIPaths.AuthPlatformEnrollmentDecision: mustMarshalDecisionResp(t, decisionResp),
		},
	}

	cmd := dockerStartCmd()
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(bytes.NewBufferString("y\n"))

	err := promptApproveComponent(cmd, context.Background(), client, models.PlatformComponentEnsemble)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "ensemble enrollment request approved")
	require.Len(t, client.postCalls, 1)
	assert.Equal(t, constants.APIPaths.AuthPlatformEnrollmentDecision, client.postCalls[0].path)

	decision, ok := client.postCalls[0].body.(models.PlatformEnrollmentDecisionRequest)
	require.True(t, ok)
	assert.Equal(t, "req-ensemble-1", decision.RequestID)
	assert.Equal(t, models.PlatformEnrollmentDecisionApprove, decision.Decision)
}

func TestPromptApproveComponent_GetErrorReturnsError(t *testing.T) {
	client := &stubWalkthroughAPIClient{
		getErr: errors.New("network down"),
	}

	cmd := dockerStartCmd()
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := promptApproveComponent(cmd, context.Background(), client, models.PlatformComponentEnsemble)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch pending list")
}

func TestPromptApproveComponent_PostErrorWrapsApprovalFailed(t *testing.T) {
	ensembleReq := models.PlatformEnrollmentPendingRequest{
		RequestID:     "req-ensemble-1",
		ComponentKind: models.PlatformComponentEnsemble,
		ComponentName: "g8ee",
		InstanceID:    "inst-1",
		Hostname:      "ensemble.local",
		State:         models.PlatformEnrollmentStatePending,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(10 * time.Minute),
	}
	client := &stubWalkthroughAPIClient{
		getResponses: map[string][]byte{
			constants.APIPaths.AuthPlatformEnrollmentPending: mustMarshalPendingResp(t, []models.PlatformEnrollmentPendingRequest{ensembleReq}),
		},
		postErr: errors.New("server error"),
	}

	cmd := dockerStartCmd()
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetIn(bytes.NewBufferString("y\n"))

	err := promptApproveComponent(cmd, context.Background(), client, models.PlatformComponentEnsemble)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDockerStartApprovalFailed)
}

func TestRunDockerStartWalkthrough_EnrollmentFailureWrapsEnrollmentFailed(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	failingEnroller := &failingWalkthroughEnroller{err: errors.New("bootstrap failed")}
	deps := dockerStartDeps{
		clientFactory:        panickingClientFactory(),
		checkOperatorRunning: func(*config.Config) error { return nil },
		enrollerFactory: func(auth.OutputFunc, fs.RuntimeFileService, *config.Config) Enroller {
			return failingEnroller
		},
		waitGatewayHealthy: func(*cobra.Command) error { return nil },
		cfg:                cfg,
		fileSvc:            fileSvc,
	}

	cmd := dockerStartCmd()
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := runDockerStartWalkthrough(cmd, deps)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDockerStartEnrollmentFailed)
}

func TestRunDockerStartWalkthrough_OperatorNotRunningWrapsEnrollmentFailed(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	deps := dockerStartDeps{
		clientFactory: panickingClientFactory(),
		checkOperatorRunning: func(*config.Config) error {
			return errors.New("connection refused")
		},
		enrollerFactory:    panickingEnrollerFactory(),
		waitGatewayHealthy: func(*cobra.Command) error { return nil },
		cfg:                cfg,
		fileSvc:            fileSvc,
	}

	cmd := dockerStartCmd()
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := runDockerStartWalkthrough(cmd, deps)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrDockerStartEnrollmentFailed)
}

// failingWalkthroughEnroller is an Enroller stub that always returns the
// configured error from Enroll.
type failingWalkthroughEnroller struct {
	err error
}

func (e *failingWalkthroughEnroller) Enroll(_ context.Context, _ auth.EnrollmentOptions) (*auth.EnrollmentResult, error) {
	return nil, e.err
}

func mustMarshalPendingResp(t *testing.T, requests []models.PlatformEnrollmentPendingRequest) []byte {
	t.Helper()
	resp := models.PlatformEnrollmentPendingResponse{Requests: requests}
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	return data
}

func mustMarshalDecisionResp(t *testing.T, resp models.PlatformEnrollmentDecisionResponse) []byte {
	t.Helper()
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	return data
}
