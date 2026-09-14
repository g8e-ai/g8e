// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// stubDraftRunner is a test evalCommandRunner that captures the args and
// writes a canned JSON summary to stdout.
type stubDraftRunner struct {
	lastArgs   []string
	lastName   string
	stdoutJSON string
	err        error
}

func (r *stubDraftRunner) Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	r.lastName = name
	r.lastArgs = args
	if r.err != nil {
		return r.err
	}
	_, _ = stdout.Write([]byte(r.stdoutJSON))
	return nil
}

// stubDraftTempFileWriter is a test evalTempFileWriter that captures the
// written data and returns a fake path.
type stubDraftTempFileWriter struct {
	lastData []byte
	lastPath string
}

func (w *stubDraftTempFileWriter) WriteTempFile(namePattern string, data []byte) (string, error) {
	w.lastData = data
	w.lastPath = "/tmp/stub-draft-request.json"
	return w.lastPath, nil
}

// stubDraftStat is a test evalFileStat that reports all paths as existing.
type stubDraftStat struct{}

func (stubDraftStat) Stat(path string) (os.FileInfo, error) { return stubFileInfo{name: path}, nil }

type stubFileInfo struct{ name string }

func (f stubFileInfo) Name() string       { return f.name }
func (f stubFileInfo) Size() int64        { return 0 }
func (f stubFileInfo) Mode() os.FileMode   { return 0o644 }
func (f stubFileInfo) ModTime() time.Time { return time.Time{} }
func (f stubFileInfo) IsDir() bool        { return false }
func (f stubFileInfo) Sys() any           { return nil }

// stubDraftConfigLoader returns a minimal config for testability.
func stubDraftConfigLoader(_ string) (*config.Config, error) {
	return &config.Config{ProjectRoot: "/test/repo"}, nil
}

func newDraftDepsForTest(runner *stubDraftRunner, writer *stubDraftTempFileWriter) evalDraftDeps {
	return evalDraftDeps{
		configLoader:   stubDraftConfigLoader,
		stat:           stubDraftStat{},
		runner:         runner,
		tempFileWriter: writer,
	}
}

func validDraftSummaryJSON() string {
	return `{"operation_kind":"diagnostic","operation_id":"diag-001","revision":"rev-1","preset":"ifeval-five-task","suite":"ifeval_subset","report_root":".local.dev/campaign/diag-test","content_hash":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789","dimensions":{"arm":"ensemble_ungoverned","model_variant_id":"gemma4:e4b","task_limit":5,"task_offset":0},"budget":{"max_requests":30,"max_tokens":491520,"max_usd":0,"concurrency":1},"authority_hashes":{"gold_set":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"endpoint_class":"ollama-local","provider":"ollama"}`
}

// ---------------------------------------------------------------------------
// Diagnostic draft
// ---------------------------------------------------------------------------


func TestEvalDiagnosticDraftCmd_WritesRequestAndEmitsSummary(t *testing.T) {
	runner := &stubDraftRunner{stdoutJSON: validDraftSummaryJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newDraftDepsForTest(runner, writer)

	cmd := evalDiagnosticDraftCmdWithDeps(deps)
	setDraftFlags(t, cmd, map[string]string{
		"preset":            "ifeval-five-task",
		"out":               "/tmp/diag-out.json",
		"operation-id":      "diag-001",
		"report-root":       ".local.dev/campaign/diag-test",
		"gold-set-path":     ".local.dev/campaign/gold.json",
		"gold-set-sha256":  strings.Repeat("a", 64),
		"evidence-key-path": ".local.dev/campaign/key.json",
		"evidence-key-id":   "key-001",
		"provider":          "ollama",
		"endpoint-class":    "ollama-local",
		"model-variant-id":  "gemma4:e4b",
	})

	var stdout strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	// Verify the runner was invoked with the draft module
	assert.Equal(t, "/test/repo/ensemble/evals/.venv/bin/python", runner.lastName)
	require.Len(t, runner.lastArgs, 3)
	assert.Equal(t, "-m", runner.lastArgs[0])
	assert.Equal(t, constants.EvalDraftModule, runner.lastArgs[1])
	assert.Equal(t, writer.lastPath, runner.lastArgs[2])

	// Verify the request JSON contains the expected fields
	var req map[string]any
	require.NoError(t, json.Unmarshal(writer.lastData, &req))
	assert.Equal(t, "diagnostic", req["kind"])
	assert.Equal(t, "ifeval-five-task", req["preset"])
	assert.Equal(t, "diag-001", req["operation_id"])
	assert.Equal(t, "gemma4:e4b", req["model_variant_id"])

	// Verify human output contains key fields
	out := stdout.String()
	assert.Contains(t, out, "Draft created: diagnostic")
	assert.Contains(t, out, "operation_id: diag-001")
	assert.Contains(t, out, "preset:        ifeval-five-task")
}

func TestEvalDiagnosticDraftCmd_JSONOutputEmitsJSON(t *testing.T) {
	runner := &stubDraftRunner{stdoutJSON: validDraftSummaryJSON()}
	writer := &stubDraftTempFileWriter{}
	deps := newDraftDepsForTest(runner, writer)

	cmd := evalDiagnosticDraftCmdWithDeps(deps)
	setDraftFlags(t, cmd, map[string]string{
		"preset":            "ifeval-five-task",
		"out":               "/tmp/diag-out.json",
		"operation-id":      "diag-001",
		"report-root":       ".local.dev/campaign/diag-test",
		"gold-set-path":     ".local.dev/campaign/gold.json",
		"gold-set-sha256":  strings.Repeat("a", 64),
		"evidence-key-path": ".local.dev/campaign/key.json",
		"evidence-key-id":   "key-001",
		"provider":          "ollama",
		"endpoint-class":    "ollama-local",
		"model-variant-id":  "gemma4:e4b",
	})
	require.NoError(t, cmd.Flags().Set("json", "true"))

	var stdout strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	var summary evalDraftSummary
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &summary))
	assert.Equal(t, "diagnostic", summary.OperationKind)
	assert.Equal(t, "diag-001", summary.OperationID)
	assert.Equal(t, "ifeval-five-task", summary.Preset)
}

func TestEvalDiagnosticDraftCmd_RunnerErrorReturnsConfigInvalid(t *testing.T) {
	runner := &stubDraftRunner{err: errors.New("python failed")}
	writer := &stubDraftTempFileWriter{}
	deps := newDraftDepsForTest(runner, writer)

	cmd := evalDiagnosticDraftCmdWithDeps(deps)
	setDraftFlags(t, cmd, map[string]string{
		"preset":            "ifeval-five-task",
		"out":               "/tmp/diag-out.json",
		"operation-id":      "diag-001",
		"report-root":       ".local.dev/campaign/diag-test",
		"gold-set-path":     ".local.dev/campaign/gold.json",
		"gold-set-sha256":  strings.Repeat("a", 64),
		"evidence-key-path": ".local.dev/campaign/key.json",
		"evidence-key-id":   "key-001",
		"provider":          "ollama",
		"endpoint-class":    "ollama-local",
		"model-variant-id":  "gemma4:e4b",
	})

	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalConfigInvalid)
}

func TestEvalDiagnosticDraftCmd_InvalidSummaryReturnsConfigInvalid(t *testing.T) {
	runner := &stubDraftRunner{stdoutJSON: "not json"}
	writer := &stubDraftTempFileWriter{}
	deps := newDraftDepsForTest(runner, writer)

	cmd := evalDiagnosticDraftCmdWithDeps(deps)
	setDraftFlags(t, cmd, map[string]string{
		"preset":            "ifeval-five-task",
		"out":               "/tmp/diag-out.json",
		"operation-id":      "diag-001",
		"report-root":       ".local.dev/campaign/diag-test",
		"gold-set-path":     ".local.dev/campaign/gold.json",
		"gold-set-sha256":  strings.Repeat("a", 64),
		"evidence-key-path": ".local.dev/campaign/key.json",
		"evidence-key-id":   "key-001",
		"provider":          "ollama",
		"endpoint-class":    "ollama-local",
		"model-variant-id":  "gemma4:e4b",
	})

	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalConfigInvalid)
}

func TestEvalDiagnosticDraftCmd_RegistersWithExpectedFlags(t *testing.T) {
	cmd := evalDiagnosticDraftCmdWithDeps(newDraftDepsForTest(&stubDraftRunner{}, &stubDraftTempFileWriter{}))
	flags := cmd.Flags()
	expectedFlags := []string{
		"preset", "out", "operation-id", "revision", "report-root",
		"gold-set-path", "gold-set-sha256", "evidence-key-path", "evidence-key-id",
		"provider", "endpoint-class", "model-variant-id", "json",
	}
	for _, name := range expectedFlags {
		assert.NotNil(t, flags.Lookup(name), "flag %s should be registered", name)
	}
	for _, name := range []string{"preset", "out", "operation-id", "report-root", "gold-set-path", "gold-set-sha256", "evidence-key-path", "evidence-key-id", "provider", "endpoint-class", "model-variant-id"} {
		assert.True(t, flags.Lookup(name).Changed == false, "flag %s should not be required by default", name)
	}
}

// ---------------------------------------------------------------------------
// Campaign draft
// ---------------------------------------------------------------------------


func TestEvalCampaignDraftCmd_WritesRequestAndEmitsSummary(t *testing.T) {
	campSummary := `{"operation_kind":"campaign","operation_id":"camp-001","revision":"rev-1","preset":"opendevops-development","suite":"ifeval_subset","report_root":".local.dev/campaign/camp-test","content_hash":"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789","dimensions":{"arms":["direct","ensemble_ungoverned"],"cohort_count":1,"repetitions":3},"budget":{"max_requests":100,"max_tokens":1000000,"max_usd":0,"concurrency":1},"authority_hashes":{"gold_set":"aaaa","preregistration":"bbbb","profile":"cccc","model_registry":"dddd"},"endpoint_class":"ollama-local","provider":"ollama"}`
	runner := &stubDraftRunner{stdoutJSON: campSummary}
	writer := &stubDraftTempFileWriter{}
	deps := newDraftDepsForTest(runner, writer)

	cmd := evalCampaignDraftCmdWithDeps(deps)
	setDraftFlags(t, cmd, map[string]string{
		"preset":                "opendevops-development",
		"out":                  "/tmp/camp-out.json",
		"operation-id":         "camp-001",
		"report-root":          ".local.dev/campaign/camp-test",
		"gold-set-path":        ".local.dev/campaign/gold.json",
		"gold-set-sha256":      strings.Repeat("a", 64),
		"evidence-key-path":    ".local.dev/campaign/key.json",
		"evidence-key-id":      "key-001",
		"provider":             "ollama",
		"endpoint-class":       "ollama-local",
		"campaign-id":          "test-campaign",
		"release-version":      "v2.1.8",
		"preregistration-path":  ".local.dev/campaign/prereg.json",
		"preregistration-sha256": strings.Repeat("b", 64),
		"profile-path":         ".local.dev/campaign/profile.json",
		"profile-sha256":      strings.Repeat("c", 64),
		"model-registry-path":   ".local.dev/campaign/registry.json",
		"model-registry-sha256": strings.Repeat("d", 64),
		"cohort-ids":           "cohort-qwen3-8b",
	})

	var stdout strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	// Verify the request JSON contains campaign-specific fields
	var req map[string]any
	require.NoError(t, json.Unmarshal(writer.lastData, &req))
	assert.Equal(t, "campaign", req["kind"])
	assert.Equal(t, "test-campaign", req["campaign_id"])
	assert.Equal(t, "v2.1.8", req["release_version"])
	assert.Equal(t, []any{"cohort-qwen3-8b"}, req["cohort_ids"])

	// Verify human output
	out := stdout.String()
	assert.Contains(t, out, "Draft created: campaign")
	assert.Contains(t, out, "operation_id: camp-001")
	assert.Contains(t, out, "preset:        opendevops-development")
}

func TestEvalCampaignDraftCmd_JSONOutputEmitsJSON(t *testing.T) {
	campSummary := `{"operation_kind":"campaign","operation_id":"camp-001","revision":"rev-1","preset":"opendevops-development","suite":"ifeval_subset","report_root":".local.dev/campaign/camp-test","content_hash":"abcdef","dimensions":{},"budget":{},"authority_hashes":{},"endpoint_class":"ollama-local","provider":"ollama"}`
	runner := &stubDraftRunner{stdoutJSON: campSummary}
	writer := &stubDraftTempFileWriter{}
	deps := newDraftDepsForTest(runner, writer)

	cmd := evalCampaignDraftCmdWithDeps(deps)
	setDraftFlags(t, cmd, map[string]string{
		"preset":                "opendevops-development",
		"out":                  "/tmp/camp-out.json",
		"operation-id":         "camp-001",
		"report-root":          ".local.dev/campaign/camp-test",
		"gold-set-path":        ".local.dev/campaign/gold.json",
		"gold-set-sha256":      strings.Repeat("a", 64),
		"evidence-key-path":    ".local.dev/campaign/key.json",
		"evidence-key-id":      "key-001",
		"provider":             "ollama",
		"endpoint-class":       "ollama-local",
		"campaign-id":          "test-campaign",
		"release-version":      "v2.1.8",
		"preregistration-path":  ".local.dev/campaign/prereg.json",
		"preregistration-sha256": strings.Repeat("b", 64),
		"profile-path":         ".local.dev/campaign/profile.json",
		"profile-sha256":      strings.Repeat("c", 64),
		"model-registry-path":   ".local.dev/campaign/registry.json",
		"model-registry-sha256": strings.Repeat("d", 64),
		"cohort-ids":           "cohort-qwen3-8b",
	})
	require.NoError(t, cmd.Flags().Set("json", "true"))

	var stdout strings.Builder
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	require.NoError(t, cmd.Execute())

	var summary evalDraftSummary
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &summary))
	assert.Equal(t, "campaign", summary.OperationKind)
}

func TestEvalCampaignDraftCmd_RegistersWithExpectedFlags(t *testing.T) {
	cmd := evalCampaignDraftCmdWithDeps(newDraftDepsForTest(&stubDraftRunner{}, &stubDraftTempFileWriter{}))
	flags := cmd.Flags()
	expectedFlags := []string{
		"preset", "out", "operation-id", "revision", "report-root",
		"gold-set-path", "gold-set-sha256", "evidence-key-path", "evidence-key-id",
		"provider", "endpoint-class", "campaign-id", "release-version",
		"preregistration-path", "preregistration-sha256",
		"profile-path", "profile-sha256",
		"model-registry-path", "model-registry-sha256",
		"cohort-ids", "json",
	}
	for _, name := range expectedFlags {
		assert.NotNil(t, flags.Lookup(name), "flag %s should be registered", name)
	}
}

// ---------------------------------------------------------------------------
// Command tree registration
// ---------------------------------------------------------------------------


func TestEvalDiagnosticCmd_RegistersDraftSubcommand(t *testing.T) {
	cmd := evalDiagnosticCmdWithDeps(newDraftDepsForTest(&stubDraftRunner{}, &stubDraftTempFileWriter{}))
	subs := cmd.Commands()
	require.Len(t, subs, 1)
	assert.Equal(t, "draft", subs[0].Use)
}

func TestEvalCampaignCmd_RegistersDraftSubcommand(t *testing.T) {
	cmd := evalCampaignCmdWithDeps(newDraftDepsForTest(&stubDraftRunner{}, &stubDraftTempFileWriter{}))
	subs := cmd.Commands()
	require.Len(t, subs, 1)
	assert.Equal(t, "draft", subs[0].Use)
}

func TestEvalCmd_RegistersDiagnosticAndCampaign(t *testing.T) {
	cmd := evalCmd()
	names := make([]string, 0)
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Use)
	}
	assert.Contains(t, names, "diagnostic")
	assert.Contains(t, names, "campaign")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------


func setDraftFlags(t *testing.T, cmd *cobra.Command, flags map[string]string) {
	t.Helper()
	for name, value := range flags {
		require.NoError(t, cmd.Flags().Set(name, value), "setting flag %s", name)
	}
}
