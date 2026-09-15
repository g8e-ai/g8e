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
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

type stubEvalLifecycleRunner struct {
	lifecycleJSON string
	leaseJSON     string
	engineJSON    string
	calls         []string
}

func (r *stubEvalLifecycleRunner) Run(_ context.Context, _ string, args []string, stdout, _ io.Writer) error {
	if len(args) < 2 {
		return errors.New("missing module")
	}
	module := args[1]
	r.calls = append(r.calls, module)
	switch module {
	case constants.EvalOperationLifecycleModule:
		_, _ = io.WriteString(stdout, r.lifecycleJSON)
	case constants.EvalLeaseStartVerificationModule:
		_, _ = io.WriteString(stdout, r.leaseJSON)
	case constants.EvalEngineModule:
		_, _ = io.WriteString(stdout, r.engineJSON)
	default:
		return errors.New("unexpected module: " + module)
	}
	return nil
}

func TestEvalDiagnosticPlanCmd_UsesTypedLifecycleModule(t *testing.T) {
	runner := &stubEvalLifecycleRunner{lifecycleJSON: `{"operation_kind":"diagnostic","operation_id":"diag-1","revision":"rev-1","selected_models":["qwen3:8b"],"arms":["direct"],"task_count":5,"task_identities":["t1","t2","t3","t4","t5"],"repetitions":1,"assignment_count":5,"warmup_calls":0,"maximum_provider_calls":10,"maximum_tokens":1000,"maximum_usd":1.5,"maximum_duration_s":600,"minimum_free_disk_gb":2,"schedule_identity":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","stop_conditions":{"idle_timeout_s":180,"max_duration_s":600}}`}
	deps := newStartDepsForTest(t, &stubStartRunner{})
	deps.runner = runner
	cmd := evalDiagnosticPlanCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{constants.EvalOperationLifecycleModule}, runner.calls)
	assert.Contains(t, stdout.String(), "assignments:            5")
	assert.Contains(t, stdout.String(), "maximum_provider_calls: 10")
}

func TestEvalCampaignCheckCmd_VerifiesLeaseBeforeProviderFreeChecks(t *testing.T) {
	runner := &stubEvalLifecycleRunner{
		leaseJSON:     validLeaseStartVerificationJSON(),
		lifecycleJSON: `{"ok":true,"operation_kind":"campaign","operation_id":"campaign-1","checks":[{"check_id":"config","status":"pass","safe_detail":"valid"}]}`,
	}
	deps := newStartDepsForTest(t, &stubStartRunner{})
	deps.runner = runner
	cmd := evalCampaignCheckCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{constants.EvalLeaseStartVerificationModule, constants.EvalOperationLifecycleModule}, runner.calls)
	assert.Contains(t, stdout.String(), "config: pass")
}

func TestEvalCampaignCheckCmd_DoesNotRunChecksWhenLeaseFails(t *testing.T) {
	runner := &stubEvalLifecycleRunner{leaseJSON: failedLeaseStartVerificationJSON("lease_expired", "expired")}
	deps := newStartDepsForTest(t, &stubStartRunner{})
	deps.runner = runner
	cmd := evalCampaignCheckCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseExpired)
	assert.Equal(t, []string{constants.EvalLeaseStartVerificationModule}, runner.calls)
}

func TestEvalCampaignStatusCmd_EmitsReconciledState(t *testing.T) {
	runner := &stubEvalLifecycleRunner{engineJSON: `{"schema_version":"1.0.0","operation":"campaign_status","operation_id":"campaign-1","status":"succeeded","payload":{"operation_kind":"campaign","operation_id":"campaign-1","revision":"rev-1","status":"running","process_state":"running","report_root":"/tmp/report","completed_assignments":2,"total_assignments":5,"provider_requests":2,"tokens":100,"spent_usd":0.25,"publication_state":"not_published"}}`}
	deps := newStartDepsForTest(t, &stubStartRunner{})
	deps.runner = runner
	cmd := evalCampaignStatusCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{constants.EvalEngineModule}, runner.calls)
	assert.Contains(t, stdout.String(), "assignments:           2/5")
}

func TestEvalDiagnosticStopCmd_ImmediateRequiresYes(t *testing.T) {
	deps := newStartDepsForTest(t, &stubStartRunner{})
	cmd := evalDiagnosticStopCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json", "--immediate"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalConfigInvalid)
}

func TestEvalLifecycleCommands_AreRegistered(t *testing.T) {
	deps := newStartDepsForTest(t, &stubStartRunner{})
	for _, cmd := range []*cobraCommandView{
		newCobraCommandView(evalDiagnosticCmdWithDeps(deps)),
		newCobraCommandView(evalCampaignCmdWithDeps(deps)),
	} {
		assert.Contains(t, cmd.names, "plan")
		assert.Contains(t, cmd.names, "check")
		assert.Contains(t, cmd.names, "status")
		assert.Contains(t, cmd.names, "stop")
		assert.Contains(t, cmd.names, "verify")
	}
}

func TestEvalU8Commands_AreRegistered(t *testing.T) {
	deps := newStartDepsForTest(t, &stubStartRunner{})
	campaignSet := newCobraCommandView(evalCampaignSetCmdWithDeps(deps))
	assert.ElementsMatch(t, []string{"plan", "validate", "verify"}, campaignSet.names)
	qualification := newCobraCommandView(evalQualificationCmd())
	assert.ElementsMatch(t, []string{"build", "candidate", "collect-runtime", "hash-source", "run-gate"}, qualification.names)
	root := newCobraCommandView(evalCmd())
	for _, name := range []string{"campaign-set", "bundle", "verify", "qualification", "bench-synthetic"} {
		assert.Contains(t, root.names, name)
	}
}

type cobraCommandView struct {
	names []string
}

func newCobraCommandView(cmd interface{ Commands() []*cobra.Command }) *cobraCommandView {
	names := make([]string, 0)
	for _, child := range cmd.Commands() {
		names = append(names, child.Name())
	}
	return &cobraCommandView{names: names}
}

func TestEvalCampaignStatusCmd_RendersNewReconciliationFields(t *testing.T) {
	runner := &stubEvalLifecycleRunner{engineJSON: `{"schema_version":"1.0.0","operation":"campaign_status","operation_id":"campaign-1","status":"succeeded","payload":{"operation_kind":"campaign","operation_id":"campaign-1","revision":"rev-1","status":"finalized","process_state":"terminal","report_root":"/tmp/report","completed_assignments":5,"total_assignments":5,"provider_requests":10,"tokens":500,"spent_usd":1.25,"stop_reason":"","verification_state":"not_verified","publication_state":"not_published"}}`}
	deps := newStartDepsForTest(t, &stubStartRunner{})
	deps.runner = runner
	cmd := evalCampaignStatusCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, stdout.String(), "verification:          not_verified")
	assert.Contains(t, stdout.String(), "publication:           not_published")
}

func TestEvalDiagnosticVerifyCmd_RendersFailedPayloadOnNonzeroExit(t *testing.T) {
	runner := &stubEvalLifecycleRunner{engineJSON: `{"schema_version":"1.0.0","operation":"diagnostic_verify","operation_id":"diag-1","status":"failed","error_code":"authority_invalid","error_stage":"verify","safe_detail":"offline verification failed with 2 failure(s)","payload":{"operation_kind":"diagnostic","operation_id":"diag-1","ok":false,"checked_layers":["manifest","attempts"],"failures":["missing manifest","missing attempts"]}}`}
	deps := newStartDepsForTest(t, &stubStartRunner{})
	deps.runner = runner
	cmd := evalDiagnosticVerifyCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalAuthorityInvalid)
	// The typed payload must be rendered even on nonzero exit
	assert.Contains(t, stdout.String(), "diagnostic verify: failed (manifest, attempts)")
	assert.Contains(t, stdout.String(), "missing manifest")
	assert.Contains(t, stdout.String(), "missing attempts")
}

func TestEvalDiagnosticVerifyCmd_JSONEmitsFailedPayloadOnNonzeroExit(t *testing.T) {
	runner := &stubEvalLifecycleRunner{engineJSON: `{"schema_version":"1.0.0","operation":"diagnostic_verify","operation_id":"diag-1","status":"failed","error_code":"authority_invalid","error_stage":"verify","safe_detail":"offline verification failed with 1 failure(s)","payload":{"operation_kind":"diagnostic","operation_id":"diag-1","ok":false,"checked_layers":["manifest"],"failures":["missing manifest"]}}`}
	deps := newStartDepsForTest(t, &stubStartRunner{})
	deps.runner = runner
	cmd := evalDiagnosticVerifyCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json", "--json"})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalAuthorityInvalid)
	// JSON output must include the typed payload fields
	assert.Contains(t, stdout.String(), `"ok":false`)
	assert.Contains(t, stdout.String(), `"checked_layers":["manifest"]`)
	assert.Contains(t, stdout.String(), `"failures":["missing manifest"]`)
}
