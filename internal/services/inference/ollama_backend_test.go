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
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaBackend_GenerateConstructsCorrectChatRequest(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	topP := float32(0.8)
	topK := int32(40)
	seed := int32(424242)
	contextLimit := int32(8192)
	parallelToolCalls := true

	var capturedBody ollamaChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &capturedBody))

		promptTokens := int32(12)
		completionTokens := int32(8)
		loadDuration := int64(2_000_000)
		promptEvalDuration := int64(10_000_000)
		generationDuration := int64(40_000_000)
		totalDuration := int64(52_000_000)
		resp := ollamaChatResponse{
			Model: capturedBody.Model,
			Message: ollamaChatMessage{
				Role:    "assistant",
				Content: "generated text",
				ToolCalls: []ollamaToolCall{{
					ID: "call-2",
					Function: ollamaToolFunction{
						Name:      "inspect",
						Arguments: json.RawMessage(`{"path":"next.txt"}`),
					},
				}},
			},
			Done:               true,
			DoneReason:         "stop",
			PromptEvalCount:    &promptTokens,
			EvalCount:          &completionTokens,
			LoadDuration:       &loadDuration,
			PromptEvalDuration: &promptEvalDuration,
			EvalDuration:       &generationDuration,
			TotalDuration:      &totalDuration,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "gemma3:4b",
		Messages: []*operatorv1.InferenceMessage{
			{
				Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_SYSTEM,
				Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: "Be precise"}}},
			},
			{
				Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
				Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: "Hello, world"}}},
			},
			{
				Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT,
				Parts: []*operatorv1.InferenceMessagePart{
					{Part: &operatorv1.InferenceMessagePart_Text{Text: "Checking both targets"}},
					{Part: &operatorv1.InferenceMessagePart_ToolCall{ToolCall: &operatorv1.InferenceToolCall{
						CallId:        "call-1",
						Name:          "inspect",
						ArgumentsJson: `{"path":"target.txt"}`,
					}}},
					{Part: &operatorv1.InferenceMessagePart_ToolCall{ToolCall: &operatorv1.InferenceToolCall{
						CallId:        "call-2",
						Name:          "inspect",
						ArgumentsJson: `{"path":"second.txt"}`,
					}}},
				},
			},
			{
				Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL,
				Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_ToolResult{ToolResult: &operatorv1.InferenceToolResult{
					CallId:     "call-1",
					Name:       "inspect",
					ResultJson: `{"ok":true}`,
				}}}},
			},
		},
		Tools: []*operatorv1.InferenceToolDeclaration{{
			Name:        "inspect",
			Description: "Inspect a target",
			JsonSchema:  `{"properties":{"path":{"type":"string"}},"type":"object"}`,
		}},
		Temperature:   0.7,
		MaxTokens:     100,
		KeepAlive:     "-1",
		TopP:          &topP,
		TopK:          &topK,
		Seed:          &seed,
		StopSequences: []string{"END", "STOP"},
		ResponseFormat: &operatorv1.InferenceResponseFormat{
			MediaType:  "application/json",
			JsonSchema: `{"properties":{"answer":{"type":"string"}},"type":"object"}`,
		},
		ToolChoice: &operatorv1.InferenceToolChoice{
			Mode:             operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO,
			AllowedToolNames: []string{"inspect"},
		},
		ParallelToolCalls: &parallelToolCalls,
		Thinking: &operatorv1.InferenceThinkingControl{
			Mode:            &operatorv1.InferenceThinkingControl_Enabled{Enabled: true},
			IncludeThoughts: true,
		},
		ContextLimit: &contextLimit,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Parts, 2)
	assert.Equal(t, "generated text", resp.Parts[0].GetText())
	assert.Equal(t, "call-2", resp.Parts[1].GetToolCall().GetCallId())
	assert.Equal(t, "inspect", resp.Parts[1].GetToolCall().GetName())
	assert.Equal(t, `{"path":"next.txt"}`, resp.Parts[1].GetToolCall().GetArgumentsJson())
	assert.Equal(t, "gemma3:4b", resp.Model)
	assert.Equal(t, int32(12), resp.PromptTokens)
	assert.Equal(t, int32(8), resp.CompletionTokens)
	assert.Equal(t, int32(20), resp.TotalTokens)
	assert.True(t, resp.UsageReported)
	require.NotNil(t, resp.LoadDurationNS)
	assert.Equal(t, int64(2_000_000), *resp.LoadDurationNS)
	require.NotNil(t, resp.PromptEvalDurationNS)
	assert.Equal(t, int64(10_000_000), *resp.PromptEvalDurationNS)
	require.NotNil(t, resp.GenerationDurationNS)
	assert.Equal(t, int64(40_000_000), *resp.GenerationDurationNS)
	require.NotNil(t, resp.TotalDurationNS)
	assert.Equal(t, int64(52_000_000), *resp.TotalDurationNS)
	assert.Equal(t, operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_PROVIDER, resp.TimingSource)
	assert.Equal(t, "stop", resp.FinishReason)

	// Verify the request was constructed correctly
	assert.Equal(t, "gemma3:4b", capturedBody.Model)
	assert.True(t, capturedBody.Stream)
	require.Len(t, capturedBody.Messages, 4)
	assert.Equal(t, ollamaChatMessage{Role: "system", Content: "Be precise"}, capturedBody.Messages[0])
	assert.Equal(t, ollamaChatMessage{Role: "user", Content: "Hello, world"}, capturedBody.Messages[1])
	require.Len(t, capturedBody.Messages[2].ToolCalls, 2)
	assert.Equal(t, "assistant", capturedBody.Messages[2].Role)
	assert.Equal(t, "Checking both targets", capturedBody.Messages[2].Content)
	assert.Equal(t, "call-1", capturedBody.Messages[2].ToolCalls[0].ID)
	assert.Equal(t, "inspect", capturedBody.Messages[2].ToolCalls[0].Function.Name)
	assert.JSONEq(t, `{"path":"target.txt"}`, string(capturedBody.Messages[2].ToolCalls[0].Function.Arguments))
	assert.Equal(t, "call-2", capturedBody.Messages[2].ToolCalls[1].ID)
	assert.JSONEq(t, `{"path":"second.txt"}`, string(capturedBody.Messages[2].ToolCalls[1].Function.Arguments))
	assert.Equal(t, "tool", capturedBody.Messages[3].Role)
	assert.Equal(t, "call-1", capturedBody.Messages[3].ToolCallID)
	assert.Equal(t, "inspect", capturedBody.Messages[3].ToolName)
	assert.Equal(t, `{"ok":true}`, capturedBody.Messages[3].Content)
	require.Len(t, capturedBody.Tools, 1)
	assert.Equal(t, "function", capturedBody.Tools[0].Type)
	assert.Equal(t, "inspect", capturedBody.Tools[0].Function.Name)
	assert.Equal(t, "Inspect a target", capturedBody.Tools[0].Function.Description)
	assert.JSONEq(t, `{"properties":{"path":{"type":"string"}},"type":"object"}`, string(capturedBody.Tools[0].Function.Parameters))
	assert.Equal(t, float32(0.7), capturedBody.Options.Temperature)
	assert.Equal(t, int32(100), capturedBody.Options.NumPredict)
	require.NotNil(t, capturedBody.Options.TopP)
	assert.Equal(t, topP, *capturedBody.Options.TopP)
	require.NotNil(t, capturedBody.Options.TopK)
	assert.Equal(t, topK, *capturedBody.Options.TopK)
	require.NotNil(t, capturedBody.Options.Seed)
	assert.Equal(t, seed, *capturedBody.Options.Seed)
	assert.Equal(t, []string{"END", "STOP"}, capturedBody.Options.Stop)
	require.NotNil(t, capturedBody.Options.NumCtx)
	assert.Equal(t, contextLimit, *capturedBody.Options.NumCtx)
	assert.Equal(t, "true", string(capturedBody.Think))
	assert.JSONEq(t, `{"properties":{"answer":{"type":"string"}},"type":"object"}`, string(capturedBody.Format))
	assert.Equal(t, json.RawMessage("-1"), capturedBody.KeepAlive)
}

func TestEncodeOllamaKeepAlive(t *testing.T) {
	t.Parallel()
	encoded, err := encodeOllamaKeepAlive("-1")
	require.NoError(t, err)
	assert.Equal(t, json.RawMessage("-1"), encoded)
	encoded, err = encodeOllamaKeepAlive("5m")
	require.NoError(t, err)
	assert.Equal(t, json.RawMessage(`"5m"`), encoded)
}

func TestOllamaBackend_GeneratePreservesOrderedStreamPartsAndMeasuresFirstToken(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ollamaChatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.True(t, request.Stream)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		events := []string{
			`{"model":"test-model","message":{"role":"assistant","thinking":"considering"},"done":false}`,
			`{"model":"test-model","message":{"role":"assistant","content":"answer"},"done":false}`,
			`{"model":"test-model","message":{"role":"assistant","tool_calls":[{"id":"call-1","function":{"name":"inspect","arguments":{"path":"target.txt"}}}]},"done":false}`,
			`{"model":"test-model","message":{"role":"assistant","content":""},"done":true,"done_reason":"tool_calls","prompt_eval_count":4,"eval_count":3,"load_duration":1,"prompt_eval_duration":2,"eval_duration":3,"total_duration":6}`,
		}
		for _, event := range events {
			time.Sleep(time.Millisecond)
			_, err := io.WriteString(w, event+"\n")
			require.NoError(t, err)
			flusher.Flush()
		}
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)

	response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model"})

	require.NoError(t, err)
	require.Len(t, response.Parts, 3)
	assert.Equal(t, "considering", response.Parts[0].GetThinking())
	assert.Equal(t, "answer", response.Parts[1].GetText())
	assert.Equal(t, "call-1", response.Parts[2].GetToolCall().GetCallId())
	assert.Equal(t, `{"path":"target.txt"}`, response.Parts[2].GetToolCall().GetArgumentsJson())
	require.NotNil(t, response.TimeToFirstTokenNS)
	assert.Positive(t, *response.TimeToFirstTokenNS)
	assert.Equal(t, operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_PROVIDER, response.TimingSource)
	assert.Equal(t, "tool_calls", response.FinishReason)
	assert.Equal(t, int32(7), response.TotalTokens)
}

func TestOllamaBackend_GenerateRejectsStreamWithoutOneTerminalEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing terminal",
			body: `{"model":"test-model","message":{"role":"assistant","content":"partial"},"done":false}` + "\n",
		},
		{
			name: "event after terminal",
			body: `{"model":"test-model","message":{"role":"assistant","content":"complete"},"done":true}` + "\n" +
				`{"model":"test-model","message":{"role":"assistant","content":"extra"},"done":false}` + "\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := io.WriteString(w, tt.body)
				require.NoError(t, err)
			}))
			defer server.Close()
			backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
			require.NoError(t, err)

			response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model"})

			require.Error(t, err)
			assert.Nil(t, response)
			assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
		})
	}
}

func TestOllamaBackend_GenerateRejectsParallelToolCallsSplitAcrossEventsWhenDisabled(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		events := []string{
			`{"model":"test-model","message":{"role":"assistant","tool_calls":[{"function":{"name":"inspect","arguments":{}}}]},"done":false}`,
			`{"model":"test-model","message":{"role":"assistant","tool_calls":[{"function":{"name":"search","arguments":{}}}]},"done":true}`,
		}
		for _, event := range events {
			_, err := io.WriteString(w, event+"\n")
			require.NoError(t, err)
		}
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)
	parallelToolCalls := false

	response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model", ParallelToolCalls: &parallelToolCalls})

	require.Error(t, err)
	assert.Nil(t, response)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
}

func TestOllamaBackend_GenerateTimeoutDuringStreamReturnsBackendTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		_, err := io.WriteString(w, `{"model":"test-model","message":{"role":"assistant","content":"partial"},"done":false}`+"\n")
		require.NoError(t, err)
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	response, err := backend.Generate(ctx, models.GenerateRequest{Model: "test-model"})

	require.Error(t, err)
	assert.Nil(t, response)
	assert.ErrorIs(t, err, constants.ErrInferenceBackendTimeout)
}

func TestOllamaBackend_GenerateAcceptsToolOnlyResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ollamaChatResponse{
			Model: "test-model",
			Message: ollamaChatMessage{Role: "assistant", ToolCalls: []ollamaToolCall{{
				ID: "call-1",
				Function: ollamaToolFunction{
					Name:      "inspect",
					Arguments: json.RawMessage(`{"path":"target.txt"}`),
				},
			}}},
			Done:       true,
			DoneReason: "tool_calls",
		}
		require.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)

	response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model"})

	require.NoError(t, err)
	require.Len(t, response.Parts, 1)
	assert.Empty(t, response.Parts[0].GetText())
	assert.Equal(t, "inspect", response.Parts[0].GetToolCall().GetName())
	assert.Equal(t, "tool_calls", response.FinishReason)
}

func TestOllamaBackend_GenerateRejectsInvalidToolCallsAndEmptyOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		message      ollamaChatMessage
		responseBody []byte
	}{
		{name: "malformed arguments", responseBody: []byte(`{"model":"test-model","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"inspect","arguments":"{"}}]},"done":true}`)},
		{name: "duplicate argument key", message: ollamaChatMessage{Role: "assistant", ToolCalls: []ollamaToolCall{{Function: ollamaToolFunction{Name: "inspect", Arguments: json.RawMessage(`{"path":"a","path":"b"}`)}}}}},
		{name: "non-object arguments", message: ollamaChatMessage{Role: "assistant", ToolCalls: []ollamaToolCall{{Function: ollamaToolFunction{Name: "inspect", Arguments: json.RawMessage(`[]`)}}}}},
		{name: "missing tool name", message: ollamaChatMessage{Role: "assistant", ToolCalls: []ollamaToolCall{{Function: ollamaToolFunction{Arguments: json.RawMessage(`{}`)}}}}},
		{name: "empty output", message: ollamaChatMessage{Role: "assistant"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.responseBody != nil {
					_, err := w.Write(tt.responseBody)
					require.NoError(t, err)
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(ollamaChatResponse{Model: "test-model", Message: tt.message, Done: true}))
			}))
			defer server.Close()
			backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
			require.NoError(t, err)

			response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model"})

			require.Error(t, err)
			assert.Nil(t, response)
			assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
		})
	}
}

func TestOllamaBackend_GenerateFiltersToolsUsingAutoChoice(t *testing.T) {
	t.Parallel()
	var capturedBody ollamaChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		require.NoError(t, json.NewEncoder(w).Encode(ollamaChatResponse{Model: "test-model", Message: ollamaChatMessage{Role: "assistant", Content: "ok"}, Done: true}))
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)

	_, err = backend.Generate(context.Background(), models.GenerateRequest{
		Model: "test-model",
		Tools: []*operatorv1.InferenceToolDeclaration{
			{Name: "inspect", JsonSchema: `{"type":"object"}`},
			{Name: "search", JsonSchema: `{"type":"object"}`},
		},
		ToolChoice: &operatorv1.InferenceToolChoice{
			Mode:             operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO,
			AllowedToolNames: []string{"inspect"},
		},
	})

	require.NoError(t, err)
	require.Len(t, capturedBody.Tools, 1)
	assert.Equal(t, "inspect", capturedBody.Tools[0].Function.Name)
}

func TestOllamaBackend_GenerateEmptyAutoChoiceAllowsAllDeclaredTools(t *testing.T) {
	t.Parallel()
	var capturedBody ollamaChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		require.NoError(t, json.NewEncoder(w).Encode(ollamaChatResponse{Model: "test-model", Message: ollamaChatMessage{Role: "assistant", Content: "ok"}, Done: true}))
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)

	_, err = backend.Generate(context.Background(), models.GenerateRequest{
		Model: "test-model",
		Tools: []*operatorv1.InferenceToolDeclaration{
			{Name: "inspect", JsonSchema: `{"type":"object"}`},
			{Name: "search", JsonSchema: `{"type":"object"}`},
		},
		ToolChoice: &operatorv1.InferenceToolChoice{Mode: operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO},
	})

	require.NoError(t, err)
	assert.Len(t, capturedBody.Tools, 2)
}

func TestOllamaBackend_GenerateRejectsParallelToolCallsWhenDisabled(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(ollamaChatResponse{
			Model: "test-model",
			Message: ollamaChatMessage{Role: "assistant", ToolCalls: []ollamaToolCall{
				{Function: ollamaToolFunction{Name: "inspect", Arguments: json.RawMessage(`{}`)}},
				{Function: ollamaToolFunction{Name: "search", Arguments: json.RawMessage(`{}`)}},
			}},
			Done: true,
		}))
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)
	parallelToolCalls := false

	response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model", ParallelToolCalls: &parallelToolCalls})

	require.Error(t, err)
	assert.Nil(t, response)
	assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
}

func TestOllamaBackend_GenerateAllowsParallelToolCallsWhenEnabledOrUnspecified(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		parallel *bool
	}{
		{name: "enabled", parallel: func() *bool { value := true; return &value }()},
		{name: "unspecified"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				require.NoError(t, json.NewEncoder(w).Encode(ollamaChatResponse{
					Model: "test-model",
					Message: ollamaChatMessage{Role: "assistant", ToolCalls: []ollamaToolCall{
						{Function: ollamaToolFunction{Name: "inspect", Arguments: json.RawMessage(`{}`)}},
						{Function: ollamaToolFunction{Name: "search", Arguments: json.RawMessage(`{}`)}},
					}},
					Done: true,
				}))
			}))
			defer server.Close()
			backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
			require.NoError(t, err)

			response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model", ParallelToolCalls: tt.parallel})

			require.NoError(t, err)
			assert.Len(t, response.Parts, 2)
		})
	}
}

func TestOllamaBackend_GenerateRejectsUnsupportedToolChoiceAndThinkingLevel(t *testing.T) {
	t.Parallel()
	backend, err := NewOllamaBackend("http://127.0.0.1:1", testutil.NewTestLogger())
	require.NoError(t, err)
	tests := []struct {
		name string
		req  models.GenerateRequest
	}{
		{
			name: "required tool choice",
			req: models.GenerateRequest{Model: "test-model", ToolChoice: &operatorv1.InferenceToolChoice{
				Mode: operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_REQUIRED,
			}},
		},
		{
			name: "thinking level",
			req: models.GenerateRequest{Model: "test-model", Thinking: &operatorv1.InferenceThinkingControl{
				Mode: &operatorv1.InferenceThinkingControl_Level{Level: "high"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := backend.Generate(context.Background(), tt.req)

			require.Error(t, err)
			assert.Nil(t, response)
			assert.ErrorIs(t, err, constants.ErrInferenceCapabilityUnsupported)
		})
	}
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
				Model:      modelsList[i],
				Message:    ollamaChatMessage{Role: "assistant", Content: "ok"},
				Done:       true,
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

func TestOllamaBackend_GenerateToolsUnsupportedReturnsErrInferenceCapabilityUnsupported(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"registry.ollama.ai/sam860/LFM2:350m does not support tools"}`))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)
	resp, err := backend.Generate(context.Background(), models.GenerateRequest{
		Role:  models.InferenceModelRolePrimary,
		Model: "sam860/LFM2:350m",
		Tools: []*operatorv1.InferenceToolDeclaration{{
			Name:        "get_command_constraints",
			Description: "constraints",
			JsonSchema:  `{"type":"object"}`,
		}},
	})

	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceCapabilityUnsupported)
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
		promptTokens := int32(5)
		completionTokens := int32(3)
		resp := ollamaChatResponse{
			Model:           "test-model",
			Message:         ollamaChatMessage{Role: "assistant", Content: "result"},
			Done:            true,
			PromptEvalCount: &promptTokens,
			EvalCount:       &completionTokens,
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
	require.Len(t, resp.Parts, 1)
	assert.Equal(t, "result", resp.Parts[0].GetText())
	assert.Equal(t, "stop", resp.FinishReason, "empty done_reason with done=true should default to stop")
}

func TestOllamaBackend_GenerateDistinguishesUnavailableUsageFromReportedZero(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		responseBody  string
		usageReported bool
	}{
		{
			name:         "unavailable",
			responseBody: `{"model":"test-model","message":{"role":"assistant","content":"ok"},"done":true}`,
		},
		{
			name:          "reported zero",
			responseBody:  `{"model":"test-model","message":{"role":"assistant","content":"ok"},"done":true,"prompt_eval_count":0,"eval_count":0,"load_duration":0}`,
			usageReported: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write([]byte(tt.responseBody))
				require.NoError(t, err)
			}))
			defer server.Close()
			backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
			require.NoError(t, err)

			response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model"})

			require.NoError(t, err)
			assert.Equal(t, tt.usageReported, response.UsageReported)
			assert.Zero(t, response.TotalTokens)
			if tt.usageReported {
				require.NotNil(t, response.LoadDurationNS)
				assert.Zero(t, *response.LoadDurationNS)
				assert.Equal(t, operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_PROVIDER, response.TimingSource)
			} else {
				assert.Nil(t, response.LoadDurationNS)
				assert.Equal(t, operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_PROVIDER, response.TimingSource)
			}
			require.NotNil(t, response.TimeToFirstTokenNS)
			assert.GreaterOrEqual(t, *response.TimeToFirstTokenNS, int64(0))
		})
	}
}

func TestOllamaBackend_GenerateRejectsContradictoryOrInvalidMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		responseBody string
	}{
		{name: "missing served model", responseBody: `{"message":{"role":"assistant","content":"ok"},"done":true}`},
		{name: "partial usage", responseBody: `{"model":"test-model","message":{"role":"assistant","content":"ok"},"done":true,"prompt_eval_count":1}`},
		{name: "negative usage", responseBody: `{"model":"test-model","message":{"role":"assistant","content":"ok"},"done":true,"prompt_eval_count":-1,"eval_count":1}`},
		{name: "negative timing", responseBody: `{"model":"test-model","message":{"role":"assistant","content":"ok"},"done":true,"load_duration":-1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write([]byte(tt.responseBody))
				require.NoError(t, err)
			}))
			defer server.Close()
			backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
			require.NoError(t, err)

			response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model"})

			require.Error(t, err)
			assert.Nil(t, response)
			assert.ErrorIs(t, err, constants.ErrInferenceProviderResponseInvalid)
		})
	}
}

func TestOllamaBackend_GenerateVerifiesFrozenModelDigestBeforeAndAfterProviderCall(t *testing.T) {
	t.Parallel()
	modelDigest := strings.Repeat("a", 64)
	tagsCalls := 0
	var providerRequest []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			tagsCalls++
			require.NoError(t, json.NewEncoder(w).Encode(ollamaTagsResponse{Models: []ollamaTagModel{{Name: "test-model", Digest: modelDigest}}}))
		case "/api/chat":
			var err error
			providerRequest, err = io.ReadAll(r.Body)
			require.NoError(t, err)
			_, err = w.Write([]byte(`{"model":"test-model","message":{"role":"assistant","content":"ok"},"done":true,"done_reason":"stop"}`))
			require.NoError(t, err)
		default:
			t.Fatalf("unexpected provider path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)

	response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model", ModelDigest: modelDigest})

	require.NoError(t, err)
	assert.Equal(t, 2, tagsCalls)
	assert.Equal(t, modelDigest, response.ServedModelDigest)
	assert.Equal(t, models.SHA256Hex(providerRequest), response.NormalizedRequestHash)
	wantOutputHash, err := models.ComputeInferenceOutputHash(response.Parts, response.FinishReason)
	require.NoError(t, err)
	assert.Equal(t, wantOutputHash, response.OutputHash)
}

func TestOllamaBackend_GenerateRejectsModelDigestDriftAfterProviderCall(t *testing.T) {
	t.Parallel()
	requestedDigest := strings.Repeat("a", 64)
	tagsCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			tagsCalls++
			digest := requestedDigest
			if tagsCalls == 2 {
				digest = strings.Repeat("b", 64)
			}
			require.NoError(t, json.NewEncoder(w).Encode(ollamaTagsResponse{Models: []ollamaTagModel{{Name: "test-model", Digest: digest}}}))
		case "/api/chat":
			_, err := w.Write([]byte(`{"model":"test-model","message":{"role":"assistant","content":"ok"},"done":true}`))
			require.NoError(t, err)
		}
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)

	response, err := backend.Generate(context.Background(), models.GenerateRequest{Model: "test-model", ModelDigest: requestedDigest})

	require.Error(t, err)
	assert.Nil(t, response)
	assert.ErrorIs(t, err, constants.ErrInferenceModelDigestMismatch)
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

func TestOllamaBackend_ListModelVariantsNormalizesOllamaDigests(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("c", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/tags", r.URL.Path)
		require.NoError(t, json.NewEncoder(w).Encode(ollamaTagsResponse{
			Models: []ollamaTagModel{{Name: "probe-model", Digest: "sha256:" + digest}},
		}))
	}))
	defer server.Close()
	backend, err := NewOllamaBackend(server.URL, testutil.NewTestLogger())
	require.NoError(t, err)
	variants, err := backend.ListModelVariants(context.Background())
	require.NoError(t, err)
	require.Len(t, variants, 1)
	assert.Equal(t, "probe-model", variants[0].GetModel())
	assert.Equal(t, digest, variants[0].GetDigest())
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

func TestOllamaBackend_GenerateStreamingPublishesProgressEvents(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		encoder := json.NewEncoder(w)
		require.NoError(t, encoder.Encode(ollamaChatResponse{
			Model:   "test-model",
			Message: ollamaChatMessage{Role: "assistant", Content: "hel"},
		}))
		require.NoError(t, encoder.Encode(ollamaChatResponse{
			Model:      "test-model",
			Message:    ollamaChatMessage{Role: "assistant", Content: "lo"},
			Done:       true,
			DoneReason: "stop",
		}))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)

	var progressEvents []*operatorv1.InferenceProgressEvent
	ctx := WithProgressReporter(context.Background(), func(event *operatorv1.InferenceProgressEvent) error {
		progressEvents = append(progressEvents, event)
		return nil
	})

	resp, err := backend.Generate(ctx, models.GenerateRequest{
		Role:              models.InferenceModelRolePrimary,
		Model:             "test-model",
		Stream:            true,
		ProviderAttemptID: "attempt-1",
		Messages: []*operatorv1.InferenceMessage{{
			Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: "hi"}}},
		}},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, progressEvents, 2)
	assert.Equal(t, uint32(1), progressEvents[0].GetSequence())
	assert.Equal(t, "hel", progressEvents[0].GetParts()[0].GetText())
	assert.NotNil(t, progressEvents[0].TimeToFirstTokenNs)
	assert.Equal(t, uint32(2), progressEvents[1].GetSequence())
	assert.Equal(t, "lo", progressEvents[1].GetParts()[0].GetText())
}

func TestOllamaBackend_GenerateStreamingReporterErrorFailsClosed(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(ollamaChatResponse{
			Model:      "test-model",
			Message:    ollamaChatMessage{Role: "assistant", Content: "hello"},
			Done:       true,
			DoneReason: "stop",
		}))
	}))
	defer server.Close()

	backend, err := NewOllamaBackend(server.URL, logger)
	require.NoError(t, err)

	ctx := WithProgressReporter(context.Background(), func(event *operatorv1.InferenceProgressEvent) error {
		return constants.ErrInferenceProgressBackpressure
	})
	resp, err := backend.Generate(ctx, models.GenerateRequest{
		Role:              models.InferenceModelRolePrimary,
		Model:             "test-model",
		Stream:            true,
		ProviderAttemptID: "attempt-1",
		Messages: []*operatorv1.InferenceMessage{{
			Role:  operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
			Parts: []*operatorv1.InferenceMessagePart{{Part: &operatorv1.InferenceMessagePart_Text{Text: "hi"}}},
		}},
	})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, constants.ErrInferenceProgressBackpressure)
}
