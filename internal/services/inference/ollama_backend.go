// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// OllamaBackend implements Backend as an HTTP client to Ollama's /api/chat
// endpoint. It constructs the Ollama chat request from GenerateRequest,
// sends it over loopback HTTP to the configured Ollama endpoint, and parses
// the response. It uses context.Context for cancellation and timeout. No
// process management, no goroutine supervision, no health-check loop —
// Ollama is a daemon that the deployment starts and monitors.
type OllamaBackend struct {
	endpoint string
	client   *http.Client
	logger   *slog.Logger
}

// NewOllamaBackend constructs an OllamaBackend for the given loopback
// endpoint. The endpoint is typically http://127.0.0.1:<port>.
func NewOllamaBackend(endpoint string, logger *slog.Logger) *OllamaBackend {
	return &OllamaBackend{
		endpoint: strings.TrimRight(endpoint, "/"),
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
		logger: logger,
	}
}

// ollamaChatRequest is the request body for Ollama's /api/chat endpoint.
type ollamaChatRequest struct {
	Model     string             `json:"model"`
	Messages  []ollamaChatMessage `json:"messages"`
	Stream    bool               `json:"stream"`
	Options   ollamaChatOptions   `json:"options,omitempty"`
	KeepAlive string             `json:"keep_alive,omitempty"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatOptions struct {
	Temperature float32 `json:"temperature,omitempty"`
	NumPredict  int32   `json:"num_predict,omitempty"`
}

// ollamaChatResponse is the response body from Ollama's /api/chat endpoint
// (non-streaming).
type ollamaChatResponse struct {
	Model     string `json:"model"`
	Message   ollamaChatMessage `json:"message"`
	Done      bool   `json:"done"`
	DoneReason string `json:"done_reason"`
	PromptEvalCount int32 `json:"prompt_eval_count"`
	EvalCount       int32 `json:"eval_count"`
}

// ollamaTagsResponse is the response body from Ollama's /api/tags endpoint.
type ollamaTagsResponse struct {
	Models []ollamaTagModel `json:"models"`
}

type ollamaTagModel struct {
	Name string `json:"name"`
}

// Generate sends a generation request to Ollama's /api/chat endpoint and
// returns the generated text, usage metadata, and finish reason.
func (b *OllamaBackend) Generate(ctx context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("ollama_backend: generate: %w", constants.ErrInferenceModelRefInvalid)
	}

	chatReq := ollamaChatRequest{
		Model: req.Model,
		Messages: []ollamaChatMessage{
			{Role: "user", Content: req.Prompt},
		},
		Stream: false,
		Options: ollamaChatOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		},
		KeepAlive: req.KeepAlive,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: marshal request: %w", err)
	}

	url := b.endpoint + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ollama_backend: generate: %w", constants.ErrInferenceBackendTimeout)
		}
		return nil, fmt.Errorf("ollama_backend: generate: %w: %v", constants.ErrInferenceBackendUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestTimeout {
		return nil, fmt.Errorf("ollama_backend: generate: %w", constants.ErrInferenceBackendTimeout)
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama_backend: generate: %w: status %d: %s", constants.ErrInferenceGenerateFailed, resp.StatusCode, string(respBody))
	}

	var chatResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("ollama_backend: decode response: %w", err)
	}

	finishReason := chatResp.DoneReason
	if finishReason == "" && chatResp.Done {
		finishReason = "stop"
	}

	return &models.GenerateResponse{
		Text:             chatResp.Message.Content,
		PromptTokens:     chatResp.PromptEvalCount,
		CompletionTokens: chatResp.EvalCount,
		TotalTokens:      chatResp.PromptEvalCount + chatResp.EvalCount,
		FinishReason:     finishReason,
		Model:            chatResp.Model,
	}, nil
}

// Status queries Ollama's /api/tags endpoint to verify the daemon is
// reachable and lists the models available in its store.
func (b *OllamaBackend) Status(ctx context.Context) (*models.BackendStatus, error) {
	url := b.endpoint + "/api/tags"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: status: build request: %w", err)
	}

	resp, err := b.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ollama_backend: status: %w", constants.ErrInferenceBackendTimeout)
		}
		return nil, fmt.Errorf("ollama_backend: status: %w: %v", constants.ErrInferenceBackendUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama_backend: status: %w: status %d", constants.ErrInferenceBackendUnavailable, resp.StatusCode)
	}

	var tagsResp ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("ollama_backend: status: decode: %w", err)
	}

	modelsList := make([]string, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models {
		modelsList = append(modelsList, m.Name)
	}

	return &models.BackendStatus{
		Available: true,
		Models:    modelsList,
	}, nil
}
