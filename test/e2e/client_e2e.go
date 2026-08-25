// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/governance"
	"github.com/g8e-ai/g8e/v2/internal/httpclient"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// dispatchRequestJSON is the typed JSON body for POST
// /api/v1/operators/commands. Mirrors gateway.OperatorCommandRequest. Defined
// locally to keep the E2E package decoupled from internal gateway types.
type dispatchRequestJSON struct {
	TargetOperatorSessionID string `json:"target_operator_session_id"`
	ActionType              string `json:"action_type"`
	Payload                 []byte `json:"payload"`
	TargetResource          string `json:"target_resource,omitempty"`
}

// dispatchResponseJSON mirrors gateway.DispatchResponse.
type dispatchResponseJSON struct {
	Success       bool   `json:"success"`
	TransactionID string `json:"transaction_id"`
	EventType     string `json:"event_type,omitempty"`
	ActionType    string `json:"action_type,omitempty"`
	ResultPayload []byte `json:"result_payload,omitempty"`
	Error         string `json:"error,omitempty"`
}

// EnsembleChatRequest is the typed request body for POST /api/v1/chat on the ensemble.
type EnsembleChatRequest struct {
	Context              EnsembleRequestContext    `json:"context"`
	Message              string                    `json:"message"`
	SentinelMode         bool                      `json:"sentinel_mode"`
	ResourceCreation     *EnsembleResourceCreation `json:"resource_creation,omitempty"`
	LLMPrimaryProvider   string                    `json:"llm_primary_provider,omitempty"`
	LLMPrimaryModel      string                    `json:"llm_primary_model,omitempty"`
	LLMPrimaryEndpoint   string                    `json:"llm_primary_endpoint,omitempty"`
	LLMAssistantProvider string                    `json:"llm_assistant_provider,omitempty"`
	LLMAssistantModel    string                    `json:"llm_assistant_model,omitempty"`
	LLMAssistantEndpoint string                    `json:"llm_assistant_endpoint,omitempty"`
	LLMLiteProvider      string                    `json:"llm_lite_provider,omitempty"`
	LLMLiteModel         string                    `json:"llm_lite_model,omitempty"`
	LLMLiteEndpoint      string                    `json:"llm_lite_endpoint,omitempty"`
}

// EnsembleBoundOperator mirrors the bound operator shape in EnsembleRequestContext.
type EnsembleBoundOperator struct {
	OperatorID        string `json:"operator_id"`
	OperatorSessionID string `json:"operator_session_id,omitempty"`
	Status            string `json:"status,omitempty"`
}

// EnsembleRequestContext is the typed context inside EnsembleChatRequest.
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

// EnsembleResourceCreation controls case creation on chat start.
type EnsembleResourceCreation struct {
	CreateCase bool   `json:"create_case"`
	CaseTitle  string `json:"case_title,omitempty"`
}

// EnsembleChatResponse is the response from POST /api/v1/chat.
type EnsembleChatResponse struct {
	Success         bool   `json:"success"`
	CaseID          string `json:"case_id"`
	InvestigationID string `json:"investigation_id"`
	Error           string `json:"error,omitempty"`
}

// DocumentResponse is the wire shape returned by GET /api/v1/data/{collection}/{id}.
type DocumentResponse map[string]json.RawMessage

// GetString extracts a string field from DocumentResponse.
func (d DocumentResponse) GetString(field string) string {
	raw, ok := d[field]
	if !ok {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// GetBool extracts a bool field from DocumentResponse.
func (d DocumentResponse) GetBool(field string) bool {
	raw, ok := d[field]
	if !ok {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// SendChatRequest posts an EnsembleChatRequest to the ensemble /api/v1/chat endpoint
// using proxy auth headers.
func (c *E2EClient) SendChatRequest(ctx context.Context, ensembleURL string, req EnsembleChatRequest) (EnsembleChatResponse, error) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return EnsembleChatResponse{}, fmt.Errorf("marshal chat request: %w", err)
	}

	chatPath := constants.APIPaths.Client["chat"]
	if chatPath == "" {
		chatPath = "/api/v1/chat"
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, ensembleURL+chatPath, bytes.NewReader(bodyBytes))
	if err != nil {
		return EnsembleChatResponse{}, fmt.Errorf("build chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.userID != "" {
		httpReq.Header.Set("X-Proxy-User-Id", c.userID)
		httpReq.Header.Set("X-Proxy-User-Email", c.userID+"@g8e.local")
	}
	if c.cliSessionID != "" {
		httpReq.Header.Set("X-Proxy-CLI-Session-Id", c.cliSessionID)
	}

	body, _, err := doRequest(c.publicClient, httpReq, http.StatusOK)
	if err != nil {
		return EnsembleChatResponse{}, fmt.Errorf("execute chat request: %w", err)
	}

	return decodeJSON[EnsembleChatResponse](body, "ensemble chat response")
}

// SubmitGovernanceEnvelope posts a GovernanceEnvelope proto to /api/v1/governance/envelopes.
func (c *E2EClient) SubmitGovernanceEnvelope(ctx context.Context, env *governance.GovernanceEnvelope) (string, int, []byte, error) {
	wire, err := protojson.Marshal(env)
	if err != nil {
		return env.Id, 0, nil, fmt.Errorf("marshal governance envelope: %w", err)
	}

	req, err := c.newAuthenticatedRequest(ctx, http.MethodPost, constants.APIPaths.GovernanceEnvelopes, bytes.NewReader(wire))
	if err != nil {
		return env.Id, 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.mtlsClient.Do(req)
	if err != nil {
		return env.Id, 0, nil, fmt.Errorf("submit governance envelope: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return env.Id, resp.StatusCode, nil, fmt.Errorf("read governance envelope response: %w", err)
	}
	return env.Id, resp.StatusCode, body, nil
}

// GetDocument retrieves a document from /api/v1/data/{collection}/{id}.
func (c *E2EClient) GetDocument(ctx context.Context, collection, documentID string) (DocumentResponse, int, error) {
	path := constants.APIPaths.DataPrefix + collection + "/" + documentID
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.mtlsClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("get document %s/%s: %w", collection, documentID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read get document response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, http.StatusNotFound, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("get document status %d: %s", resp.StatusCode, truncateBody(body))
	}
	var doc DocumentResponse
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode document response: %w", err)
	}
	return doc, resp.StatusCode, nil
}

// GetStateRoot fetches the current state_merkle_root from /state on the gateway.
func (c *E2EClient) GetStateRoot(ctx context.Context) (string, error) {
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, constants.APIPaths.State, nil)
	if err != nil {
		return "", err
	}
	body, _, err := doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("fetch state root: %w", err)
	}
	var h struct {
		StateMerkleRoot string `json:"state_merkle_root"`
		StateRoot       string `json:"state_root"`
	}
	if err := json.Unmarshal(body, &h); err != nil {
		return "", fmt.Errorf("decode state root response: %w", err)
	}
	if h.StateMerkleRoot != "" {
		return h.StateMerkleRoot, nil
	}
	return h.StateRoot, nil
}

// E2EApprovalAutoApprover listens on gateway SSE and auto-approves file edit approval requests.
type E2EApprovalAutoApprover struct {
	client      *E2EClient
	ensembleURL string
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	connectedCh chan struct{}
}

// StartApprovalAutoApprover starts a background SSE listener that auto-approves file edit approval requests.
func (c *E2EClient) StartApprovalAutoApprover(ctx context.Context, ensembleURL string) *E2EApprovalAutoApprover {
	subCtx, cancel := context.WithCancel(ctx)
	approver := &E2EApprovalAutoApprover{
		client:      c,
		ensembleURL: ensembleURL,
		cancel:      cancel,
		connectedCh: make(chan struct{}),
	}

	sseHTTPClient := &http.Client{
		Timeout:   0,
		Transport: c.mtlsClient.Transport,
	}
	sseURL := fmt.Sprintf("%s%s?since_id=0", c.gatewayHTTPS, constants.APIPaths.SSEStream)
	sseClient := sse.NewClient(sseURL, sseHTTPClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, c.cliSessionID)
	var once sync.Once
	sseClient.SetOnConnect(func() {
		once.Do(func() {
			close(approver.connectedCh)
		})
	})

	approver.wg.Add(1)
	go func() {
		defer approver.wg.Done()
		sseClient.Run(subCtx, func(eventType, data string) {
			var payload models.SSEPushPayload
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				return
			}
			var wire struct {
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(payload.Event, &wire); err != nil {
				return
			}
			innerType := eventType
			if innerType == "" {
				innerType = wire.Type
			}
			if innerType != string(constants.EventOperatorFileEditApprovalRequested) {
				return
			}
			var approvalData struct {
				ApprovalID      string `json:"approval_id"`
				UserID          string `json:"user_id"`
				CLISessionID    string `json:"cli_session_id"`
				CaseID          string `json:"case_id"`
				InvestigationID string `json:"investigation_id"`
			}
			if err := json.Unmarshal(wire.Data, &approvalData); err != nil || approvalData.ApprovalID == "" {
				return
			}
			approver.respondApproval(approvalData)
		})
	}()

	return approver
}

// WaitForConnection waits for the SSE stream connection to be established.
func (a *E2EApprovalAutoApprover) WaitForConnection(ctx context.Context) error {
	select {
	case <-a.connectedCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop stops the auto approver.
func (a *E2EApprovalAutoApprover) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
	a.wg.Wait()
}

func (a *E2EApprovalAutoApprover) respondApproval(ad struct {
	ApprovalID      string `json:"approval_id"`
	UserID          string `json:"user_id"`
	CLISessionID    string `json:"cli_session_id"`
	CaseID          string `json:"case_id"`
	InvestigationID string `json:"investigation_id"`
}) {
	userID := ad.UserID
	if userID == "" {
		userID = a.client.userID
	}
	cliSessionID := ad.CLISessionID
	if cliSessionID == "" {
		cliSessionID = a.client.cliSessionID
	}

	body := map[string]any{
		"context": map[string]any{
			"cli_session_id":   cliSessionID,
			"user_id":          userID,
			"case_id":          ad.CaseID,
			"investigation_id": ad.InvestigationID,
			"source_component": "CLIENT",
			"bound_operators":  []map[string]any{},
		},
		"approval_id": ad.ApprovalID,
		"approved":    true,
		"reason":      "Auto-approved by E2E test",
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return
	}

	url := a.ensembleURL + "/api/v1/operator/approval/respond"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if userID != "" {
		req.Header.Set("X-Proxy-User-Id", userID)
		req.Header.Set("X-Proxy-User-Email", userID+"@g8e.local")
	}
	if cliSessionID != "" {
		req.Header.Set("X-Proxy-CLI-Session-Id", cliSessionID)
	}

	resp, err := a.client.publicClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

// newE2EClient constructs an E2EClient from the resolved e2eConfig. It loads
// the owner CLI certificate and key from disk, reads the CA bundle from the
// runtime tree, and builds strict mTLS HTTP clients using ServerName derived
// from the validated HTTPS URL. No InsecureSkipVerify — normal Go TLS hostname
// and chain verification is used.
func newE2EClient(ctx context.Context, cfg *e2eConfig) (*E2EClient, error) {
	cliCert, err := tls.LoadX509KeyPair(cfg.cliCertPath, cfg.cliKeyPath)
	if err != nil {
		return nil, fmt.Errorf("e2e: load CLI key pair: %w", err)
	}

	caBundle, err := cfg.readCABundle(ctx)
	if err != nil {
		return nil, fmt.Errorf("e2e: read CA bundle: %w", err)
	}

	caPool, err := parseCAPool(caBundle)
	if err != nil {
		return nil, fmt.Errorf("e2e: parse CA bundle: %w", err)
	}

	serverName, err := extractServerName(cfg.gatewayHTTPSURL)
	if err != nil {
		return nil, fmt.Errorf("e2e: extract server name: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates:     []tls.Certificate{cliCert},
		RootCAs:          caPool,
		ServerName:       serverName,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: certs.FIPSCurvePreferences(),
	}

	mtlsClient := &http.Client{
		Transport: httpclient.NewIPv4Transport(tlsConfig),
		Timeout:   defaultClientTimeout,
	}

	publicTLSConfig := &tls.Config{
		RootCAs:          caPool,
		ServerName:       serverName,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: certs.FIPSCurvePreferences(),
	}

	publicClient := &http.Client{
		Transport: httpclient.NewIPv4Transport(publicTLSConfig),
		Timeout:   defaultClientTimeout,
	}

	return &E2EClient{
		publicClient: publicClient,
		mtlsClient:   mtlsClient,
		cliSessionID: cfg.cliSessionID,
		userID:       cfg.userID,
		gatewayHTTPS: cfg.gatewayHTTPSURL,
	}, nil
}
