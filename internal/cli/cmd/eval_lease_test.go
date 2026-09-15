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
// Test stubs
// ---------------------------------------------------------------------------

// stubLeaseRunner is a Tier 1 stub evalCommandRunner for lease tests.
type stubLeaseRunner struct {
	stdoutJSON string
	err        error
	lastArgs   []string
	lastName   string
}

func (r *stubLeaseRunner) Run(_ context.Context, name string, args []string, stdout, stderr io.Writer) error {
	r.lastName = name
	r.lastArgs = append([]string(nil), args...)
	if r.err != nil {
		return r.err
	}
	_, _ = io.WriteString(stdout, r.stdoutJSON)
	return nil
}

// stubLeaseCandidateResolver is a Tier 1 stub evalCandidateResolver.
type stubLeaseCandidateResolver struct {
	candidate evalCandidateIdentity
	err       error
}

func (s stubLeaseCandidateResolver) Resolve(_ context.Context, _ string) (evalCandidateIdentity, error) {
	if s.err != nil {
		return evalCandidateIdentity{}, s.err
	}
	return s.candidate, nil
}

// stubLeaseModelInventoryResolver is a Tier 1 stub.
type stubLeaseModelInventoryResolver struct {
	digest string
	err    error
}

func (s stubLeaseModelInventoryResolver) Digest(_ context.Context, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.digest, nil
}

func validLeaseIssueResultJSON() string {
	return `{"lease_id":"test-diagnostic-20260914-120000-abcdef12","lease_path":"/tmp/leases/test.json","status":"active","issued_at":"2026-09-14T12:00:00Z","start_deadline":"2026-09-14T12:05:00Z","expires_at":"2026-09-14T12:30:00Z","content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","request_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","operation_config_content_hash":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}`
}

func validLeaseInspectResultJSON() string {
	return `{"found":true,"lease":{"lease_id":"test-diagnostic-20260914-120000-abcdef12","status":"active","operation_identity":"test-diagnostic","report_root":"reports/test","issued_at":"2026-09-14T12:00:00Z","expires_at":"2026-09-14T12:30:00Z","content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`
}

func validLeaseTransitionResultJSON() string {
	return `{"lease_id":"test-diagnostic-20260914-120000-abcdef12","previous_status":"active","new_status":"stopped","transitioned_at":"2026-09-14T12:15:00Z"}`
}

func newLeaseDepsForTest(
	t *testing.T,
	runner *stubLeaseRunner,
	writer *stubDraftTempFileWriter,
) evalLeaseDeps {
	fileSvc, cfg := newCmdTestEnv(t)
	_ = cfg
	return evalLeaseDeps{
		configLoader:           stubDraftConfigLoader,
		fileSvcFactory:         fileSvcFactoryFor(fileSvc),
		stat:                   stubDraftStat{},
		runner:                 runner,
		tempFileWriter:         writer,
		candidateResolver:      stubLeaseCandidateResolver{candidate: evalCandidateIdentity{SourceTreeHash: strings.Repeat("1", 64), ExecutionSourceManifestHash: strings.Repeat("2", 64), BinarySHA256: strings.Repeat("3", 64), ImageIDs: []string{}}},
		modelInventoryResolver: stubLeaseModelInventoryResolver{digest: strings.Repeat("5", 64)},
	}
}

// ---------------------------------------------------------------------------
// Lease issue
// ---------------------------------------------------------------------------

func TestEvalLeaseIssueCmd_RequiresYes(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseIssueResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseIssueCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("endpoint", "http://192.168.1.2:11434"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
}

func TestEvalLeaseIssueCmd_EmitsResult(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseIssueResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseIssueCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("endpoint", "http://192.168.1.2:11434"))
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	out := stdout.String()
	assert.Contains(t, out, "Lease issued:")
	assert.Contains(t, out, "status:        active")
	assert.Contains(t, out, "request_digest:")
}

func TestEvalLeaseIssueCmd_JSONOutput(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseIssueResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseIssueCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("endpoint", "http://192.168.1.2:11434"))
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	var result evalLeaseIssueResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, "active", result.Status)
	assert.NotEmpty(t, result.LeaseID)
}

func TestEvalLeaseIssueCmd_RunnerErrorReturnsLeaseError(t *testing.T) {
	runner := &stubLeaseRunner{err: errors.New("python failed")}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseIssueCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("endpoint", "http://192.168.1.2:11434"))
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
}

func TestEvalLeaseIssueCmd_InvalidExpiresInReturnsConfigInvalid(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseIssueResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseIssueCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("endpoint", "http://192.168.1.2:11434"))
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	require.NoError(t, cmd.Flags().Set("expires-in", "not-a-duration"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalConfigInvalid)
}

func TestEvalLeaseIssueCmd_RegistersExpectedFlags(t *testing.T) {
	cmd := evalLeaseIssueCmdWithDeps(newLeaseDepsForTest(t, &stubLeaseRunner{}, &stubDraftTempFileWriter{}))
	flags := cmd.Flags()
	for _, name := range []string{"expires-in", "endpoint", "app-identity", "operator-session-identity", "yes", "json"} {
		assert.NotNil(t, flags.Lookup(name), "flag %s should be registered", name)
	}
}

// ---------------------------------------------------------------------------
// Lease inspect
// ---------------------------------------------------------------------------

func TestEvalLeaseInspectCmd_EmitsResult(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseInspectResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseInspectCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	out := stdout.String()
	assert.Contains(t, out, "Lease:")
	assert.Contains(t, out, "status:        active")
}

func TestEvalLeaseInspectCmd_JSONOutput(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseInspectResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseInspectCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	var result evalLeaseInspectResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.True(t, result.Found)
}

func TestEvalLeaseInspectCmd_RegistersExpectedFlags(t *testing.T) {
	cmd := evalLeaseInspectCmdWithDeps(newLeaseDepsForTest(t, &stubLeaseRunner{}, &stubDraftTempFileWriter{}))
	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("json"), "flag json should be registered")
}

// ---------------------------------------------------------------------------
// Lease stop
// ---------------------------------------------------------------------------

func TestEvalLeaseStopCmd_RequiresYes(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseTransitionResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseStopCmdWithDeps(deps)
	cmd.SetArgs([]string{"/tmp/config.json"})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalLeaseMissing)
}

func TestEvalLeaseStopCmd_EmitsResult(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseTransitionResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseStopCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	out := stdout.String()
	assert.Contains(t, out, "Lease Stopped:")
	assert.Contains(t, out, "new_status:       stopped")
}

func TestEvalLeaseStopCmd_JSONOutput(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseTransitionResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseStopCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	var result evalLeaseTransitionResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	assert.Equal(t, "stopped", result.NewStatus)
}

// ---------------------------------------------------------------------------
// Lease expire
// ---------------------------------------------------------------------------

func TestEvalLeaseExpireCmd_EmitsResult(t *testing.T) {
	runner := &stubLeaseRunner{stdoutJSON: validLeaseTransitionResultJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newLeaseDepsForTest(t, runner, writer)

	cmd := evalLeaseExpireCmdWithDeps(deps)
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	cmd.SetArgs([]string{"/tmp/config.json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&bytes.Buffer{})
	require.NoError(t, cmd.Execute())

	out := stdout.String()
	assert.Contains(t, out, "Lease Expired:")
}

// ---------------------------------------------------------------------------
// Command tree registration
// ---------------------------------------------------------------------------

func TestEvalLeaseCmd_RegistersSubcommands(t *testing.T) {
	cmd := evalLeaseCmdWithDeps(newLeaseDepsForTest(t, &stubLeaseRunner{}, &stubDraftTempFileWriter{}))
	subs := cmd.Commands()
	names := make([]string, 0)
	for _, sub := range subs {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "issue")
	assert.Contains(t, names, "inspect")
	assert.Contains(t, names, "stop")
	assert.Contains(t, names, "expire")
}

func TestEvalCmd_RegistersLease(t *testing.T) {
	cmd := evalCmd()
	names := make([]string, 0)
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Use)
	}
	assert.Contains(t, names, "lease")
}
