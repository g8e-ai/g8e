// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func panickingNativeEvalDeps(t *testing.T) nativeEvalDeps {
	t.Helper()
	return nativeEvalDeps{
		configLoader: func(string) (*config.Config, error) {
			t.Fatal("config loader called")
			return nil, nil
		},
		fileSvcFactory: func(string, *slog.Logger) (fs.RuntimeFileService, error) {
			t.Fatal("file service factory called")
			return nil, nil
		},
		clientFactory: func(harnessconfig.Config) (*harnessclient.Client, error) {
			t.Fatal("client factory called")
			return nil, nil
		},
		authLoader: func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			t.Fatal("auth loader called")
			return nil, nil
		},
		now:   time.Now,
		newID: func() string { return "run-id" },
	}
}

func TestEvalCmd_ContainsOnlyNativeCommands(t *testing.T) {
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	names := make([]string, 0, len(command.Commands()))
	for _, child := range command.Commands() {
		names = append(names, child.Name())
	}
	assert.ElementsMatch(t, []string{
		"run", "verify", "show", "inference", "chat", "inventory", "campaign", "provider-observer",
	}, names)
	assert.Len(t, names, 8)
	assert.Contains(t, command.Aliases, "evals")

	var inference *cobra.Command
	for _, child := range command.Commands() {
		if child.Name() == "inference" {
			inference = child
			break
		}
	}
	require.NotNil(t, inference)
	inferenceNames := make([]string, 0, len(inference.Commands()))
	for _, child := range inference.Commands() {
		inferenceNames = append(inferenceNames, child.Name())
	}
	assert.ElementsMatch(t, []string{"status", "freeze-registry", "probe", "accept"}, inferenceNames)

	var chat *cobra.Command
	for _, child := range command.Commands() {
		if child.Name() == "chat" {
			chat = child
			break
		}
	}
	require.NotNil(t, chat)
	chatNames := make([]string, 0, len(chat.Commands()))
	for _, child := range chat.Commands() {
		chatNames = append(chatNames, child.Name())
	}
	assert.ElementsMatch(t, []string{"accept"}, chatNames)
}

func TestEvalRun_RejectsUnsupportedSuiteBeforeDependencies(t *testing.T) {
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	command.SetArgs([]string{"run", "removed-python-suite"})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvaluationSuiteUnsupported)
}

func TestWriteNativeEvalRun_JSONEmitsCanonicalReportOnly(t *testing.T) {
	report := &evalv1.EvaluationReport{SchemaVersion: "1.0.0", Run: &evalv1.EvaluationRun{RunId: "run-1"}}
	verification := &compliancev1.ComplianceVerificationReport{ReportId: "run-1", Valid: true}
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	var output bytes.Buffer
	command.SetOut(&output)
	require.NoError(t, writeNativeEvalRun(command, report, verification, true))
	expected, err := evalv1.MarshalCanonical(report)
	require.NoError(t, err)
	assert.Equal(t, string(expected)+"\n", output.String())
}

func TestWriteNativeVerification_JSONEmitsCanonicalReportOnly(t *testing.T) {
	report := &compliancev1.ComplianceVerificationReport{ReportId: "run-1", Valid: false}
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	var output bytes.Buffer
	command.SetOut(&output)
	require.NoError(t, writeNativeVerification(command, report, true))
	expected, err := compliancev1.MarshalCanonical(report)
	require.NoError(t, err)
	assert.Equal(t, string(expected)+"\n", output.String())
}

type nativeEvalRunnerStub struct {
	run func(context.Context, evaluation.RunRequest) (*evalv1.EvaluationReport, error)
}

func (s nativeEvalRunnerStub) Run(ctx context.Context, request evaluation.RunRequest) (*evalv1.EvaluationReport, error) {
	return s.run(ctx, request)
}

type nativeEvalVerifierStub struct {
	verify func(context.Context, string) (*compliancev1.ComplianceVerificationReport, error)
}

func (s nativeEvalVerifierStub) Verify(ctx context.Context, runID string) (*compliancev1.ComplianceVerificationReport, error) {
	return s.verify(ctx, runID)
}

type nativeEvalStoreStub struct {
	loadReport        func(context.Context, string) (*evalv1.EvaluationReport, error)
	savedReports      []*evalv1.EvaluationReport
	savedVerification *compliancev1.ComplianceVerificationReport
}

func (s *nativeEvalStoreStub) SaveReport(_ context.Context, report *evalv1.EvaluationReport) error {
	s.savedReports = append(s.savedReports, report)
	return nil
}

func (s *nativeEvalStoreStub) SaveTargetState(context.Context, *evalv1.EvaluationTargetState) (*compliancev1.ComplianceEvidenceReference, error) {
	return nil, nil
}

func (s *nativeEvalStoreStub) SaveVerification(_ context.Context, runID string, report *compliancev1.ComplianceVerificationReport) (*compliancev1.ComplianceEvidenceReference, error) {
	s.savedVerification = report
	return &compliancev1.ComplianceEvidenceReference{RunId: runID}, nil
}

func (s *nativeEvalStoreStub) LoadReport(ctx context.Context, runID string) (*evalv1.EvaluationReport, error) {
	return s.loadReport(ctx, runID)
}

func nativeEvalTestReport(runID string) *evalv1.EvaluationReport {
	return &evalv1.EvaluationReport{
		SchemaVersion: "1.0.0",
		Run:           &evalv1.EvaluationRun{RunId: runID},
		SummaryStatus: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_FAIL,
		Summary:       "required invariant failed",
	}
}

func nativeEvalCommandDeps(t *testing.T, store nativeEvalStore, runner nativeEvalRunner, verifier nativeEvalVerifier) nativeEvalDeps {
	t.Helper()
	return nativeEvalDeps{
		configLoader:      func(string) (*config.Config, error) { return &config.Config{ProjectRoot: "/project"}, nil },
		fileSvcFactory:    func(string, *slog.Logger) (fs.RuntimeFileService, error) { return nil, nil },
		createRuntimeTree: func(context.Context, fs.RuntimeFileService) error { return nil },
		clientFactory:     func(harnessconfig.Config) (*harnessclient.Client, error) { return nil, nil },
		authLoader: func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			return &auth.ClientAuthContext{UserID: "user-1", CLISessionID: "cli-session-1"}, nil
		},
		laneFactory: func(*harnessclient.Client, fs.RuntimeFileService, harnessclient.Persona) evaluation.PlatformLane {
			return nil
		},
		observerFactory: func(string) evaluation.TargetObserver { return nil },
		runnerFactory: func(evaluation.PlatformLane, evaluation.TargetObserver, evaluation.ReportStore, func() time.Time, func(string) string) nativeEvalRunner {
			return runner
		},
		storeFactory:    func(fs.RuntimeFileService) nativeEvalStore { return store },
		verifierFactory: func(fs.RuntimeFileService, func() time.Time) nativeEvalVerifier { return verifier },
		now:             time.Now,
		newID:           func() string { return "run-1" },
	}
}

func TestEvalRun_AbsentCLIIdentityFailsBeforeClientCreation(t *testing.T) {
	deps := nativeEvalCommandDeps(t, &nativeEvalStoreStub{}, nil, nil)
	deps.authLoader = func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
		return nil, constants.ErrNotAuthenticated
	}
	deps.clientFactory = func(harnessconfig.Config) (*harnessclient.Client, error) {
		t.Fatal("client factory called")
		return nil, nil
	}
	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"run", evaluation.CoreExecutionBoundarySuiteID})

	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
}

func TestEvalShow_JSONLoadsPersistedReportAndEmitsCanonicalProtojson(t *testing.T) {
	report := nativeEvalTestReport("run-1")
	store := &nativeEvalStoreStub{loadReport: func(_ context.Context, runID string) (*evalv1.EvaluationReport, error) {
		assert.Equal(t, "run-1", runID)
		return report, nil
	}}
	command := evalCmdWithConfig(nativeEvalCommandDeps(t, store, nil, nil))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"show", "run-1", "--json"})

	require.NoError(t, command.Execute())
	expected, err := evalv1.MarshalCanonical(report)
	require.NoError(t, err)
	assert.Equal(t, string(expected)+"\n", output.String())
}

func TestEvalVerify_JSONPrintsInvalidTypedReportBeforeNonzeroExit(t *testing.T) {
	report := &compliancev1.ComplianceVerificationReport{ReportId: "run-1", Valid: false}
	verifier := nativeEvalVerifierStub{verify: func(_ context.Context, runID string) (*compliancev1.ComplianceVerificationReport, error) {
		assert.Equal(t, "run-1", runID)
		return report, nil
	}}
	command := evalCmdWithConfig(nativeEvalCommandDeps(t, &nativeEvalStoreStub{}, nil, verifier))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetArgs([]string{"verify", "run-1", "--json"})

	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalRunVerificationFailed)
	expected, marshalErr := compliancev1.MarshalCanonical(report)
	require.NoError(t, marshalErr)
	assert.Equal(t, string(expected)+"\n", output.String())
}

func TestEvalRun_PersistsAndPrintsFailureReportsWithExactSessionBinding(t *testing.T) {
	testCases := []struct {
		name              string
		runErr            error
		verificationValid bool
	}{
		{name: "topology failure", runErr: errors.New("topology failed"), verificationValid: true},
		{name: "posture failure", runErr: errors.New("posture failed"), verificationValid: true},
		{name: "dispatch failure", runErr: errors.New("dispatch failed"), verificationValid: true},
		{name: "observation failure", runErr: errors.New("observation failed"), verificationValid: true},
		{name: "evidence verification failure", verificationValid: false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			report := nativeEvalTestReport("run-1")
			var request evaluation.RunRequest
			runner := nativeEvalRunnerStub{run: func(_ context.Context, actual evaluation.RunRequest) (*evalv1.EvaluationReport, error) {
				request = actual
				return report, testCase.runErr
			}}
			verification := &compliancev1.ComplianceVerificationReport{ReportId: "run-1", Valid: testCase.verificationValid}
			verifier := nativeEvalVerifierStub{verify: func(context.Context, string) (*compliancev1.ComplianceVerificationReport, error) {
				return verification, nil
			}}
			store := &nativeEvalStoreStub{}
			command := evalCmdWithConfig(nativeEvalCommandDeps(t, store, runner, verifier))
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SilenceErrors = true
			command.SilenceUsage = true
			command.SetArgs([]string{"run", evaluation.CoreExecutionBoundarySuiteID, "--operator-session", "operator-session-exact", "--json"})

			err := command.Execute()
			require.Error(t, err)
			assert.Equal(t, "operator-session-exact", request.PinnedOperatorSessionID)
			assert.Same(t, verification, store.savedVerification)
			require.Len(t, store.savedReports, 1)
			assert.Same(t, report, store.savedReports[0])
			expected, marshalErr := evalv1.MarshalCanonical(report)
			require.NoError(t, marshalErr)
			assert.Equal(t, string(expected)+"\n", output.String())
		})
	}
}
