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
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func ultraLightSpeedsterTestVariants() []*evalv1.ModelVariant {
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	return []*evalv1.ModelVariant{
		{
			VariantId:      "phi35-mini-38b-speed",
			ProviderClass:  "ollama",
			ServedModelTag: "phi3.5:3.8b-mini-instruct-q4_K_M",
			ModelDigest:    digest,
			ModelFamily:    "Phi-3.5",
			ParameterCount: 3_800_000_000,
			Quantization:   "Q4_K_M",
		},
		{
			VariantId:      "gemma2-2b-speed",
			ProviderClass:  "ollama",
			ServedModelTag: "gemma2:2b-instruct-q4_K_M",
			ModelDigest:    digest,
			ModelFamily:    "Gemma 2",
			ParameterCount: 2_000_000_000,
			Quantization:   "Q4_K_M",
		},
		{
			VariantId:      "qwen25-05b-speed",
			ProviderClass:  "ollama",
			ServedModelTag: "qwen2.5:0.5b-instruct-q4_K_M",
			ModelDigest:    digest,
			ModelFamily:    "Qwen 2.5",
			ParameterCount: 500_000_000,
			Quantization:   "Q4_K_M",
		},
	}
}

func setupCampaignFormationsEnv(t *testing.T, variants []*evalv1.ModelVariant) (root string, deps nativeEvalDeps, cleanup func()) {
	t.Helper()
	root, deps, _, cleanup = setupCampaignOrchestrateEnv(t)
	freeze, err := evaluation.MaterializeModelRegistry("formation-smoke", variants)
	require.NoError(t, err)
	fileSvc, err := fs.NewRuntimeFileService(root, slog.Default())
	require.NoError(t, err)
	payload, err := modelInventoryFreezeJSON(freeze)
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), evaluation.DefaultModelInventoryRelPath, payload, constants.PermFileReadOnly))
	certPath, keyPath := writeInferenceAppCredentialFiles(t, root)
	t.Setenv(string(constants.EnvVar.AppCert), certPath)
	t.Setenv(string(constants.EnvVar.AppKey), keyPath)
	return root, deps, cleanup
}

func TestCampaignEvalFormationsList_ShowCatalog(t *testing.T) {
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"campaign", "formations", "list", "--project-root", t.TempDir()})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "ultra-light-speedster")

	output.Reset()
	command.SetArgs([]string{"campaign", "formations", "show", "ultra-light-speedster", "--project-root", t.TempDir()})
	require.NoError(t, command.Execute())
	assert.Contains(t, output.String(), "phi3.5:3.8b-mini-instruct-q4_K_M")
}

func TestCampaignEvalFormationsList_JSON(t *testing.T) {
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	rootCmd := cmdtest.GlobalJSONRoot(t, command)
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetArgs([]string{"eval", "campaign", "formations", "list", "--project-root", t.TempDir()})
	require.NoError(t, rootCmd.Execute())

	var payload formationListJSON
	require.NoError(t, json.Unmarshal(output.Bytes(), &payload))
	require.Len(t, payload.Formations, 4)
}

func TestCampaignEvalFormationsRun_RequiresInferenceSession(t *testing.T) {
	root, deps, _, cleanup := setupCampaignOrchestrateEnv(t)
	defer cleanup()

	command := evalCmdWithConfig(deps)
	command.SetArgs([]string{"campaign", "formations", "run", "--project-root", root})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--inference-session is required")
}

func TestCampaignEvalFormationsRun_RejectsMissingRegistryVariant(t *testing.T) {
	root, deps, cleanup := setupCampaignFormationsEnv(t, []*evalv1.ModelVariant{
		ultraLightSpeedsterTestVariants()[0],
	})
	defer cleanup()
	restore := enableCampaignWitnessGateway(t, root, deps)
	defer restore()

	command := evalCmdWithConfig(deps)
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs([]string{
		"campaign", "formations", "run",
		"--project-root", root,
		"--inference-session", "infer-session",
		"--data-session", "data-session",
	})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationRegistryBinding)
}

func TestCampaignEvalFormationsRun_RejectsServedTagMismatch(t *testing.T) {
	variants := ultraLightSpeedsterTestVariants()
	variants[0].ServedModelTag = "wrong:tag"
	root, deps, cleanup := setupCampaignFormationsEnv(t, variants)
	defer cleanup()
	restore := enableCampaignWitnessGateway(t, root, deps)
	defer restore()

	command := evalCmdWithConfig(deps)
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs([]string{
		"campaign", "formations", "run",
		"--project-root", root,
		"--inference-session", "infer-session",
		"--data-session", "data-session",
	})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationStackMismatch)
}

func TestCampaignEvalFormationsRun_RejectsUnknownInferenceSession(t *testing.T) {
	root, deps, cleanup := setupCampaignFormationsEnv(t, ultraLightSpeedsterTestVariants())
	defer cleanup()
	restore := enableCampaignWitnessGateway(t, root, deps)
	defer restore()

	command := evalCmdWithConfig(deps)
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs([]string{
		"campaign", "formations", "run",
		"--project-root", root,
		"--inference-session", "missing-inference-session",
		"--data-session", "data-session",
	})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceOperatorNotCapable)
}

func TestCampaignEvalFormationsRun_RejectsUnknownDataSession(t *testing.T) {
	root, deps, cleanup := setupCampaignFormationsEnv(t, ultraLightSpeedsterTestVariants())
	defer cleanup()
	restore := enableCampaignWitnessGateway(t, root, deps)
	defer restore()

	command := evalCmdWithConfig(deps)
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs([]string{
		"campaign", "formations", "run",
		"--project-root", root,
		"--inference-session", "infer-session",
		"--data-session", "missing-data-session",
	})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "campaign data operator session")
}

func TestCampaignEvalFormationsRun_RejectsInferenceSessionAsDataSession(t *testing.T) {
	root, deps, cleanup := setupCampaignFormationsEnv(t, ultraLightSpeedsterTestVariants())
	defer cleanup()
	restore := enableCampaignWitnessGateway(t, root, deps)
	defer restore()

	command := evalCmdWithConfig(deps)
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs([]string{
		"campaign", "formations", "run",
		"--project-root", root,
		"--inference-session", "infer-session",
		"--data-session", "infer-session",
	})
	err := command.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "campaign data operator session")
}

func TestCampaignEvalFormationsShow_RejectsUnknownFormation(t *testing.T) {
	command := evalCmdWithConfig(panickingNativeEvalDeps(t))
	command.SilenceUsage = true
	command.SilenceErrors = true
	command.SetArgs([]string{"campaign", "formations", "show", "not-a-real-formation", "--project-root", t.TempDir()})
	err := command.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFormationInvalid)
}
