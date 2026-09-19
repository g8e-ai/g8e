// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"fmt"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nilStatusBackend returns a nil status without an error, exercising the
// fail-closed nil-status path that stubBackend's default prevents.
type nilStatusBackend struct{}

func (nilStatusBackend) Generate(ctx context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	return nil, fmt.Errorf("nilStatusBackend: generate: %w", constants.ErrInferenceBackendUnavailable)
}

func (nilStatusBackend) Status(ctx context.Context) (*models.BackendStatus, error) {
	return nil, nil
}

func readinessTestConfig() config.InferenceConfig {
	return config.InferenceConfig{
		Enabled:        true,
		Backend:        "ollama",
		PrimaryModel:   "gemma3:4b",
		AssistantModel: "llama3.2:3b",
		LiteModel:      "qwen3:1.5b",
	}
}

func TestVerifyProviderReady_ConfiguredModelsPresent(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusResp: &models.BackendStatus{
		Available: true,
		Models:    []string{"gemma3:4b", "llama3.2:3b", "qwen3:1.5b"},
	}}

	err := VerifyProviderReady(context.Background(), backend, readinessTestConfig())
	require.NoError(t, err)
}

func TestVerifyProviderReady_UntaggedModelMatchesLatestTag(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusResp: &models.BackendStatus{
		Available: true,
		Models:    []string{"gemma3:latest", "llama3.2:3b", "qwen3:1.5b"},
	}}
	cfg := readinessTestConfig()
	cfg.PrimaryModel = "gemma3"

	err := VerifyProviderReady(context.Background(), backend, cfg)
	require.NoError(t, err, "Ollama resolves an untagged model name to :latest; readiness must match the provider's resolution")
}

func TestVerifyProviderReady_MissingConfiguredModelReturnsErrInferenceModelNotFound(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusResp: &models.BackendStatus{
		Available: true,
		Models:    []string{"gemma3:4b", "qwen3:1.5b"},
	}}

	err := VerifyProviderReady(context.Background(), backend, readinessTestConfig())

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
	assert.Contains(t, err.Error(), "llama3.2:3b", "the error must name the missing configured model")
}

func TestVerifyProviderReady_UnreachableProviderPropagatesUnavailable(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusErr: constants.ErrInferenceBackendUnavailable}

	err := VerifyProviderReady(context.Background(), backend, readinessTestConfig())

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestVerifyProviderReady_MalformedStatusPropagatesInvalid(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusErr: constants.ErrInferenceProviderResponseInvalid}

	err := VerifyProviderReady(context.Background(), backend, readinessTestConfig())

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
}

func TestVerifyProviderReady_UnavailableStatusReturnsErrInferenceBackendUnavailable(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusResp: &models.BackendStatus{Available: false}}

	err := VerifyProviderReady(context.Background(), backend, readinessTestConfig())

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestVerifyProviderReady_NilStatusReturnsErrInferenceBackendUnavailable(t *testing.T) {
	t.Parallel()

	err := VerifyProviderReady(context.Background(), nilStatusBackend{}, readinessTestConfig())

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestVerifyProviderReady_NilBackendFailsClosed(t *testing.T) {
	t.Parallel()

	err := VerifyProviderReady(context.Background(), nil, readinessTestConfig())

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendNotRegistered)
}

func TestVerifyProviderReady_NoConfiguredModelsPasses(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusResp: &models.BackendStatus{Available: true}}
	cfg := readinessTestConfig()
	cfg.PrimaryModel = ""
	cfg.AssistantModel = ""
	cfg.LiteModel = ""

	err := VerifyProviderReady(context.Background(), backend, cfg)
	require.NoError(t, err, "unconfigured roles fail closed at request time; readiness only verifies configured models")
}
