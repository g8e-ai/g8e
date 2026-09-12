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
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// ProviderRequestTimeout is the single request deadline authority for calls
// to the remote inference provider. The gateway's inference dispatch deadline
// (dispatch.RequestDeadline) is derived from it so the dispatch wait always
// outlives an in-flight provider call; a dispatch that outlives its deadline
// reports an unknown remote outcome rather than a clean timeout.
const ProviderRequestTimeout = 5 * time.Minute

// ProviderStatusTimeout bounds the read-only startup readiness check against
// the remote provider. It is deliberately shorter than
// ProviderRequestTimeout: a /api/tags round trip is cheap, and an
// unreachable provider must fail operator startup promptly rather than
// after the generation deadline.
const ProviderStatusTimeout = 30 * time.Second

// defaultMaxResponseBytes bounds every response body read from the remote
// provider. A non-streaming chat response contains the full generated text;
// the bound is generous enough for any configured num_predict while still
// failing closed on a misbehaving or hostile endpoint.
const defaultMaxResponseBytes int64 = 32 << 20

// maxErrorBodyBytes bounds the error-body drain on non-OK responses. The
// body is drained only to allow connection reuse; provider error text is
// never included in returned errors or logs because it can echo prompt
// material or carry provider internals.
const maxErrorBodyBytes int64 = 4 << 10

// OllamaBackend implements Backend as an HTTP client to Ollama's /api/chat
// endpoint. It constructs the Ollama chat request from GenerateRequest,
// sends it over HTTP to the configured remote Ollama endpoint, and parses
// the response. It uses context.Context for cancellation and timeout. No
// process management, no goroutine supervision, no health-check loop —
// Ollama is a remote daemon that the deployment owns and monitors.
type OllamaBackend struct {
	base             *url.URL
	client           *http.Client
	maxResponseBytes int64
	logger           *slog.Logger
}

// NewOllamaBackend constructs an OllamaBackend for the given provider
// endpoint. The endpoint is parsed and validated once: it must be an
// absolute http or https URL with a host. A path prefix is preserved, so
// endpoints behind a reverse-proxied prefix such as
// http://host/ollama resolve to http://host/ollama/api/chat.
func NewOllamaBackend(endpoint string, logger *slog.Logger) (*OllamaBackend, error) {
	base, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: endpoint: %w: %w", constants.ErrInferenceEndpointInvalid, err)
	}
	if (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("ollama_backend: endpoint %q: %w", endpoint, constants.ErrInferenceEndpointInvalid)
	}
	return &OllamaBackend{
		base: base,
		client: &http.Client{
			Timeout: ProviderRequestTimeout,
		},
		maxResponseBytes: defaultMaxResponseBytes,
		logger:           logger,
	}, nil
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.apiURL("api", "chat"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, transportError("generate", ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain a bounded slice of the error body for connection reuse.
		// Provider error text is never returned or logged: it can echo
		// prompt material and provider internals.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))
		switch resp.StatusCode {
		case http.StatusRequestTimeout:
			return nil, fmt.Errorf("ollama_backend: generate: %w", constants.ErrInferenceBackendTimeout)
		case http.StatusNotFound:
			return nil, fmt.Errorf("ollama_backend: generate: %w: model %q", constants.ErrInferenceModelNotFound, req.Model)
		default:
			return nil, fmt.Errorf("ollama_backend: generate: %w: status %d", constants.ErrInferenceGenerateFailed, resp.StatusCode)
		}
	}

	var chatResp ollamaChatResponse
	if err := b.decodeResponse("generate", resp.Body, &chatResp); err != nil {
		return nil, err
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, b.apiURL("api", "tags"), nil)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: status: build request: %w", err)
	}

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, transportError("status", ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))
		return nil, fmt.Errorf("ollama_backend: status: %w: status %d", constants.ErrInferenceBackendUnavailable, resp.StatusCode)
	}

	var tagsResp ollamaTagsResponse
	if err := b.decodeResponse("status", resp.Body, &tagsResp); err != nil {
		return nil, err
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

// apiURL resolves a provider API path against the validated base endpoint.
func (b *OllamaBackend) apiURL(elem ...string) string {
	return b.base.JoinPath(elem...).String()
}

// decodeResponse reads a bounded provider response body and unmarshals it.
// Oversized or malformed bodies fail closed with
// ErrInferenceProviderResponseInvalid; the raw body text is never included
// in the error.
func (b *OllamaBackend) decodeResponse(op string, body io.Reader, out any) error {
	data, err := io.ReadAll(io.LimitReader(body, b.maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("ollama_backend: %s: %w: %w", op, constants.ErrInferenceProviderResponseInvalid, err)
	}
	if int64(len(data)) > b.maxResponseBytes {
		return fmt.Errorf("ollama_backend: %s: %w: response exceeds %d bytes", op, constants.ErrInferenceProviderResponseInvalid, b.maxResponseBytes)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("ollama_backend: %s: %w: %w", op, constants.ErrInferenceProviderResponseInvalid, err)
	}
	return nil
}

// transportError classifies a failed provider call. Caller cancellation
// stays distinguishable as context.Canceled; a context or client deadline
// is ErrInferenceBackendTimeout; anything else is
// ErrInferenceBackendUnavailable with the underlying transport cause
// preserved in the chain.
func transportError(op string, ctx context.Context, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		return fmt.Errorf("ollama_backend: %s: %w", op, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("ollama_backend: %s: %w", op, constants.ErrInferenceBackendTimeout)
	default:
		return fmt.Errorf("ollama_backend: %s: %w: %w", op, constants.ErrInferenceBackendUnavailable, err)
	}
}
