// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func markFirstAssignmentRunning(t *testing.T, env *runEnv) {
	t.Helper()
	ctx := context.Background()
	store := env.store(t)
	assignments, err := store.ListAssignments(ctx, "run-a-1")
	require.NoError(t, err)
	require.NotEmpty(t, assignments)
	assignments[0].LifecycleStatus = evalv1.EvaluationAssignmentLifecycleStatus_EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING
	assignments[0].StartedAt = timestamppb.New(env.deps.now())
	require.NoError(t, store.SaveAssignment(ctx, assignments[0]))
}

func TestResolveCampaignOllamaEndpoint_UsesExactOperatorRuntimeConfig(t *testing.T) {
	operators := []models.OperatorDocumentGo{{
		ID:                "inference-1",
		OperatorSessionID: "inference-session",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		RuntimeConfig: &models.RuntimeConfig{
			InferenceEnabled:        true,
			InferenceOllamaEndpoint: "http://provider.example:11434",
		},
	}}
	endpoint, err := resolveCampaignOllamaEndpoint(operators, "inference-session")
	require.NoError(t, err)
	assert.Equal(t, "http://provider.example:11434", endpoint)
}

func TestRunCommands_DoNotExposeOllamaEndpointOverride(t *testing.T) {
	t.Parallel()
	commands := []*cobra.Command{
		runsStartCmd(nativeEvalDeps{}),
		runsResumeCmd(nativeEvalDeps{}),
		rolloutRunCmd(nativeEvalDeps{}),
	}
	for _, command := range commands {
		assert.Nil(t, command.Flags().Lookup("ollama-endpoint"), command.Use)
	}
}

func TestVerificationReportHelpers(t *testing.T) {
	report := &evalv1.EvaluationVerificationReport{
		Status:         evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		FailureCount:   2,
		FailureReasons: []string{"missing witness", "digest mismatch"},
	}

	assert.Equal(t, report.GetStatus().String(), verificationStatusString(report))
	assert.Equal(t, 2, verificationFailureCount(report))
	assert.Equal(t, []string{"missing witness", "digest mismatch"}, verificationFailureReasons(report))

	assert.Empty(t, verificationStatusString(nil))
	assert.Zero(t, verificationFailureCount(nil))
	assert.Nil(t, verificationFailureReasons(nil))
}
