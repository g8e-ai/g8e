// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type recordingCanaryRunner struct {
	calls []canaryOptions
	err   error
}

func (r *recordingCanaryRunner) run(_ *cobra.Command, opts canaryOptions) error {
	r.calls = append(r.calls, opts)
	return r.err
}

func canaryFailure(id evaluation.CanaryID) error {
	return fmt.Errorf("evaluation: environment canaries failed (%s): %w", id, constants.ErrEvaluationEnvironmentCanaryFailed)
}

func neverRunGate(t *testing.T) rolloutGateFactory {
	t.Helper()
	return func(fs.RuntimeFileService, []*evalv1.ModelVariant) rolloutGateRunner {
		return func(*cobra.Command, evaluation.CampaignQueueModel, string) (*rolloutRunSuccess, error) {
			t.Fatal("no model may be run when an environment canary fails")
			return nil, nil
		}
	}
}

func TestReportEnvironmentCanaries_NamesEveryFailedCanary(t *testing.T) {
	cmd := &cobra.Command{}
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	report := evaluation.CanaryReport{Results: []evaluation.CanaryResult{
		{ID: evaluation.CanaryToolsDeclared, Passed: true, Detail: "all declared"},
		{ID: evaluation.CanarySeedDelivered, Detail: "seed not echoed"},
		{ID: evaluation.CanaryRegistryMCP, Detail: "lockdown drift"},
	}}

	err := reportEnvironmentCanaries(cmd, report, nil)

	require.ErrorIs(t, err, constants.ErrEvaluationEnvironmentCanaryFailed)
	assert.Contains(t, err.Error(), "seed-delivered, registry-mcp")
	assert.Contains(t, stdout.String(), "CANARY PASS tools-declared: all declared")
	assert.Contains(t, stderr.String(), "ENVIRONMENT ERROR (seed-delivered): seed not echoed")
	assert.Contains(t, stderr.String(), "ENVIRONMENT ERROR (registry-mcp): lockdown drift")
}
