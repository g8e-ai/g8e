// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBackend is a test-only Backend implementation for contract tests.
type stubBackend struct {
	generateResp *models.GenerateResponse
	generateErr  error
	statusResp   *models.BackendStatus
	statusErr    error
	lastReq      models.GenerateRequest
	calls        int
}

func (s *stubBackend) Generate(ctx context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	s.calls++
	s.lastReq = req
	if s.generateErr != nil {
		return nil, s.generateErr
	}
	if s.generateResp != nil {
		return s.generateResp, nil
	}
	return &models.GenerateResponse{Text: "stub response", Model: req.Model}, nil
}

func (s *stubBackend) Status(ctx context.Context) (*models.BackendStatus, error) {
	if s.statusErr != nil {
		return nil, s.statusErr
	}
	if s.statusResp != nil {
		return s.statusResp, nil
	}
	return &models.BackendStatus{Available: true, Models: []string{"test-model"}}, nil
}

func TestBackendContract_GenerateReturnsResponse(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{
		generateResp: &models.GenerateResponse{
			Text:             "hello world",
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
			FinishReason:     "stop",
			Model:            "test-model",
		},
	}

	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:   models.InferenceModelRolePrimary,
		Model:  "test-model",
		Prompt: "hi",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "hello world", resp.Text)
	assert.Equal(t, int32(10), resp.PromptTokens)
	assert.Equal(t, int32(5), resp.CompletionTokens)
	assert.Equal(t, int32(15), resp.TotalTokens)
	assert.Equal(t, "stop", resp.FinishReason)
	assert.Equal(t, "test-model", resp.Model)
}

func TestBackendContract_GeneratePropagatesError(t *testing.T) {
	t.Parallel()
	backendErr := constants.ErrInferenceGenerateFailed
	backend := &stubBackend{generateErr: backendErr}

	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, backendErr)
}

func TestBackendContract_StatusReturnsModels(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{
		statusResp: &models.BackendStatus{
			Available: true,
			Models:    []string{"gemma3:4b", "llama3.2:3b", "qwen3:1.5b"},
		},
	}

	status, err := backend.Status(context.Background())

	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.Available)
	assert.Len(t, status.Models, 3)
	assert.Contains(t, status.Models, "gemma3:4b")
	assert.Contains(t, status.Models, "llama3.2:3b")
	assert.Contains(t, status.Models, "qwen3:1.5b")
}

func TestBackendContract_StatusPropagatesError(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{statusErr: constants.ErrInferenceBackendUnavailable}

	status, err := backend.Status(context.Background())

	require.Error(t, err)
	assert.Nil(t, status)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestBackendContract_GeneratePassesAllRoles(t *testing.T) {
	t.Parallel()
	roles := []models.InferenceModelRole{
		models.InferenceModelRolePrimary,
		models.InferenceModelRoleAssistant,
		models.InferenceModelRoleLite,
	}

	for _, role := range roles {
		backend := &stubBackend{}
		_, err := backend.Generate(context.Background(), models.GenerateRequest{
			Role:  role,
			Model: "test-model",
		})
		require.NoError(t, err)
		assert.Equal(t, role, backend.lastReq.Role, "role should be passed through for role %d", role)
	}
}

func TestBackendContract_GenerateRespectsContextCancellation(t *testing.T) {
	t.Parallel()
	backend := &stubBackend{generateErr: constants.ErrInferenceBackendTimeout}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := backend.Generate(ctx, models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendTimeout)
}

func TestBackendRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()
	backend := &stubBackend{}

	err := registry.Register("ollama", backend)
	require.NoError(t, err)

	got, err := registry.Get("ollama")
	require.NoError(t, err)
	assert.Same(t, backend, got)
}

func TestBackendRegistry_RegisterNilFailsClosed(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()

	err := registry.Register("ollama", nil)
	require.Error(t, err)
}

func TestBackendRegistry_RegisterEmptyNameFailsClosed(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()
	backend := &stubBackend{}

	err := registry.Register("", backend)
	require.Error(t, err)
}

func TestBackendRegistry_RegisterDuplicateFailsClosed(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()
	backend := &stubBackend{}

	require.NoError(t, registry.Register("ollama", backend))
	err := registry.Register("ollama", &stubBackend{})
	require.Error(t, err)
}

func TestBackendRegistry_GetUnregisteredReturnsError(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()

	_, err := registry.Get("nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendNotRegistered)
}

func TestRegisterBackends_RegistersAll(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()
	backends := map[string]Backend{
		"ollama": &stubBackend{},
		"stub":   &stubBackend{},
	}

	err := RegisterBackends(registry, backends)
	require.NoError(t, err)

	_, err = registry.Get("ollama")
	assert.NoError(t, err)
	_, err = registry.Get("stub")
	assert.NoError(t, err)
}

func TestRegisterBackends_NilRegistryFailsClosed(t *testing.T) {
	t.Parallel()
	err := RegisterBackends(nil, map[string]Backend{"ollama": &stubBackend{}})
	require.Error(t, err)
}

func TestRegisterBackends_NilBackendFailsClosed(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()
	err := RegisterBackends(registry, map[string]Backend{"ollama": nil})
	require.Error(t, err)
}

func TestRegisterBackends_DuplicateFailsClosed(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()
	err := RegisterBackends(registry, map[string]Backend{
		"ollama": &stubBackend{},
		"stub":   &stubBackend{},
	})
	require.NoError(t, err)

	err = RegisterBackends(registry, map[string]Backend{"ollama": &stubBackend{}})
	require.Error(t, err)
}

func TestRegisterBackends_PreservesErrorChain(t *testing.T) {
	t.Parallel()
	registry := NewBackendRegistry()
	originalErr := errors.New("custom backend error")
	backend := &stubBackend{generateErr: originalErr}

	err := RegisterBackends(registry, map[string]Backend{"ollama": backend})
	require.NoError(t, err)

	got, err := registry.Get("ollama")
	require.NoError(t, err)
	_, genErr := got.Generate(context.Background(), models.GenerateRequest{Model: "test"})
	assert.ErrorIs(t, genErr, originalErr)
}
