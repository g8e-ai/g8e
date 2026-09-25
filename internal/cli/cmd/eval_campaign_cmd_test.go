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
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func prepareCampaignRunViaStart(t *testing.T, root string, deps nativeEvalDeps) *evaluation.ActiveCampaignRun {
	t.Helper()
	cmd := silentCobraCommand()
	cmd.SetContext(context.Background())
	cmd.Flags().String("project-root", root, "")

	result, err := runCampaignStartFlow(cmd, deps, campaignStartFlowOptions{
		ModelTag:    "qwen3:4b",
		PrepareOnly: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Plan)

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	active, err := evaluation.LoadActiveCampaignRunFromRuntime(context.Background(), fileSvc)
	require.NoError(t, err)
	require.NotEmpty(t, active.RunID)
	return active
}

func TestCampaignEvalList_EmptyProject(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "list", "--project-root", root})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "No campaigns found")
}

func TestCampaignEvalList_AfterPrepare(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "list", "--project-root", root})
	require.NoError(t, command.Execute())
	out := output.String()
	assert.Contains(t, out, active.CampaignID)
	assert.Contains(t, out, active.RunID)
}

func TestCampaignEvalList_JSON(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	rootCmd := globalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "campaign", "list", "--project-root", root})
	require.NoError(t, rootCmd.Execute())

	var payload campaignListOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	require.NotEmpty(t, payload.Campaigns)
	assert.Equal(t, active.CampaignID, payload.Campaigns[0].CampaignID)
}

func TestCampaignEvalShow_AfterPrepare(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "show", "--project-root", root, active.RunID})
	require.NoError(t, command.Execute())
	out := output.String()
	assert.Contains(t, out, active.RunID)
	assert.Contains(t, out, active.CampaignID)
	assert.Contains(t, out, "Expected assignments")
}

func TestCampaignEvalShow_JSON(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	rootCmd := globalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "campaign", "show", "--project-root", root, active.RunID})
	require.NoError(t, rootCmd.Execute())

	var payload campaignShowOutput
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	assert.Equal(t, active.RunID, payload.RunID)
	assert.Equal(t, active.CampaignID, payload.CampaignID)
}

func TestCampaignEvalStatus_AfterPrepare(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "status", "--project-root", root, active.RunID})
	require.NoError(t, command.Execute())
	out := output.String()
	assert.Contains(t, out, active.RunID)
	assert.Contains(t, out, "Expected assignments")
}

func TestCampaignEvalStatus_UsesActiveRunWhenOmitted(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "status", "--project-root", root})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), active.RunID)
}

func TestCampaignEvalAccount_ReportsIncompleteScheduledRun(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "account", "--project-root", root, active.RunID})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrEvalRunVerificationFailed)
	assert.Contains(t, output.String(), "incomplete")
}

func TestCampaignEvalExport_WritesArtifacts(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)
	outputRelDir := filepath.Join("data", "eval", "runs", active.RunID, "export")

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{
		"campaign", "export", "--project-root", root,
		"--output-dir", outputRelDir, active.RunID,
	})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), active.RunID)

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	exists, err := fileSvc.FileExists(context.Background(), filepath.Join(outputRelDir, constants.EvaluationRunSummaryFilename))
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestCampaignEvalExport_RejectsExternalOutputDir(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	active := prepareCampaignRunViaStart(t, root, deps)
	externalDir := filepath.Join(root, "exports", active.RunID)

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{
		"campaign", "export", "--project-root", root,
		"--output-dir", externalDir, active.RunID,
	})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output directory must be runtime-relative")
}

func TestCampaignEvalInit_RequiresFields(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "init", "--project-root", root})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestCampaignEvalVerify_RejectsMissingRun(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "verify", "--project-root", root, "missing-run-id"})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "campaign verify")
}

func TestPublicRestore_RequiresTarget(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := publicRestoreCmdWithConfig(deps.configLoader, deps.fileSvcFactory)
	command.SetArgs([]string{"--project-root", root})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify --queue or --run-id")
}

func TestPublicRestore_RejectsBothTargets(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := publicRestoreCmdWithConfig(deps.configLoader, deps.fileSvcFactory)
	command.SetArgs([]string{
		"--project-root", root,
		"--queue", "--run-id", "run-1",
	})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestResolveCampaignOllamaEndpoint_UsesFlag(t *testing.T) {
	endpoint, err := resolveCampaignOllamaEndpoint("http://127.0.0.1:11434", nil, "")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:11434", endpoint)
}

func TestCampaignEvalVerify_ViaCLIPersistsAndPublishesPopulationBoundReport(t *testing.T) {
	root, deps, cmd, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()
	defer enableCampaignWitnessGateway(t, root, deps)()
	publication := &recordingCampaignVerificationPublication{}
	deps.campaignPublicationFactory = func(*cobra.Command, fs.RuntimeFileService) (campaignVerificationPublication, error) {
		return publication, nil
	}

	lookup := &campaignTraceLookup{root: root, deps: deps}
	ensemble := newTestEnsembleServer(lookup.resolve)
	defer ensemble.Close()

	cmd.SetOut(&bytes.Buffer{})
	result, err := runCampaignStartFlow(cmd, deps, campaignStartFlowOptions{
		ModelTag:           "qwen3:4b",
		EnsembleURL:        ensemble.URL,
		NoAutoRefresh:      true,
		InferenceSessionID: "infer-session",
		DataSessionID:      "data-session",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Plan)

	command := evalCmdWithConfig(deps)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "verify", "--project-root", root, result.Plan.RunID})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "PASS")

	fileSvc, err := deps.fileSvcFactory(root, nil)
	require.NoError(t, err)
	loaded, err := evaluation.NewStore(fileSvc).LoadCampaignVerification(context.Background(), result.Plan.RunID)
	require.NoError(t, err)
	assertPopulationBoundCampaignReport(t, loaded, 75, 1)
	assert.Equal(t, []string{result.Plan.RunID}, publication.completionRunIDs)
	require.Len(t, publication.reports, 1)
	assertPopulationBoundCampaignReport(t, publication.reports[0], 75, 1)
	assert.Equal(t, loaded.GetReportDigestRef().GetSha256(), publication.reports[0].GetReportDigestRef().GetSha256())
}
