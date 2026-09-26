// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestProviderObserverVerify_RequiresAttemptID(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"dev", "provider-observer", "verify", "--project-root", root})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestProviderObserverVerify_ReportsMissingWindow(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"dev", "provider-observer", "verify", "--project-root", root, "--provider-attempt-id", "missing-attempt"})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider observer verify")
}

func TestProviderObserverVerify_ReportsIncompleteCoverage(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	windowStore, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          "attempt-1",
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
		Samples: []*evalv1.ProviderBoundaryHardwareSample{{
			ObservedAtUnixNanos: uint64(time.Unix(1_700_000_001, 0).UnixNano()),
		}},
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	require.NoError(t, windowStore.Save(context.Background(), window))
	attemptStore, err := inference.NewAttemptStore(fileSvc)
	require.NoError(t, err)
	require.NoError(t, attemptStore.Begin(context.Background(), &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-1",
		TransactionId:     "tx-1",
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		StartedAtUnixMs:   time.Unix(1_700_000_000, 0).UnixMilli(),
		CompletedAtUnixMs: time.Unix(1_700_000_010, 0).UnixMilli(),
	}))

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"dev", "provider-observer", "verify", "--project-root", root, "--provider-attempt-id", "attempt-1"})
	execErr := command.Execute()
	require.Error(t, execErr)
	assert.ErrorIs(t, execErr, constants.ErrEvalRunVerificationFailed)
	assert.Contains(t, output.String(), "incomplete")
}
