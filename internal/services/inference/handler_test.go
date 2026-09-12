// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// testCommandMessage is a test-only implementation of governance.CommandMessage.
type testCommandMessage struct {
	payload []byte
}

func (m *testCommandMessage) GetPayload() []byte { return m.payload }
func (m *testCommandMessage) SetPayload(p []byte) { m.payload = p }

func mustNewScrubbingSvc(t *testing.T) *scrubbing.ScrubbingService {
	t.Helper()
	svc, err := scrubbing.NewScrubbingService(context.Background(), &scrubbing.Config{Enabled: true, StrictMode: false}, testutil.NewTestLogger(), nil)
	require.NoError(t, err)
	return svc
}

func mustMarshalInferenceRequested(t *testing.T, req *operatorv1.InferenceRequested) []byte {
	t.Helper()
	data, err := proto.Marshal(req)
	require.NoError(t, err)
	return data
}

// wantDigest returns the canonical result digest the handler must return as
// the receipt summary for the given backend response.
func wantDigest(t *testing.T, resp *models.GenerateResponse) string {
	t.Helper()
	digest, err := models.ComputeInferenceResultDigest(resp.ToProtoInferenceResult())
	require.NoError(t, err)
	return digest
}

func TestInferenceHandler_ExecuteVerifiedTransaction_PrimaryRole(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Text:         "primary response",
		FinishReason: "stop",
		Model:        "gemma3:4b",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:       true,
		PrimaryModel:  "gemma3:4b",
		KeepAlive:     "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:    operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt:  "What is 2+2?",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	summary, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, wantDigest(t, backend.generateResp), summary, "summary must be the canonical result digest binding the complete result")
	assert.Equal(t, "gemma3:4b", backend.lastReq.Model, "should use primary model config default")
	assert.Equal(t, "What is 2+2?", backend.lastReq.Prompt)
	assert.Equal(t, "-1", backend.lastReq.KeepAlive)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_AssistantRole(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Text:         "assistant response",
		FinishReason: "stop",
		Model:        "llama3.2:3b",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:         true,
		AssistantModel:  "llama3.2:3b",
		KeepAlive:       "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_ASSISTANT,
		Prompt: "Summarize this",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	summary, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, wantDigest(t, backend.generateResp), summary, "summary must be the canonical result digest binding the complete result")
	assert.Equal(t, "llama3.2:3b", backend.lastReq.Model, "should use assistant model config default")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_LiteRole(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Text:         "lite response",
		FinishReason: "stop",
		Model:        "qwen3:1.5b",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:    true,
		LiteModel: "qwen3:1.5b",
		KeepAlive: "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_LITE,
		Prompt: "triage",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	summary, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, wantDigest(t, backend.generateResp), summary, "summary must be the canonical result digest binding the complete result")
	assert.Equal(t, "qwen3:1.5b", backend.lastReq.Model, "should use lite model config default")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_MatchingModelOverrideAccepted(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Text:         "primary response",
		FinishReason: "stop",
		Model:        "gemma3:4b",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:      true,
		PrimaryModel: "gemma3:4b",
		KeepAlive:   "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:  "gemma3:4b",
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	summary, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, wantDigest(t, backend.generateResp), summary, "summary must be the canonical result digest binding the complete result")
	assert.Equal(t, "gemma3:4b", backend.lastReq.Model, "an override naming the configured role model is permitted")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_UnauthorizedModelOverrideDenied(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:      true,
		PrimaryModel: "gemma3:4b",
		KeepAlive:   "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:  "custom-model:latest",
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelOverrideDenied)
	assert.Equal(t, 0, backend.calls, "backend must not be called for an unauthorized override")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_CrossRoleModelDenied(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:       true,
		PrimaryModel:  "gemma3:4b",
		LiteModel:     "qwen3:1.5b",
		KeepAlive:     "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Model:  "qwen3:1.5b",
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelOverrideDenied,
		"the configured model for a different role is still an unauthorized override")
	assert.Equal(t, 0, backend.calls)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_UnspecifiedRoleRejected(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true, PrimaryModel: "gemma3:4b"}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_UNSPECIFIED,
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceRoleInvalid)
	assert.Equal(t, 0, backend.calls)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_EmptyPayloadReturnsErrPubSubEmptyPayload(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true, PrimaryModel: "gemma3:4b"}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	cmdMsg := &testCommandMessage{payload: nil}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPubSubEmptyPayload)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_InvalidPayloadReturnsError(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true, PrimaryModel: "gemma3:4b"}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	cmdMsg := &testCommandMessage{payload: []byte("not a protobuf message")}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_NilBackendReturnsErrInferenceBackendNotRegistered(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true, PrimaryModel: "gemma3:4b"}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(nil, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendNotRegistered)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_BackendErrorReturnsErrInferenceGenerateFailed(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateErr: constants.ErrInferenceGenerateFailed}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true, PrimaryModel: "gemma3:4b"}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceGenerateFailed)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_NoDefaultModelReturnsErrInferenceModelRefInvalid(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{}
	cfg := &config.Config{Inference: config.InferenceConfig{Enabled: true}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInferenceModelRefInvalid)
}

func TestInferenceHandler_ExecuteVerifiedTransaction_ScrubsPromptBeforeBackend(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Text:         "response",
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:      true,
		PrimaryModel: "test-model",
		KeepAlive:   "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	// Include an email-like pattern that scrubbing should redact
	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "Contact me at user@example.com please",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	// The scrubbed prompt should not contain the raw email
	assert.NotContains(t, backend.lastReq.Prompt, "user@example.com")
}

// TestInferenceHandler_ExecuteVerifiedTransaction_LongOutputStillBindsDigest
// proves that an output larger than ReceiptSummaryMaxBytes still produces a
// fixed-width canonical digest as the receipt summary — the digest binds the
// complete result rather than a truncated text prefix.
func TestInferenceHandler_ExecuteVerifiedTransaction_LongOutputStillBindsDigest(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	longText := make([]byte, constants.ReceiptSummaryMaxBytes+100)
	for i := range longText {
		longText[i] = 'a'
	}
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Text:         string(longText),
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:      true,
		PrimaryModel: "test-model",
		KeepAlive:   "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	summary, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, wantDigest(t, backend.generateResp), summary, "summary must be the digest of the complete untruncated result")
	assert.Len(t, summary, 64, "digest summary is a fixed-width lowercase hex SHA-256")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_AppliesConfigKeepAliveDefault(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Text:         "response",
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:      true,
		PrimaryModel: "test-model",
		KeepAlive:   "30m",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	// Request does not set keep_alive, so config default should be used
	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:   operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt: "test",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, "30m", backend.lastReq.KeepAlive, "config keep_alive default should be applied")
}

func TestInferenceHandler_ExecuteVerifiedTransaction_RequestKeepAliveOverridesConfig(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend := &stubBackend{generateResp: &models.GenerateResponse{
		Text:         "response",
		FinishReason: "stop",
		Model:        "test-model",
	}}
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:      true,
		PrimaryModel: "test-model",
		KeepAlive:   "-1",
	}}
	scrubbingSvc := mustNewScrubbingSvc(t)
	handler := NewInferenceExecutionHandler(backend, cfg, scrubbingSvc, logger)

	payload := mustMarshalInferenceRequested(t, &operatorv1.InferenceRequested{
		Role:      operatorv1.ModelRole_MODEL_ROLE_PRIMARY,
		Prompt:    "test",
		KeepAlive: "5m",
	})
	cmdMsg := &testCommandMessage{payload: payload}

	_, err := handler.ExecuteVerifiedTransaction(context.Background(), constants.Event.Operator.Inference.Requested, cmdMsg)

	require.NoError(t, err)
	assert.Equal(t, "5m", backend.lastReq.KeepAlive, "request keep_alive should override config default")
}

func TestInferenceHandler_DefaultModelForRole_UnspecifiedReturnsEmpty(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	cfg := &config.Config{Inference: config.InferenceConfig{
		Enabled:      true,
		PrimaryModel: "primary",
		AssistantModel: "assistant",
		LiteModel:    "lite",
	}}
	handler := NewInferenceExecutionHandler(&stubBackend{}, cfg, mustNewScrubbingSvc(t), logger)

	assert.Equal(t, "primary", handler.defaultModelForRole(models.InferenceModelRolePrimary))
	assert.Equal(t, "assistant", handler.defaultModelForRole(models.InferenceModelRoleAssistant))
	assert.Equal(t, "lite", handler.defaultModelForRole(models.InferenceModelRoleLite))
	assert.Equal(t, "", handler.defaultModelForRole(models.InferenceModelRoleUnspecified))
}

// Verify the handler implements governance.ExecutionHandler at compile time.
var _ governance.ExecutionHandler = (*InferenceExecutionHandler)(nil)
