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

	"github.com/g8e-ai/g8e/internal/constants"
)

// EnsembleChatRequest is the typed body for POST /api/v1/chat against the
// ensemble (g8ee). It mirrors the Python ChatMessageRequest shape: a
// RequestContext, the user message, optional resource_creation, and LLM
// overrides. The harness fills the context from the persona and GovKit so
// scenarios do not construct it by hand.
type EnsembleChatRequest struct {
	Context            EnsembleRequestContext    `json:"context"`
	Message            string                    `json:"message"`
	SentinelMode       bool                      `json:"sentinel_mode"`
	ResourceCreation   *EnsembleResourceCreation `json:"resource_creation,omitempty"`
	LLMPrimaryProvider string                    `json:"llm_primary_provider,omitempty"`
	LLMPrimaryModel    string                    `json:"llm_primary_model,omitempty"`
	LLMPrimaryEndpoint string                    `json:"llm_primary_endpoint,omitempty"`
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
	WebSessionID    string                  `json:"web_session_id,omitempty"`
	CLISessionID    string                  `json:"cli_session_id,omitempty"`
	UserID          string                  `json:"user_id,omitempty"`
	OrganizationID  string                  `json:"organization_id,omitempty"`
	CaseID          string                  `json:"case_id,omitempty"`
	InvestigationID string                  `json:"investigation_id,omitempty"`
	BoundOperators  []EnsembleBoundOperator `json:"bound_operators,omitempty"`
	SourceComponent string                  `json:"source_component"`
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
	if p.UserAgent != "" {
		httpReq.Header.Set("User-Agent", p.UserAgent)
		httpReq.Header.Set("X-G8E-Client-Persona", p.ID)
	}
	if p.UserID != "" {
		httpReq.Header.Set(HeaderProxyUserID, p.UserID)
		// The ensemble auth service (auth_service.py:59) requires BOTH
		// X-Proxy-User-Id AND X-Proxy-User-Email with AND logic for proxy
		// auth — without the email header it falls through to Bearer token
		// auth, which the harness does not send for ensemble calls, and
		// returns 401 G8E-1200. The harness has no real email (headless
		// enrollment produces no browser-registered email), so derive a
		// synthetic one from the user id. The ensemble only stores the
		// email on the AuthenticatedUser record; it does not validate the
		// email against the gateway, so a synthetic value is safe and
		// matches the headless enrollment pattern.
		httpReq.Header.Set(HeaderProxyUserEmail, p.UserID+ProxyUserEmailSyntheticDomain)
	}
	if p.CLISessionID != "" {
		httpReq.Header.Set(HeaderProxyCLISessionID, p.CLISessionID)
	}

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
