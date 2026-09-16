// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// InferenceAcceptanceCaseID names one Phase 1A non-scored vertical gate.
type InferenceAcceptanceCaseID string

const (
	InferenceAcceptanceCaseUnaryBasic         InferenceAcceptanceCaseID = "unary-basic"
	InferenceAcceptanceCaseRolePrimary        InferenceAcceptanceCaseID = "role-primary"
	InferenceAcceptanceCaseRoleAssistant      InferenceAcceptanceCaseID = "role-assistant"
	InferenceAcceptanceCaseRoleLite           InferenceAcceptanceCaseID = "role-lite"
	InferenceAcceptanceCaseDeterministicSeed  InferenceAcceptanceCaseID = "deterministic-seed"
	InferenceAcceptanceCaseStreamingProgress  InferenceAcceptanceCaseID = "streaming-progress"
	InferenceAcceptanceCaseStructuredJSON     InferenceAcceptanceCaseID = "structured-json"
	InferenceAcceptanceCaseToolSelection      InferenceAcceptanceCaseID = "tool-selection"
	InferenceAcceptanceCaseToolContinuation   InferenceAcceptanceCaseID = "tool-continuation"
)

// InferenceAcceptanceCase describes one Phase 1A vertical acceptance probe.
type InferenceAcceptanceCase struct {
	ID     InferenceAcceptanceCaseID
	Stream bool
	Apply  func(base InferenceProbeRequest) InferenceProbeRequest
}

// DefaultInferenceAcceptanceCases returns the Phase 1A inference-only vertical matrix.
// Chat-path cases such as homogeneous role control and evaluation trace import remain
// separate g8ee acceptance gates.
func DefaultInferenceAcceptanceCases() []InferenceAcceptanceCase {
	return []InferenceAcceptanceCase{
		{
			ID: InferenceAcceptanceCaseUnaryBasic,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				base.Prompt = "Reply with exactly: probe-ok"
				return base
			},
		},
		{
			ID: InferenceAcceptanceCaseRolePrimary,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				base.Role = models.InferenceModelRolePrimary
				base.Prompt = "Reply with exactly: role-primary-ok"
				return base
			},
		},
		{
			ID: InferenceAcceptanceCaseRoleAssistant,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				base.Role = models.InferenceModelRoleAssistant
				base.Prompt = "Reply with exactly: role-assistant-ok"
				return base
			},
		},
		{
			ID: InferenceAcceptanceCaseRoleLite,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				base.Role = models.InferenceModelRoleLite
				base.Prompt = "Reply with exactly: role-lite-ok"
				return base
			},
		},
		{
			ID: InferenceAcceptanceCaseDeterministicSeed,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				seed := int32(42)
				base.Seed = &seed
				base.Prompt = "Reply with exactly: seed-ok"
				return base
			},
		},
		{
			ID:     InferenceAcceptanceCaseStreamingProgress,
			Stream: true,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				base.Stream = true
				base.Prompt = "Reply with exactly: stream-ok"
				return base
			},
		},
		{
			ID: InferenceAcceptanceCaseStructuredJSON,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				base.Messages = []*operatorv1.InferenceMessage{
					{
						Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_SYSTEM,
						Parts: []*operatorv1.InferenceMessagePart{{
							Part: &operatorv1.InferenceMessagePart_Text{
								Text: "Respond with JSON only. The answer field must be exactly the string structured-ok.",
							},
						}},
					},
					{
						Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
						Parts: []*operatorv1.InferenceMessagePart{{
							Part: &operatorv1.InferenceMessagePart_Text{
								Text: "Return JSON matching the schema with {\"answer\":\"structured-ok\"}. No markdown and no explanation.",
							},
						}},
					},
				}
				base.ResponseFormat = ProbeStructuredResponseFormat()
				return base
			},
		},
		{
			ID: InferenceAcceptanceCaseToolSelection,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				base.Prompt = "Call probe_echo with message exactly tool-ok. Do not answer in plain text."
				base.Tools = []*operatorv1.InferenceToolDeclaration{ProbeEchoToolDeclaration()}
				base.ToolChoice = &operatorv1.InferenceToolChoice{
					Mode:             operatorv1.InferenceToolChoiceMode_INFERENCE_TOOL_CHOICE_MODE_AUTO,
					AllowedToolNames: []string{"probe_echo"},
				}
				return base
			},
		},
		{
			ID: InferenceAcceptanceCaseToolContinuation,
			Apply: func(base InferenceProbeRequest) InferenceProbeRequest {
				base.Messages = []*operatorv1.InferenceMessage{
					{
						Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
						Parts: []*operatorv1.InferenceMessagePart{{
							Part: &operatorv1.InferenceMessagePart_Text{
								Text: "The probe_echo tool already returned echo hello. Reply with exactly: continuation-ok",
							},
						}},
					},
					{
						Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_ASSISTANT,
						Parts: []*operatorv1.InferenceMessagePart{{
							Part: &operatorv1.InferenceMessagePart_ToolCall{
								ToolCall: &operatorv1.InferenceToolCall{
									CallId:         "probe-call-1",
									Name:           "probe_echo",
									ArgumentsJson:  `{"message":"hello"}`,
								},
							},
						}},
					},
					{
						Role: operatorv1.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_TOOL,
						Parts: []*operatorv1.InferenceMessagePart{{
							Part: &operatorv1.InferenceMessagePart_ToolResult{
								ToolResult: &operatorv1.InferenceToolResult{
									CallId:     "probe-call-1",
									Name:       "probe_echo",
									ResultJson: `{"echo":"hello"}`,
								},
							},
						}},
					},
				}
				base.Tools = []*operatorv1.InferenceToolDeclaration{ProbeEchoToolDeclaration()}
				return base
			},
		},
	}
}

// LookupInferenceAcceptanceCase returns one case by ID.
func LookupInferenceAcceptanceCase(id InferenceAcceptanceCaseID) (InferenceAcceptanceCase, error) {
	for _, candidate := range DefaultInferenceAcceptanceCases() {
		if candidate.ID == id {
			return candidate, nil
		}
	}
	return InferenceAcceptanceCase{}, fmt.Errorf("evaluation: unknown inference acceptance case %q", id)
}

// SelectInferenceAcceptanceCases resolves the requested case IDs. An empty slice
// selects the full default matrix.
func SelectInferenceAcceptanceCases(ids []InferenceAcceptanceCaseID) ([]InferenceAcceptanceCase, error) {
	if len(ids) == 0 {
		return DefaultInferenceAcceptanceCases(), nil
	}
	selected := make([]InferenceAcceptanceCase, 0, len(ids))
	for _, id := range ids {
		candidate, err := LookupInferenceAcceptanceCase(id)
		if err != nil {
			return nil, err
		}
		selected = append(selected, candidate)
	}
	return selected, nil
}

// ValidateInferenceAcceptanceCase applies case-specific success checks after the
// shared governed probe validation passes.
func ValidateInferenceAcceptanceCase(caseID InferenceAcceptanceCaseID, resp *operatorv1.InferenceDispatchResponse) error {
	result := resp.GetResult()
	switch caseID {
	case InferenceAcceptanceCaseUnaryBasic:
		return requireResponseTextContains(result, "probe-ok")
	case InferenceAcceptanceCaseRolePrimary:
		return requireResponseTextContains(result, "role-primary-ok")
	case InferenceAcceptanceCaseRoleAssistant:
		return requireResponseTextContains(result, "role-assistant-ok")
	case InferenceAcceptanceCaseRoleLite:
		return requireResponseTextContains(result, "role-lite-ok")
	case InferenceAcceptanceCaseDeterministicSeed:
		return requireResponseTextContains(result, "seed-ok")
	case InferenceAcceptanceCaseStreamingProgress:
		return requireResponseTextContains(result, "stream-ok")
	case InferenceAcceptanceCaseStructuredJSON:
		return validateStructuredAnswer(result, "structured-ok")
	case InferenceAcceptanceCaseToolSelection:
		return requireToolCallNamed(result, "probe_echo")
	case InferenceAcceptanceCaseToolContinuation:
		return requireResponseTextContains(result, "continuation-ok")
	default:
		return fmt.Errorf("evaluation: validate inference acceptance case: unknown case %q", caseID)
	}
}

func requireResponseTextContains(result *operatorv1.InferenceResult, want string) error {
	text := strings.ToLower(collectResponseText(result))
	if !strings.Contains(text, strings.ToLower(want)) {
		return fmt.Errorf("evaluation: acceptance response missing %q: %w", want, constants.ErrInferenceProviderResponseInvalid)
	}
	return nil
}

func collectResponseText(result *operatorv1.InferenceResult) string {
	var builder strings.Builder
	for _, part := range result.GetParts() {
		if part.GetText() != "" {
			builder.WriteString(part.GetText())
		}
	}
	return builder.String()
}

var structuredJSONFencePattern = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*\\})\\s*```")

func validateStructuredAnswer(result *operatorv1.InferenceResult, want string) error {
	text := extractStructuredJSONPayload(collectResponseText(result))
	if text == "" {
		return fmt.Errorf("evaluation: structured acceptance missing JSON text: %w", constants.ErrInferenceProviderResponseInvalid)
	}
	var payload struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return fmt.Errorf("evaluation: structured acceptance invalid JSON: %w", constants.ErrInferenceProviderResponseInvalid)
	}
	if payload.Answer != want {
		return fmt.Errorf("evaluation: structured acceptance answer %q, want %q: %w", payload.Answer, want, constants.ErrInferenceProviderResponseInvalid)
	}
	return nil
}

func extractStructuredJSONPayload(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if match := structuredJSONFencePattern.FindStringSubmatch(text); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(text[start : end+1])
	}
	return text
}

func requireToolCallNamed(result *operatorv1.InferenceResult, name string) error {
	for _, part := range result.GetParts() {
		if part.GetToolCall() != nil && part.GetToolCall().GetName() == name {
			return nil
		}
	}
	return fmt.Errorf("evaluation: acceptance missing tool call %q: %w", name, constants.ErrInferenceProviderResponseInvalid)
}
