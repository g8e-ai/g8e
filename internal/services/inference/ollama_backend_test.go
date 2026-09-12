// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaBackend_GenerateConstructsCorrectChatRequest(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	var capturedBody ollamaChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		resp := ollamaChatResponse{
			Model:   capturedBody.Model,
			Message: ollamaChatMessage{Role: "assistant", Content: "generated text"},
			Done:    true,
			DoneReason: "stop",
			PromptEvalCount: 12,
			EvalCount:       8,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:        models.InferenceModelRolePrimary,
		Model:       "gemma3:4b",
		Prompt:      "Hello, world",
		Temperature: 0.7,
		MaxTokens:   100,
		KeepAlive:   "-1",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "generated text", resp.Text)
	assert.Equal(t, "gemma3:4b", resp.Model)
	assert.Equal(t, int32(12), resp.PromptTokens)
	assert.Equal(t, int32(8), resp.CompletionTokens)
	assert.Equal(t, int32(20), resp.TotalTokens)
	assert.Equal(t, "stop", resp.FinishReason)

	// Verify the request was constructed correctly
	assert.Equal(t, "gemma3:4b", capturedBody.Model)
	assert.False(t, capturedBody.Stream)
	assert.Len(t, capturedBody.Messages, 1)
	assert.Equal(t, "user", capturedBody.Messages[0].Role)
	assert.Equal(t, "Hello, world", capturedBody.Messages[0].Content)
	assert.Equal(t, float32(0.7), capturedBody.Options.Temperature)
	assert.Equal(t, int32(100), capturedBody.Options.NumPredict)
	assert.Equal(t, "-1", capturedBody.KeepAlive)
}

func TestOllamaBackend_GenerateHandlesAllThreeRoles(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	roles := []models.InferenceModelRole{
		models.InferenceModelRolePrimary,
		models.InferenceModelRoleAssistant,
		models.InferenceModelRoleLite,
	}
	modelsList := []string{"gemma3:4b", "llama3.2:3b", "qwen3:1.5b"}

	for i, role := range roles {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := ollamaChatResponse{
				Model:     modelsList[i],
				Message:   ollamaChatMessage{Role: "assistant", Content: "ok"},
				Done:      true,
				DoneReason: "stop",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
		resp, err := backend.Generate(context.Background(), models.GenerateRequest{
			Role:  role,
			Model: modelsList[i],
		})

		require.NoError(t, err)
		assert.Equal(t, modelsList[i], resp.Model, "role %d should use model %s", i, modelsList[i])
	}
}

func TestOllamaBackend_GenerateEmptyModelReturnsErrInferenceModelRefInvalid(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend, err := NewOllamaBackend("http://127.0.0.1:11434", logger)
	require.NoError(t, err)

	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceModelRefInvalid)
}

func TestOllamaBackend_GenerateUnavailableReturnsErrInferenceBackendUnavailable(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	// Use a port that's almost certainly not listening
	backend, err := NewOllamaBackend("http://127.0.0.1:1", logger)
	require.NoError(t, err)

	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestOllamaBackend_GenerateNonOKStatusReturnsErrInferenceGenerateFailed(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceGenerateFailed)
}

func TestOllamaBackend_GenerateRequestTimeoutReturnsErrInferenceBackendTimeout(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestTimeout)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendTimeout)
}

func TestOllamaBackend_GenerateContextCancelledReturnsErrInferenceBackendTimeout(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	// Server that hangs forever
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	resp, err := backend.Generate(ctx, models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendTimeout)
}

func TestOllamaBackend_GenerateParsesResponseWithoutDoneReason(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaChatResponse{
			Model:           "test-model",
			Message:         ollamaChatMessage{Role: "assistant", Content: "result"},
			Done:            true,
			PromptEvalCount: 5,
			EvalCount:       3,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.NoError(t, err)
	assert.Equal(t, "result", resp.Text)
	assert.Equal(t, "stop", resp.FinishReason, "empty done_reason with done=true should default to stop")
}

func TestOllamaBackend_StatusReturnsAvailableAndModels(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)

		resp := ollamaTagsResponse{
			Models: []ollamaTagModel{
				{Name: "gemma3:4b"},
				{Name: "llama3.2:3b"},
				{Name: "qwen3:1.5b"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	status, err := backend.Status(context.Background())

	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.Available)
	assert.Len(t, status.Models, 3)
	assert.Contains(t, status.Models, "gemma3:4b")
	assert.Contains(t, status.Models, "llama3.2:3b")
	assert.Contains(t, status.Models, "qwen3:1.5b")
}

func TestOllamaBackend_StatusUnavailableReturnsErrInferenceBackendUnavailable(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	backend, err := NewOllamaBackend("http://127.0.0.1:1", logger)
	require.NoError(t, err)

	status, err := backend.Status(context.Background())

	require.Error(t, err)
	assert.Nil(t, status)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestOllamaBackend_StatusNonOKReturnsErrInferenceBackendUnavailable(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	status, err := backend.Status(context.Background())

	require.Error(t, err)
	assert.Nil(t, status)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
}

func TestOllamaBackend_StatusContextCancelledReturnsErrInferenceBackendTimeout(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	status, err := backend.Status(ctx)

	require.Error(t, err)
	assert.Nil(t, status)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendTimeout)
}

func TestOllamaBackend_TrimsTrailingSlashFromEndpoint(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		resp := ollamaTagsResponse{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL+"/", logger)
	require.NoError(t, err)
	_, err = backend.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/api/tags", capturedPath, "trailing slash should be trimmed")
}

func TestOllamaBackend_NewRejectsInvalidEndpoint(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	cases := []struct {
		name     string
		endpoint string
	}{
		{name: "empty", endpoint: ""},
		{name: "unparseable", endpoint: "://missing-scheme"},
		{name: "unsupported scheme", endpoint: "ftp://192.168.1.2:11434"},
		{name: "missing host", endpoint: "http://"},
		{name: "scheme-relative", endpoint: "//192.168.1.2:11434"},
		{name: "bare host", endpoint: "192.168.1.2:11434"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backend, err := NewOllamaBackend(tc.endpoint, logger)
			require.Error(t, err)
			assert.Nil(t, backend)
			assert.ErrorIs(t, err, constants.ErrInferenceEndpointInvalid)
		})
	}
}

func TestOllamaBackend_GenerateErrorDoesNotLeakProviderResponseBody(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	const providerCanary = "provider-secret-canary-text"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"` + providerCanary + `"}`))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceGenerateFailed)
	assert.NotContains(t, err.Error(), providerCanary, "provider response text must not leak into errors")
}

func TestOllamaBackend_GenerateCallerCancellationReturnsContextCanceled(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := backend.Generate(ctx, models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, context.Canceled, "caller cancellation must stay distinguishable from a deadline")
	assert.NotErrorIs(t, err, constants.ErrInferenceBackendTimeout)
}

func TestOllamaBackend_GeneratePreservesUnderlyingTransportCause(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	backend, err := NewOllamaBackend("http://127.0.0.1:1", logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendUnavailable)
	var urlErr *url.Error
	assert.ErrorAs(t, err, &urlErr, "the underlying transport error must stay in the chain")
}

func TestOllamaBackend_GenerateMalformedResponseReturnsErrInferenceProviderResponseInvalid(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not json"))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
}

func TestOllamaBackend_GenerateOversizedResponseReturnsErrInferenceProviderResponseInvalid(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	backend.maxResponseBytes = 1024

	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "test-model",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
}

func TestOllamaBackend_GenerateModelMissingReturnsErrInferenceModelNotFound(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model 'missing:1b' not found"}`))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "missing:1b",
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceModelNotFound)
}

func TestOllamaBackend_StatusMalformedResponseReturnsErrInferenceProviderResponseInvalid(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not json"))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	status, err := backend.Status(context.Background())

	require.Error(t, err)
	assert.Nil(t, status)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
}

func TestOllamaBackend_EndpointPathPrefixPreserved(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		resp := ollamaTagsResponse{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL+"/ollama", logger)
	require.NoError(t, err)
	_, err = backend.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/ollama/api/tags", capturedPath, "a path-prefixed endpoint must keep its prefix")
}
