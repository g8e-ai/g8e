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
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
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
	Model     string              `json:"model"`
	Messages  []ollamaChatMessage `json:"messages"`
	Tools     []ollamaTool        `json:"tools,omitempty"`
	Stream    bool                `json:"stream"`
	Think     json.RawMessage     `json:"think,omitempty"`
	Format    json.RawMessage     `json:"format,omitempty"`
	Options   ollamaChatOptions   `json:"options,omitempty"`
	KeepAlive string              `json:"keep_alive,omitempty"`
}

type ollamaChatMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName   string           `json:"tool_name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolCall struct {
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ollamaChatOptions struct {
	Temperature float32  `json:"temperature,omitempty"`
	NumPredict  int32    `json:"num_predict,omitempty"`
	TopP        *float32 `json:"top_p,omitempty"`
	TopK        *int32   `json:"top_k,omitempty"`
	Stop        []string `json:"stop,omitempty"`
	NumCtx      *int32   `json:"num_ctx,omitempty"`
}

// ollamaChatResponse is the response body from Ollama's /api/chat endpoint
// (non-streaming).
type ollamaChatResponse struct {
	Model              string            `json:"model"`
	Message            ollamaChatMessage `json:"message"`
	Done               bool              `json:"done"`
	DoneReason         string            `json:"done_reason"`
	PromptEvalCount    *int32            `json:"prompt_eval_count"`
	EvalCount          *int32            `json:"eval_count"`
	LoadDuration       *int64            `json:"load_duration"`
	PromptEvalDuration *int64            `json:"prompt_eval_duration"`
	EvalDuration       *int64            `json:"eval_duration"`
	TotalDuration      *int64            `json:"total_duration"`
}

// ollamaTagsResponse is the response body from Ollama's /api/tags endpoint.
type ollamaTagsResponse struct {
	Models []ollamaTagModel `json:"models"`
}

type ollamaTagModel struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// Generate sends a generation request to Ollama's /api/chat endpoint and
// returns the generated text, usage metadata, and finish reason.
func (b *OllamaBackend) Generate(ctx context.Context, req models.GenerateRequest) (*models.GenerateResponse, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("ollama_backend: generate: %w", constants.ErrInferenceModelRefInvalid)
	}
	if req.ModelDigest != "" {
		digest, err := b.modelDigest(ctx, req.Model)
		if err != nil {
			return nil, err
		}
		if digest != req.ModelDigest {
			return nil, fmt.Errorf("ollama_backend: generate: %w", constants.ErrInferenceModelDigestMismatch)
		}
	}

	messages, err := inferenceMessagesToOllama(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: generate: messages: %w", err)
	}
	tools, err := inferenceToolsToOllama(req.Tools, req.ToolChoice)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: generate: tools: %w", err)
	}
	var think json.RawMessage
	if req.Thinking != nil {
		switch mode := req.Thinking.GetMode().(type) {
		case *operatorv1.InferenceThinkingControl_Enabled:
			think = json.RawMessage("false")
			if mode.Enabled {
				think = json.RawMessage("true")
			}
		case *operatorv1.InferenceThinkingControl_Level:
			return nil, fmt.Errorf("ollama_backend: generate: thinking level %q: %w", mode.Level, constants.ErrInferenceCapabilityUnsupported)
		default:
			return nil, fmt.Errorf("ollama_backend: generate: thinking: %w", constants.ErrInferenceGenerationOptionsInvalid)
		}
	}
	var format json.RawMessage
	if req.ResponseFormat != nil {
		if req.ResponseFormat.GetMediaType() != inferenceJSONMediaType {
			return nil, fmt.Errorf("ollama_backend: generate: response format: %w", constants.ErrInferenceCapabilityUnsupported)
		}
		if err := validateInferenceToolSchema(req.ResponseFormat.GetJsonSchema()); err != nil {
			return nil, fmt.Errorf("ollama_backend: generate: response format: %w", err)
		}
		format = json.RawMessage(req.ResponseFormat.GetJsonSchema())
	}
	chatReq := ollamaChatRequest{
		Model:    req.Model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
		Think:    think,
		Format:   format,
		Options: ollamaChatOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
			TopP:        req.TopP,
			TopK:        req.TopK,
			Stop:        req.StopSequences,
			NumCtx:      req.ContextLimit,
		},
		KeepAlive: req.KeepAlive,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: marshal request: %w", err)
	}
	normalizedRequestHash := models.SHA256Hex(body)

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
	if chatResp.Model == "" {
		return nil, fmt.Errorf("ollama_backend: generate: served model: %w", constants.ErrInferenceProviderResponseInvalid)
	}
	if (chatResp.PromptEvalCount == nil) != (chatResp.EvalCount == nil) {
		return nil, fmt.Errorf("ollama_backend: generate: token usage: %w", constants.ErrInferenceProviderResponseInvalid)
	}
	promptTokens, completionTokens := int32(0), int32(0)
	usageReported := chatResp.PromptEvalCount != nil
	if usageReported {
		promptTokens = *chatResp.PromptEvalCount
		completionTokens = *chatResp.EvalCount
		if promptTokens < 0 || completionTokens < 0 {
			return nil, fmt.Errorf("ollama_backend: generate: token usage: %w", constants.ErrInferenceProviderResponseInvalid)
		}
	}
	timings := []*int64{chatResp.LoadDuration, chatResp.PromptEvalDuration, chatResp.EvalDuration, chatResp.TotalDuration}
	timingSource := operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_UNSPECIFIED
	for _, duration := range timings {
		if duration == nil {
			continue
		}
		if *duration < 0 {
			return nil, fmt.Errorf("ollama_backend: generate: timing: %w", constants.ErrInferenceProviderResponseInvalid)
		}
		timingSource = operatorv1.InferenceTimingSource_INFERENCE_TIMING_SOURCE_PROVIDER
	}

	allowParallelToolCalls := req.ParallelToolCalls == nil || *req.ParallelToolCalls
	parts, err := ollamaResponseParts(chatResp.Message, allowParallelToolCalls)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: generate: response parts: %w", err)
	}
	outputHash, err := models.ComputeInferenceOutputHash(parts, finishReason)
	if err != nil {
		return nil, fmt.Errorf("ollama_backend: generate: %w", err)
	}
	servedModelDigest := ""
	if req.ModelDigest != "" {
		servedModelDigest, err = b.modelDigest(ctx, chatResp.Model)
		if err != nil {
			return nil, err
		}
		if servedModelDigest != req.ModelDigest {
			return nil, fmt.Errorf("ollama_backend: generate: %w", constants.ErrInferenceModelDigestMismatch)
		}
	}
	return &models.GenerateResponse{
		Parts:                 parts,
		PromptTokens:          promptTokens,
		CompletionTokens:      completionTokens,
		TotalTokens:           promptTokens + completionTokens,
		UsageReported:         usageReported,
		LoadDurationNS:        chatResp.LoadDuration,
		PromptEvalDurationNS:  chatResp.PromptEvalDuration,
		GenerationDurationNS:  chatResp.EvalDuration,
		TotalDurationNS:       chatResp.TotalDuration,
		TimingSource:          timingSource,
		FinishReason:          finishReason,
		Model:                 chatResp.Model,
		ServedModelDigest:     servedModelDigest,
		NormalizedRequestHash: normalizedRequestHash,
		OutputHash:            outputHash,
	}, nil
}

func inferenceMessagesToOllama(messages []*operatorv1.InferenceMessage) ([]ollamaChatMessage, error) {
	result := make([]ollamaChatMessage, 0, len(messages))
	for messageIndex, message := range messages {
		if message == nil || len(message.GetParts()) == 0 {
			return nil, fmt.Errorf("%w: message %d", constants.ErrInferenceMessageInvalid, messageIndex)
		}
		role, err := inferenceMessageRoleToOllama(message.GetRole())
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", messageIndex, err)
		}
		mapped := ollamaChatMessage{Role: role}
		for partIndex, part := range message.GetParts() {
			if part == nil || part.GetPart() == nil {
				return nil, fmt.Errorf("%w: message %d part %d", constants.ErrInferenceMessageInvalid, messageIndex, partIndex)
			}
			switch value := part.GetPart().(type) {
			case *operatorv1.InferenceMessagePart_Text:
				if message.GetRole() == operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL {
					return nil, fmt.Errorf("%w: message %d text part", constants.ErrInferenceMessageInvalid, messageIndex)
				}
				mapped.Content += value.Text
			case *operatorv1.InferenceMessagePart_ToolCall:
				if message.GetRole() != operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT || value.ToolCall == nil || value.ToolCall.GetName() == "" {
					return nil, fmt.Errorf("%w: message %d tool call", constants.ErrInferenceMessageInvalid, messageIndex)
				}
				arguments, err := canonicalizeJSON(value.ToolCall.GetArgumentsJson(), true, nil)
				if err != nil {
					return nil, fmt.Errorf("%w: message %d tool call arguments", constants.ErrInferenceMessageInvalid, messageIndex)
				}
				mapped.ToolCalls = append(mapped.ToolCalls, ollamaToolCall{
					ID:   value.ToolCall.GetCallId(),
					Type: "function",
					Function: ollamaToolFunction{
						Name:      value.ToolCall.GetName(),
						Arguments: json.RawMessage(arguments),
					},
				})
			case *operatorv1.InferenceMessagePart_ToolResult:
				if message.GetRole() != operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL || value.ToolResult == nil || value.ToolResult.GetName() == "" {
					return nil, fmt.Errorf("%w: message %d tool result", constants.ErrInferenceMessageInvalid, messageIndex)
				}
				resultJSON, err := canonicalizeJSON(value.ToolResult.GetResultJson(), false, nil)
				if err != nil {
					return nil, fmt.Errorf("%w: message %d tool result JSON", constants.ErrInferenceMessageInvalid, messageIndex)
				}
				result = append(result, ollamaChatMessage{
					Role:       "tool",
					Content:    resultJSON,
					ToolName:   value.ToolResult.GetName(),
					ToolCallID: value.ToolResult.GetCallId(),
				})
			default:
				return nil, fmt.Errorf("%w: message %d part %d", constants.ErrInferenceMessageInvalid, messageIndex, partIndex)
			}
		}
		if message.GetRole() != operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL {
			result = append(result, mapped)
		}
	}
	return result, nil
}

func inferenceMessageRoleToOllama(role operatorv1.InferenceMessageRole) (string, error) {
	switch role {
	case operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_SYSTEM:
		return "system", nil
	case operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER:
		return "user", nil
	case operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT:
		return "assistant", nil
	case operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL:
		return "tool", nil
	default:
		return "", constants.ErrInferenceMessageInvalid
	}
}

func inferenceToolsToOllama(tools []*operatorv1.InferenceToolDeclaration, choice *operatorv1.InferenceToolChoice) ([]ollamaTool, error) {
	allowed := make(map[string]struct{})
	if choice != nil {
		if choice.GetMode() != operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO {
			return nil, constants.ErrInferenceCapabilityUnsupported
		}
		for _, name := range choice.GetAllowedToolNames() {
			allowed[name] = struct{}{}
		}
	}
	result := make([]ollamaTool, 0, len(tools))
	for toolIndex, tool := range tools {
		if tool == nil || tool.GetName() == "" {
			return nil, fmt.Errorf("%w: tool %d", constants.ErrInferenceToolSchemaInvalid, toolIndex)
		}
		if err := validateInferenceToolSchema(tool.GetJsonSchema()); err != nil {
			return nil, fmt.Errorf("%w: tool %d: %w", constants.ErrInferenceToolSchemaInvalid, toolIndex, err)
		}
		if len(allowed) > 0 {
			if _, ok := allowed[tool.GetName()]; !ok {
				continue
			}
		}
		result = append(result, ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        tool.GetName(),
				Description: tool.GetDescription(),
				Parameters:  json.RawMessage(tool.GetJsonSchema()),
			},
		})
	}
	return result, nil
}

func ollamaResponseParts(message ollamaChatMessage, allowParallelToolCalls bool) ([]*operatorv1.InferenceResponsePart, error) {
	if !allowParallelToolCalls && len(message.ToolCalls) > 1 {
		return nil, constants.ErrInferenceProviderResponseInvalid
	}
	parts := make([]*operatorv1.InferenceResponsePart, 0, len(message.ToolCalls)+1)
	if message.Content != "" {
		parts = append(parts, &operatorv1.InferenceResponsePart{Part: &operatorv1.InferenceResponsePart_Text{Text: message.Content}})
	}
	for callIndex, call := range message.ToolCalls {
		if call.Function.Name == "" || len(call.Function.Arguments) == 0 {
			return nil, fmt.Errorf("%w: tool call %d", constants.ErrInferenceProviderResponseInvalid, callIndex)
		}
		arguments, err := canonicalizeJSON(string(call.Function.Arguments), true, nil)
		if err != nil {
			return nil, fmt.Errorf("%w: tool call %d arguments", constants.ErrInferenceProviderResponseInvalid, callIndex)
		}
		parts = append(parts, &operatorv1.InferenceResponsePart{
			Part: &operatorv1.InferenceResponsePart_ToolCall{ToolCall: &operatorv1.InferenceToolCall{
				CallId:        call.ID,
				Name:          call.Function.Name,
				ArgumentsJson: arguments,
			}},
		})
	}
	if len(parts) == 0 {
		return nil, constants.ErrInferenceProviderResponseInvalid
	}
	return parts, nil
}

func (b *OllamaBackend) modelDigest(ctx context.Context, model string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, b.apiURL("api", "tags"), nil)
	if err != nil {
		return "", fmt.Errorf("ollama_backend: model digest: build request: %w", err)
	}
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return "", transportError("model digest", ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))
		return "", fmt.Errorf("ollama_backend: model digest: %w: status %d", constants.ErrInferenceBackendUnavailable, resp.StatusCode)
	}
	var tagsResp ollamaTagsResponse
	if err := b.decodeResponse("model digest", resp.Body, &tagsResp); err != nil {
		return "", err
	}
	for _, candidate := range tagsResp.Models {
		if candidate.Name != model {
			continue
		}
		if !models.IsSHA256Hex(candidate.Digest) {
			return "", fmt.Errorf("ollama_backend: model digest: %w", constants.ErrInferenceProviderResponseInvalid)
		}
		return candidate.Digest, nil
	}
	return "", fmt.Errorf("ollama_backend: model digest: %w", constants.ErrInferenceModelNotFound)
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
