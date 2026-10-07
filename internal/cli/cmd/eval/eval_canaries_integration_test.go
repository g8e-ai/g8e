// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package eval

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestGatesChat_RunsTheEnvironmentCanariesFirstAndRunsNoCaseWhenOneFails(t *testing.T) {
	env := setupRunEnv(t)
	canaries := &recordingCanaryRunner{err: canaryFailure(evaluation.CanaryWorkspaceReachable)}
	env.deps.canaryRunner = canaries.run

	_, err := env.run(t, "gates", "chat", "--model", "qwen3:4b", "--ensemble-url", "http://ensemble.invalid:9")

	require.ErrorIs(t, err, constants.ErrEvaluationEnvironmentCanaryFailed)
	assert.Contains(t, err.Error(), string(evaluation.CanaryWorkspaceReachable))
	require.Len(t, canaries.calls, 1)
	assert.Equal(t, canaryOptions{Model: "qwen3:4b", EnsembleURL: "http://ensemble.invalid:9"}, canaries.calls[0])
}

func TestGatesChat_RequiresAModelBeforeRunningCanaries(t *testing.T) {
	env := setupRunEnv(t)
	canaries := &recordingCanaryRunner{}
	env.deps.canaryRunner = canaries.run

	_, err := env.run(t, "gates", "chat")

	require.Error(t, err)
	assert.Empty(t, canaries.calls)
}

func TestGatesChat_HelpDescribesTheCanaries(t *testing.T) {
	env := setupRunEnv(t)

	out := env.mustRun(t, "gates", "chat", "--help")

	for _, canary := range []evaluation.CanaryID{
		evaluation.CanaryToolsDeclared, evaluation.CanarySeedDelivered, evaluation.CanaryWorkspaceReachable,
		evaluation.CanaryGuidanceDelivered, evaluation.CanarySemanticJudge, evaluation.CanaryRegistryMCP,
	} {
		assert.Contains(t, out, string(canary))
	}
	assert.Contains(t, out, "ENVIRONMENT ERROR")
}

func rolloutCanaryOptions(t *testing.T) rolloutRunOptions {
	t.Helper()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	t.Cleanup(healthy.Close)
	return rolloutRunOptions{SkipVerified: true, GateSmoke: true, EnsembleHealthURL: healthy.URL, MirrorBootstrapURL: healthy.URL}
}

func TestRolloutRun_GateSmokeAbortsBeforeAnyModelWhenACanaryFails(t *testing.T) {
	env := setupRunEnv(t)
	env.mustRun(t, "rollout", "add", "qwen3:4b")
	canaries := &recordingCanaryRunner{err: canaryFailure(evaluation.CanaryToolsDeclared)}
	env.deps.canaryRunner = canaries.run

	err := runRolloutWith(env.cmd, env.deps, rolloutCanaryOptions(t), neverRunGate(t))

	require.ErrorIs(t, err, constants.ErrEvaluationEnvironmentCanaryFailed)
	assert.Contains(t, err.Error(), "before any model is allocated")
	require.Len(t, canaries.calls, 1, "canaries run once, not once per model")
	assert.Equal(t, "qwen3:4b", canaries.calls[0].Model)
}

func TestRolloutRun_WithoutGateSmokeRunsNoCanaries(t *testing.T) {
	env := setupRunEnv(t)
	env.mustRun(t, "rollout", "add", "qwen3:4b")
	canaries := &recordingCanaryRunner{err: canaryFailure(evaluation.CanaryToolsDeclared)}
	env.deps.canaryRunner = canaries.run
	opts := rolloutCanaryOptions(t)
	opts.GateSmoke = false
	var out bytes.Buffer
	env.cmd.SetOut(&out)
	env.cmd.SetErr(&out)
	attempted := 0
	gates := func(fs.RuntimeFileService, []*evalv1.ModelVariant) rolloutGateRunner {
		return func(*cobra.Command, evaluation.CampaignQueueModel, string) (*rolloutRunSuccess, error) {
			attempted++
			return nil, context.Canceled
		}
	}

	_ = runRolloutWith(env.cmd, env.deps, opts, gates)

	assert.Empty(t, canaries.calls)
	assert.Equal(t, 1, attempted)
}

func TestRolloutRun_HelpDescribesTheCanaries(t *testing.T) {
	env := setupRunEnv(t)

	out := env.mustRun(t, "rollout", "run", "--help")

	assert.Contains(t, out, "environment canaries")
	assert.Contains(t, out, "ENVIRONMENT ERROR")
}

func TestResolveChatProbeModel_ReadsTheFrozenInventory(t *testing.T) {
	env := setupRunEnv(t)
	fileSvc := env.fileSvc(t)

	found, err := resolveChatProbeModel(context.Background(), fileSvc, env.root, chatAcceptCampaignID, "qwen3:4b")

	require.NoError(t, err)
	assert.Equal(t, testQwenVariant().GetModelDigest(), found.Digest)
	assert.NotEmpty(t, found.RegistryDigest)
	require.Len(t, found.Registry, 1)
	assert.Equal(t, "qwen3:4b", found.Registry[0].GetModel())

	other, err := resolveChatProbeModel(context.Background(), fileSvc, env.root, "another-campaign", "qwen3:4b")
	require.NoError(t, err)
	assert.NotEqual(t, found.RegistryDigest, other.RegistryDigest, "the registry digest is bound to its campaign")

	_, err = resolveChatProbeModel(context.Background(), fileSvc, env.root, chatAcceptCampaignID, "unknown:1b")
	require.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
}
