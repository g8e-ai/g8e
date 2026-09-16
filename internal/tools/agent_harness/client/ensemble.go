// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// EnsembleChatRequest is the typed body for POST /api/v1/chat against the
// ensemble (g8ee). It mirrors the Python ChatMessageRequest shape: a
// RequestContext, the user message, optional resource_creation, and LLM
// overrides. The harness fills the context from the persona and GovKit so
// scenarios do not construct it by hand.
// EnsembleModelVariant mirrors the frozen campaign model registry entry carried
// through production chat evaluation context.
type EnsembleModelVariant struct {
	Model  string `json:"model"`
	Digest string `json:"digest"`
}

// EnsembleEvaluationContext mirrors the Python EvaluationInferenceContext
// attached to POST /api/v1/chat for scored evaluation assignments.
type EnsembleEvaluationContext struct {
	CampaignID              string                 `json:"campaign_id"`
	RunID                   string                 `json:"run_id"`
	AssignmentID            string                 `json:"assignment_id"`
	EvaluationAttemptID     string                 `json:"evaluation_attempt_id"`
	ScenarioID              string                 `json:"scenario_id"`
	ModelRegistryDigest     string                 `json:"model_registry_digest"`
	ModelRegistry           []EnsembleModelVariant `json:"model_registry"`
	TargetOperatorSessionID string                 `json:"target_operator_session_id"`
	EvaluationLane          string                 `json:"evaluation_lane,omitempty"`
	DesignatedModelRole     string                         `json:"designated_model_role,omitempty"`
	GradingMethod           string                         `json:"grading_method,omitempty"`
	GoldSummary             *EnsembleEvaluationGoldSummary `json:"gold_summary,omitempty"`
}

// EnsembleEvaluationGoldSummary mirrors the private gold summary carried with
// scored semantic-judge campaign assignments.
type EnsembleEvaluationGoldSummary struct {
	UserPrompt       string   `json:"user_prompt"`
	ExpectedBehavior string   `json:"expected_behavior"`
	RequiredConcepts []string `json:"required_concepts"`
	ExpectedTools    []string `json:"expected_tools"`
	ForbiddenTools   []string `json:"forbidden_tools"`
}

type EnsembleChatRequest struct {
	Context              EnsembleRequestContext     `json:"context"`
	EvaluationContext    *EnsembleEvaluationContext `json:"evaluation_context,omitempty"`
	Message              string                     `json:"message"`
	SentinelMode         bool                       `json:"sentinel_mode"`
	ResourceCreation     *EnsembleResourceCreation  `json:"resource_creation,omitempty"`
	LLMPrimaryProvider   string                     `json:"llm_primary_provider,omitempty"`
	LLMPrimaryModel      string                     `json:"llm_primary_model,omitempty"`
	LLMPrimaryEndpoint   string                     `json:"llm_primary_endpoint,omitempty"`
	LLMAssistantProvider string                     `json:"llm_assistant_provider,omitempty"`
	LLMAssistantModel    string                     `json:"llm_assistant_model,omitempty"`
	LLMAssistantEndpoint string                     `json:"llm_assistant_endpoint,omitempty"`
	LLMLiteProvider      string                     `json:"llm_lite_provider,omitempty"`
	LLMLiteModel         string                     `json:"llm_lite_model,omitempty"`
	LLMLiteEndpoint      string                     `json:"llm_lite_endpoint,omitempty"`
}

// EnsembleBoundOperator mirrors the Python BoundOperator model in the
// RequestContext. The harness passes bound operators explicitly as an intent
// signal scoping which operators the AI can act through.
type EnsembleBoundOperator struct {
	OperatorID        string `json:"operator_id"`
	OperatorSessionID string `json:"operator_session_id,omitempty"`
	Status            string `json:"status,omitempty"`
}

// EnsembleRequestContext is the typed RequestContext the ensemble expects in
// the chat body. source_component is always "CLIENT" for harness-driven
// requests; the validator requires user_id and either web_session_id or
// cli_session_id for that source.
type EnsembleRequestContext struct {
	WebSessionID      string                  `json:"web_session_id,omitempty"`
	CLISessionID      string                  `json:"cli_session_id,omitempty"`
	UserID            string                  `json:"user_id,omitempty"`
	OrganizationID    string                  `json:"organization_id,omitempty"`
	CaseID            string                  `json:"case_id,omitempty"`
	InvestigationID   string                  `json:"investigation_id,omitempty"`
	OperatorID        string                  `json:"operator_id,omitempty"`
	OperatorSessionID string                  `json:"operator_session_id,omitempty"`
	BoundOperators    []EnsembleBoundOperator `json:"bound_operators,omitempty"`
	SourceComponent   string                  `json:"source_component"`
}

// EnsembleResourceCreation controls inline case/investigation creation. When
// CreateCase is true the ensemble creates a new case and investigation before
// firing the chat background task.
type EnsembleResourceCreation struct {
	CreateCase bool   `json:"create_case"`
	CaseTitle  string `json:"case_title,omitempty"`
}

// EnsembleChatResponse mirrors the Python ChatStartedResponse: success plus the
// case_id and investigation_id the ensemble created or reused.
type EnsembleChatResponse struct {
	Success         bool   `json:"success"`
	CaseID          string `json:"case_id"`
	InvestigationID string `json:"investigation_id"`
}

// EnsembleChat sends a POST /api/v1/chat to the ensemble (g8ee) HTTP surface.
// The ensemble is a Python/FastAPI app on its own port; the harness dials it
// directly (no mTLS — the ensemble is behind a reverse proxy in production and
// reads identity from X-Proxy-User-Id / X-Proxy-User-Email headers). The
// response is non-streaming: the ensemble creates case/investigation inline
// (when ResourceCreation.CreateCase is set), fires run_chat as a background
// task, and returns immediately with the case/investigation IDs. Scenarios
// poll for side effects (file appearance, audit vault) rather than waiting for
// the AI response.
//
// The persona supplies the proxy user identity (UserID, CLISessionID). Returns
// an error if EnsembleBaseURL is not configured.
func (c *Client) EnsembleChat(ctx context.Context, p Persona, req EnsembleChatRequest) (*EnsembleChatResponse, error) {
	if c.cfg.EnsembleBaseURL == "" {
		return nil, fmt.Errorf("ensemble chat: %w", constants.ErrEnsembleURLNotConfigured)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ensemble chat: marshal request: %w", err)
	}

	// The ensemble reads identity from proxy headers, not mTLS. Use the plain
	// http client (no client cert) so the ensemble's auth dependency extracts
	// the user from X-Proxy-User-Id.
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.EnsembleBaseURL+EnsembleChatPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ensemble chat: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyEnsemblePersonaHeaders(httpReq, p)

	resp, err := c.http.Do(httpReq)
	ex := Exchange{Persona: p.ID, Method: http.MethodPost, URL: c.cfg.EnsembleBaseURL + EnsembleChatPath, At: start}
	attachBody(&ex.ReqBody, &ex.ReqRaw, body)
	if err != nil {
		ex.Err = err.Error()
		ex.LatencyMS = time.Since(start).Milliseconds()
		c.append(ex, c.cfg.Verbose)
		return nil, fmt.Errorf("ensemble chat: execute request: %w", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	ex.Status = resp.StatusCode
	ex.LatencyMS = time.Since(start).Milliseconds()
	attachBody(&ex.RespBody, &ex.RespRaw, out)
	c.append(ex, c.cfg.Verbose)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ensemble chat: status %d: %s", resp.StatusCode, truncateResp(out))
	}

	var chatResp EnsembleChatResponse
	if err := json.Unmarshal(out, &chatResp); err != nil {
		return nil, fmt.Errorf("ensemble chat: decode response: %w", err)
	}
	return &chatResp, nil
}

// EnsembleChatPath is the canonical ensemble chat endpoint, matching
// InternalAPIPaths.G8EE_CHAT in the ensemble (api_paths.json: g8ee.chat under
// the /api/v1 prefix).
const EnsembleChatPath = "/api/v1/chat"

// EnsembleEvaluationTracePath is the authenticated read-only evaluation trace
// lookup exposed by g8ee after chat completion.
const EnsembleEvaluationTracePath = "/api/v1/evaluation/trace/%s/%s"

// EnsembleEvaluationTraceResponse mirrors the Python EvaluationTraceResponse.
type EnsembleEvaluationTraceResponse struct {
	Trace map[string]any `json:"trace"`
}

// GetEvaluationTrace loads one persisted evaluation assignment trace from g8ee.
func (c *Client) GetEvaluationTrace(ctx context.Context, p Persona, assignmentID, evaluationAttemptID string) (map[string]any, error) {
	if c.cfg.EnsembleBaseURL == "" {
		return nil, fmt.Errorf("ensemble evaluation trace: %w", constants.ErrEnsembleURLNotConfigured)
	}
	if assignmentID == "" || evaluationAttemptID == "" {
		return nil, fmt.Errorf("ensemble evaluation trace: %w", constants.ErrMissingRequiredField)
	}
	path := fmt.Sprintf(EnsembleEvaluationTracePath, assignmentID, evaluationAttemptID)
	start := time.Now()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.EnsembleBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("ensemble evaluation trace: build request: %w", err)
	}
	applyEnsemblePersonaHeaders(httpReq, p)

	resp, err := c.http.Do(httpReq)
	ex := Exchange{Persona: p.ID, Method: http.MethodGet, URL: c.cfg.EnsembleBaseURL + path, At: start}
	if err != nil {
		ex.Err = err.Error()
		ex.LatencyMS = time.Since(start).Milliseconds()
		c.append(ex, c.cfg.Verbose)
		return nil, fmt.Errorf("ensemble evaluation trace: execute request: %w", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	ex.Status = resp.StatusCode
	ex.LatencyMS = time.Since(start).Milliseconds()
	attachBody(&ex.RespBody, &ex.RespRaw, out)
	c.append(ex, c.cfg.Verbose)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ensemble evaluation trace: status %d: %s", resp.StatusCode, truncateResp(out))
	}
	var traceResp EnsembleEvaluationTraceResponse
	if err := json.Unmarshal(out, &traceResp); err != nil {
		return nil, fmt.Errorf("ensemble evaluation trace: decode response: %w", err)
	}
	if len(traceResp.Trace) == 0 {
		return nil, fmt.Errorf("ensemble evaluation trace: %w", constants.ErrMissingRequiredField)
	}
	return traceResp.Trace, nil
}

// applyEnsemblePersonaHeaders attaches the identity headers g8ee uses to bind
// Bearer operator sessions to a user and CLI session. GET endpoints such as
// evaluation trace lookup have no JSON body, so these headers must be sent even
// when Authorization is present.
func applyEnsemblePersonaHeaders(httpReq *http.Request, p Persona) {
	if p.UserAgent != "" {
		httpReq.Header.Set("User-Agent", p.UserAgent)
		httpReq.Header.Set("X-G8E-Client-Persona", p.ID)
	}
	if p.OperatorSessionID != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.OperatorSessionID)
	}
	if p.UserID != "" {
		httpReq.Header.Set(HeaderProxyUserID, p.UserID)
		httpReq.Header.Set(HeaderProxyUserEmail, p.UserID+ProxyUserEmailSyntheticDomain)
	}
	if p.CLISessionID != "" {
		httpReq.Header.Set(HeaderProxyCLISessionID, p.CLISessionID)
	}
}

// HeaderProxyUserID is the X-Proxy-User-Id header the ensemble auth
// dependency reads as a fallback when no authenticated user is in request
// state. Matches ensemble/app/constants/__init__.py X_PROXY_USER_ID (sourced
// from g8e.constants.PROXY_USER_ID_HEADER).
const HeaderProxyUserID = "X-Proxy-User-Id"

// HeaderProxyCLISessionID is the X-Proxy-CLI-Session-Id header carrying the
// CLI session id into the ensemble context. The ensemble's auth service reads
// this to populate the request context.
const HeaderProxyCLISessionID = "X-Proxy-CLI-Session-Id"

// HeaderProxyUserEmail is the X-Proxy-User-Email header the ensemble auth
// service requires alongside X-Proxy-User-Id for proxy authentication
// (auth_service.py:59 uses AND logic on both headers). Matches
// ensemble/app/constants/__init__.py X_PROXY_USER_EMAIL (sourced from
// g8e.constants.PROXY_USER_EMAIL_HEADER).
const HeaderProxyUserEmail = "X-Proxy-User-Email"

// ProxyUserEmailSyntheticDomain is the domain appended to the user id to form
// a synthetic proxy user email when the harness has no real email (headless
// enrollment produces none). The ensemble does not validate the email against
// the gateway, so a synthetic value is safe.
const ProxyUserEmailSyntheticDomain = "@g8e.local"

// truncateResp returns a shortened representation of body bytes for error
// messages, capped at 512 characters. Shared with client_helpers but kept
// local to avoid an import cycle with the e2e package.
func truncateResp(body []byte) string {
	const max = 512
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...(truncated)"
}
