// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestComposeTargetReader_ObservesPresentAndAbsentControlledTargetState(t *testing.T) {
	observedAt := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	target := filepath.Join(constants.EvaluationTargetContainerDir, constants.TestEvaluationTargetFilename)
	tests := []struct {
		name        string
		result      observerCommandResult
		wantPresent bool
		wantContent []byte
	}{
		{name: "present target returns raw bytes", result: observerCommandResult{stdout: []byte("marker\n"), exitCode: 0}, wantPresent: true, wantContent: []byte("marker\n")},
		{name: "absent target returns typed absence", result: observerCommandResult{exitCode: constants.EvaluationObserverAbsentExitCode}, wantPresent: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			observer := newComposeTargetReader(constants.TestPathRepoRootFromCompliancePackage, func(_ context.Context, dir, name string, args ...string) observerCommandResult {
				calls++
				assert.Equal(t, constants.TestPathRepoRootFromCompliancePackage, dir)
				assert.Equal(t, constants.DockerExecutable, name)
				assert.Equal(t, []string{
					"compose",
					"--file",
					constants.DockerComposeFile,
					"--file",
					constants.DockerNativeEvalComposeFile,
					"run",
					"--rm",
					"--no-deps",
					"-e",
					constants.EvaluationObserverTargetEnv + "=" + constants.TestEvaluationTargetFilename,
					constants.DockerNativeTargetReaderService,
				}, args)
				return test.result
			}, func() time.Time { return observedAt })

			state, err := observer.Observe(context.Background(), target)

			require.NoError(t, err)
			require.NotNil(t, state)
			assert.Equal(t, test.wantPresent, state.Present)
			assert.Equal(t, test.wantContent, state.Content)
			assert.Equal(t, observedAt, state.ObservedAt)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestComposeTargetReader_FailsClosedForInvalidTargetAndCommandFailure(t *testing.T) {
	errCommand := errors.New("command failed")
	tests := []struct {
		name      string
		target    string
		result    observerCommandResult
		wantCalls int
	}{
		{name: "relative target is rejected before execution", target: constants.TestEvaluationTargetFilename},
		{name: "target outside controlled directory is rejected before execution", target: filepath.Join(constants.TestPathVarLibDataDir, constants.TestEvaluationTargetFilename)},
		{name: "observer command failure is unavailable", target: filepath.Join(constants.EvaluationTargetContainerDir, constants.TestEvaluationTargetFilename), result: observerCommandResult{exitCode: 1, err: errCommand}, wantCalls: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			observer := newComposeTargetReader(constants.TestPathRepoRootFromCompliancePackage, func(context.Context, string, string, ...string) observerCommandResult {
				calls++
				return test.result
			}, time.Now)

			state, err := observer.Observe(context.Background(), test.target)

			assert.Nil(t, state)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrEvaluationObservationUnavailable)
			assert.Equal(t, test.wantCalls, calls)
		})
	}
}
