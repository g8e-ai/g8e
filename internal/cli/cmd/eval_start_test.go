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
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ---------------------------------------------------------------------------
// Multi-call stub runner for start tests
// ---------------------------------------------------------------------------

// stubStartRunner is a Tier 1 stub evalCommandRunner that differentiates
// between lease verification, engine, and lease transition calls by
// inspecting the module name in the args. Each call type can return a
// canned JSON response or an error.
type stubStartRunner struct {
	leaseVerifyJSON string
	leaseVerifyErr  error
	engineJSON      string
	engineErr       error
	transitionJSON  string
	transitionErr   error
	lifecycleJSON   string
	lifecycleErr    error

	calls []string // records module names invoked, in order
}

func (r *stubStartRunner) Run(_ context.Context, _ string, args []string, stdout, _ io.Writer) error {
	// args[1] is the module name (args[0] is "-m")
	module := ""
	if len(args) >= 2 {
		module = args[1]
	}
	r.calls = append(r.calls, module)

	switch module {
	case constants.EvalLeaseStartVerificationModule:
		if r.leaseVerifyErr != nil {
			return r.leaseVerifyErr
		}
		_, _ = io.WriteString(stdout, r.leaseVerifyJSON)
		return nil
	case constants.EvalEngineModule:
		if r.engineErr != nil {
			return r.engineErr
		}
		_, _ = io.WriteString(stdout, r.engineJSON)
		return nil
	case constants.EvalLeaseLifecycleModule:
		if r.transitionErr != nil {
			return r.transitionErr
		}
		_, _ = io.WriteString(stdout, r.transitionJSON)
		return nil
	case constants.EvalOperationLifecycleModule:
		if r.lifecycleErr != nil {
			return r.lifecycleErr
		}
		payload := r.lifecycleJSON
		if payload == "" {
			payload = `{"ok":true,"operation_kind":"diagnostic","operation_id":"diag-001","checks":[]}`
		}
		_, _ = io.WriteString(stdout, payload)
		return nil
	default:
		return errors.New("unexpected module: " + module)
	}
}

// validLeaseStartVerificationJSON returns a successful lease verification
// result JSON.
func validLeaseStartVerificationJSON() string {
	return `{"verified":true,"operation_id":"diag-001","revision":"rev-1","report_root":"reports/test","lease_id":"test-diagnostic-20260914-120000-abcdef12","lease_path":"/tmp/leases/test.json","content_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}`
}

// failedLeaseStartVerificationJSON returns a failed lease verification
// result with the given failure code.
func failedLeaseStartVerificationJSON(code, detail string) string {
	return `{"verified":false,"failure_code":"` + code + `","failure_detail":"` + detail + `"}`
}

// validEngineResultJSON returns a successful engine result JSON.
func validEngineResultJSON() string {
	return `{"schema_version":"1.0.0","operation":"diagnostic_start","operation_id":"diag-001","status":"succeeded"}`
}

// failedEngineResultJSON returns a failed engine result JSON.
func failedEngineResultJSON() string {
	return `{"schema_version":"1.0.0","operation":"diagnostic_start","operation_id":"diag-001","status":"failed","error_code":"provider_unreachable","safe_detail":"provider endpoint unreachable"}`
}

// validTransitionResultJSON returns a successful transition result.
func validTransitionResultJSON() string {
	return `{"lease_id":"test-diagnostic-20260914-120000-abcdef12","previous_status":"active","new_status":"completed","transitioned_at":"2026-09-14T12:15:00Z"}`
}

func newStartDepsForTest(t *testing.T, runner *stubStartRunner) evalLeaseDeps {
	fileSvc, cfg := newCmdTestEnv(t)
	_ = cfg
	return evalLeaseDeps{
		configLoader:           stubDraftConfigLoader,
		fileSvcFactory:         fileSvcFactoryFor(fileSvc),
		stat:                   stubDraftStat{},
		runner:                 runner,
		tempFileWriter:         &stubDraftTempFileWriter{},
		candidateResolver:      stubLeaseCandidateResolver{candidate: evalCandidateIdentity{SourceTreeHash: strings.Repeat("1", 64), ExecutionSourceManifestHash: strings.Repeat("2", 64), BinarySHA256: strings.Repeat("3", 64), ImageIDs: []string{}}},
		modelInventoryResolver: stubLeaseModelInventoryResolver{digest: strings.Repeat("5", 64)},
	}
}

// ---------------------------------------------------------------------------
// Diagnostic start
// ---------------------------------------------------------------------------

func TestEvalDiagnosticStartCmd_RequiresYes(t *testing.T) {
	runner := &stubStartRunner{leaseVerifyJSON: validLeaseStartVerificationJSON()}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
	assert.Empty(t, runner.calls, "lease verification must not run when --yes is missing")
}

func TestEvalDiagnosticStartCmd_LeaseVerificationFailedReturnsLeaseError(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: failedLeaseStartVerificationJSON("lease_missing", "no lease supplied"),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
	assert.Equal(t, []string{constants.EvalLeaseStartVerificationModule}, runner.calls, "engine must not be invoked when lease verification fails")
}

func TestEvalDiagnosticStartCmd_LeaseExpiredReturnsLeaseExpiredError(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: failedLeaseStartVerificationJSON("lease_expired", "lease expired"),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseExpired)
	assert.Equal(t, []string{constants.EvalLeaseStartVerificationModule}, runner.calls)
}

func TestEvalDiagnosticStartCmd_LeaseConsumedReturnsLeaseConsumedError(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: failedLeaseStartVerificationJSON("lease_consumed", "lease already completed"),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseConsumed)
}

func TestEvalDiagnosticStartCmd_CandidateDriftReturnsCandidateDriftError(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: failedLeaseStartVerificationJSON("candidate_binary_drift", "binary hash mismatch"),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalCandidateDrift)
}

func TestEvalDiagnosticStartCmd_ReportRootReusedReturnsReportRootReusedError(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: failedLeaseStartVerificationJSON("report_root_reused", "report root exists"),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalReportRootReused)
}

func TestEvalDiagnosticStartCmd_SuccessTransitionsLeaseToCompleted(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: validLeaseStartVerificationJSON(),
		engineJSON:      validEngineResultJSON(),
		transitionJSON:  validTransitionResultJSON(),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	out := stdout.String()
	assert.Contains(t, out, "diagnostic_start: succeeded")
	assert.Contains(t, out, "lease_status: completed")

	require.Len(t, runner.calls, 4)
	assert.Equal(t, constants.EvalLeaseStartVerificationModule, runner.calls[0])
	assert.Equal(t, constants.EvalOperationLifecycleModule, runner.calls[1])
	assert.Equal(t, constants.EvalEngineModule, runner.calls[2])
	assert.Equal(t, constants.EvalLeaseLifecycleModule, runner.calls[3])
}

func TestEvalDiagnosticStartCmd_EngineFailureTransitionsLeaseToStopped(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: validLeaseStartVerificationJSON(),
		engineJSON:      failedEngineResultJSON(),
		transitionJSON:  validTransitionResultJSON(),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute(), "failed engine status is a terminal state, not a launcher defect")

	var result evalStartResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, "failed", result.Status)
	assert.Equal(t, "stopped", result.LeaseStatus)
}

func TestEvalDiagnosticStartCmd_JSONOutput(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: validLeaseStartVerificationJSON(),
		engineJSON:      validEngineResultJSON(),
		transitionJSON:  validTransitionResultJSON(),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	var result evalStartResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, "succeeded", result.Status)
	assert.Equal(t, "completed", result.LeaseStatus)
	assert.Equal(t, "diag-001", result.OperationID)
}

func TestEvalDiagnosticStartCmd_LeaseVerificationRunnerErrorReturnsLeaseMissing(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyErr: errors.New("python failed"),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
}

func TestEvalDiagnosticStartCmd_RegistersExpectedFlags(t *testing.T) {
	cmd := evalDiagnosticStartCmdWithDeps(newStartDepsForTest(t, &stubStartRunner{}))
	flags := cmd.Flags()
	for _, name := range []string{"yes", "json", "verbose"} {
		assert.NotNil(t, flags.Lookup(name), "flag %s should be registered", name)
	}
}

// ---------------------------------------------------------------------------
// Campaign start
// ---------------------------------------------------------------------------

func TestEvalCampaignStartCmd_RequiresYes(t *testing.T) {
	runner := &stubStartRunner{leaseVerifyJSON: validLeaseStartVerificationJSON()}
	deps := newStartDepsForTest(t, runner)

	cmd := evalCampaignStartCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
	assert.Empty(t, runner.calls)
}

func TestEvalCampaignStartCmd_LeaseVerificationFailedReturnsLeaseError(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: failedLeaseStartVerificationJSON("lease_missing", "no lease"),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalCampaignStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
	assert.Equal(t, []string{constants.EvalLeaseStartVerificationModule}, runner.calls)
}

func TestEvalCampaignStartCmd_SuccessTransitionsLeaseToCompleted(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: validLeaseStartVerificationJSON(),
		engineJSON:      `{"schema_version":"1.0.0","operation":"campaign_start","operation_id":"diag-001","status":"succeeded"}`,
		transitionJSON:  validTransitionResultJSON(),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalCampaignStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	out := stdout.String()
	assert.Contains(t, out, "campaign_start: succeeded")
	assert.Contains(t, out, "lease_status: completed")
}

func TestEvalCampaignStartCmd_RegistersExpectedFlags(t *testing.T) {
	cmd := evalCampaignStartCmdWithDeps(newStartDepsForTest(t, &stubStartRunner{}))
	flags := cmd.Flags()
	for _, name := range []string{"yes", "json", "verbose"} {
		assert.NotNil(t, flags.Lookup(name), "flag %s should be registered", name)
	}
}

// ---------------------------------------------------------------------------
// Controller run
// ---------------------------------------------------------------------------

func TestEvalControllerRunCmd_RequiresYes(t *testing.T) {
	runner := &stubStartRunner{leaseVerifyJSON: validLeaseStartVerificationJSON()}
	deps := newStartDepsForTest(t, runner)

	cmd := evalControllerRunCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("work-dir", "/tmp/work"))
	cmd.SetArgs([]string{"/tmp/manifest.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
	assert.Empty(t, runner.calls)
}

func TestEvalControllerRunCmd_LeaseVerificationFailedReturnsLeaseError(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: failedLeaseStartVerificationJSON("lease_missing", "no lease"),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalControllerRunCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("work-dir", "/tmp/work"))
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/manifest.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
	assert.Equal(t, []string{constants.EvalLeaseStartVerificationModule}, runner.calls)
}

func TestEvalControllerRunCmd_SuccessTransitionsLeaseToCompleted(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: validLeaseStartVerificationJSON(),
		engineJSON:      `{"schema_version":"1.0.0","operation":"controller_run","operation_id":"diag-001","status":"succeeded"}`,
		transitionJSON:  validTransitionResultJSON(),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalControllerRunCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("work-dir", "/tmp/work"))
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/manifest.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	out := stdout.String()
	assert.Contains(t, out, "controller_run: succeeded")
	assert.Contains(t, out, "lease_status: completed")
}

func TestEvalControllerRunCmd_RegistersExpectedFlags(t *testing.T) {
	cmd := evalControllerRunCmdWithDeps(newStartDepsForTest(t, &stubStartRunner{}))
	flags := cmd.Flags()
	for _, name := range []string{"work-dir", "yes", "json", "verbose"} {
		assert.NotNil(t, flags.Lookup(name), "flag %s should be registered", name)
	}
}

func TestEvalControllerCmd_RegistersRunSubcommand(t *testing.T) {
	cmd := evalControllerCmdWithDeps(newStartDepsForTest(t, &stubStartRunner{}))
	subs := cmd.Commands()
	names := make([]string, 0)
	for _, sub := range subs {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "run")
}

func TestEvalCmd_RegistersController(t *testing.T) {
	cmd := evalCmd()
	names := make([]string, 0)
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "controller")
}

// ---------------------------------------------------------------------------
// Lease verification cannot be bypassed
// ---------------------------------------------------------------------------

// TestEvalStart_LeaseVerificationCannotBeBypassed proves that when lease
// verification fails, the engine module is never invoked. The stub runner
// records every call; if the engine were invoked, the calls slice would
// contain EvalEngineModule. This test asserts it does not.
func TestEvalStart_LeaseVerificationCannotBeBypassed(t *testing.T) {
	runner := &stubStartRunner{
		leaseVerifyJSON: failedLeaseStartVerificationJSON("lease_missing", "no lease"),
		engineJSON:      validEngineResultJSON(),
	}
	deps := newStartDepsForTest(t, runner)

	cmd := evalDiagnosticStartCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)

	for _, call := range runner.calls {
		assert.NotEqual(t, constants.EvalEngineModule, call, "engine must not be invoked when lease verification fails")
	}
}
