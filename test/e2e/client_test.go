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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// E2EClient owns bounded public and mTLS HTTP clients for communicating with
// a running production platform. It has no Docker fields, no test framework
// dependency, no bootstrap mutation, and no filesystem writes. Each request
// accepts a context, uses constants from constants.APIPaths, applies the CLI
// session header when authenticated, limits response reads, checks status
// codes, decodes a typed response, and returns contextual errors.
type E2EClient struct {
	publicClient *http.Client // no client cert, for health/CA bundle endpoints
	mtlsClient   *http.Client // owner CLI cert, for authenticated endpoints
	cliSessionID string
	gatewayHTTPS string
}

// newAuthenticatedRequest builds an HTTP request with the CLI session header
// set, targeting the gateway HTTPS URL. The body parameter may be nil for GET
// requests.
func (c *E2EClient) newAuthenticatedRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	reqURL := c.gatewayHTTPS + path
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, path, err)
	}
	req.Header.Set(constants.HeaderCLISessionID, c.cliSessionID)
	return req, nil
}

// GetHealth fetches the gateway health endpoint over HTTP (no mTLS) and
// returns the typed HealthResponse.
func (c *E2EClient) GetHealth(ctx context.Context, httpURL string) (models.HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpURL+constants.APIPaths.Health, nil)
	if err != nil {
		return models.HealthResponse{}, fmt.Errorf("build health request: %w", err)
	}
	body, _, err := doRequest(c.publicClient, req, http.StatusOK)
	if err != nil {
		return models.HealthResponse{}, fmt.Errorf("health check: %w", err)
	}
	return decodeJSON[models.HealthResponse](body, "health response")
}

// GetCABundle fetches the CA bundle from the gateway's well-known PKI endpoint
// over HTTP (no mTLS). Returns the raw PEM bytes.
func (c *E2EClient) GetCABundle(ctx context.Context, httpURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpURL+constants.APIPaths.WellKnownPKICABundle, nil)
	if err != nil {
		return nil, fmt.Errorf("build CA bundle request: %w", err)
	}
	body, _, err := doRequest(c.publicClient, req, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("fetch CA bundle: %w", err)
	}
	return body, nil
}

// GetPendingEnrollments fetches the authenticated pending platform enrollment
// list and returns the typed response.
func (c *E2EClient) GetPendingEnrollments(ctx context.Context) (models.PlatformEnrollmentPendingResponse, error) {
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
	if err != nil {
		return models.PlatformEnrollmentPendingResponse{}, err
	}
	body, _, err := doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return models.PlatformEnrollmentPendingResponse{}, fmt.Errorf("fetch pending enrollments: %w", err)
	}
	return decodeJSON[models.PlatformEnrollmentPendingResponse](body, "pending enrollments")
}

// GetPendingRaw fetches the authenticated pending platform enrollment list and
// returns the raw JSON body as a string. Used by tests that verify the wire
// payload does not contain secret fields.
func (c *E2EClient) GetPendingRaw(ctx context.Context) (string, error) {
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, constants.APIPaths.AuthPlatformEnrollmentPending, nil)
	if err != nil {
		return "", err
	}
	body, _, err := doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("fetch pending raw: %w", err)
	}
	return string(body), nil
}

// PostEnrollmentDecision posts an approve or deny decision for the given
// request ID via the authenticated decision endpoint.
func (c *E2EClient) PostEnrollmentDecision(ctx context.Context, requestID string, decision models.PlatformEnrollmentDecision) error {
	body, err := json.Marshal(models.PlatformEnrollmentDecisionRequest{
		RequestID: requestID,
		Decision:  decision,
	})
	if err != nil {
		return fmt.Errorf("marshal decision body: %w", err)
	}
	req, err := c.newAuthenticatedRequest(ctx, http.MethodPost, constants.APIPaths.AuthPlatformEnrollmentDecision, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	_, _, err = doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return fmt.Errorf("post enrollment decision: %w", err)
	}
	return nil
}

// ApproveEnrollment posts an approve decision for the given request ID.
func (c *E2EClient) ApproveEnrollment(ctx context.Context, requestID string) error {
	return c.PostEnrollmentDecision(ctx, requestID, models.PlatformEnrollmentDecisionApprove)
}

// DenyEnrollment posts a deny decision for the given request ID.
func (c *E2EClient) DenyEnrollment(ctx context.Context, requestID string) error {
	return c.PostEnrollmentDecision(ctx, requestID, models.PlatformEnrollmentDecisionDeny)
}

// ListOperators fetches the operator list via the owner-authenticated operators
// endpoint and returns the typed slot response.
func (c *E2EClient) ListOperators(ctx context.Context) (models.OperatorSlotResponse, error) {
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, constants.APIPaths.Operators, nil)
	if err != nil {
		return models.OperatorSlotResponse{}, err
	}
	body, _, err := doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return models.OperatorSlotResponse{}, fmt.Errorf("list operators: %w", err)
	}
	return decodeJSON[models.OperatorSlotResponse](body, "operator list")
}

// GetOperatorBySession fetches a single operator by session ID via the
// owner-authenticated session lookup endpoint and returns the typed response.
func (c *E2EClient) GetOperatorBySession(ctx context.Context, sessionID string) (models.OperatorResponse, error) {
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, constants.APIPaths.OperatorsSession+sessionID, nil)
	if err != nil {
		return models.OperatorResponse{}, err
	}
	body, _, err := doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return models.OperatorResponse{}, fmt.Errorf("get operator by session: %w", err)
	}
	return decodeJSON[models.OperatorResponse](body, "operator response")
}

// DispatchCommand sends a command dispatch request to the gateway via the
// owner-authenticated commands endpoint. The dispatchRequestJSON body carries
// the target operator session ID, action type, and protobuf payload. The
// dispatchResponseJSON body carries the transaction ID, event type, and result
// payload. The timeout parameter overrides the default client timeout for
// long-running dispatch operations.
func (c *E2EClient) DispatchCommand(ctx context.Context, body dispatchRequestJSON, timeout time.Duration) (dispatchResponseJSON, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return dispatchResponseJSON{}, fmt.Errorf("marshal dispatch body: %w", err)
	}
	req, err := c.newAuthenticatedRequest(ctx, http.MethodPost, constants.APIPaths.OperatorsCommands, bytes.NewReader(bodyBytes))
	if err != nil {
		return dispatchResponseJSON{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.mtlsClient
	if timeout > 0 && timeout != defaultClientTimeout {
		client = &http.Client{
			Transport: client.Transport,
			Timeout:   timeout,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return dispatchResponseJSON{}, fmt.Errorf("execute dispatch request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return dispatchResponseJSON{}, fmt.Errorf("read dispatch response: %w", err)
	}
	if len(respBody) > maxResponseBytes {
		return dispatchResponseJSON{}, fmt.Errorf("dispatch response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return dispatchResponseJSON{}, fmt.Errorf("dispatch status %d: %s", resp.StatusCode, truncateBody(respBody))
	}
	return decodeJSON[dispatchResponseJSON](respBody, "dispatch response")
}

// GetEnsembleHealth fetches the ensemble /health endpoint over HTTP (no mTLS)
// and returns the raw body bytes. The ensemble is a Python/FastAPI service
// with its own health endpoint on a separate port.
func (c *E2EClient) GetEnsembleHealth(ctx context.Context, ensembleURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ensembleURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("build ensemble health request: %w", err)
	}
	body, _, err := doRequest(c.publicClient, req, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("ensemble health: %w", err)
	}
	return body, nil
}

// GetEnsembleDetailedHealth fetches the ensemble /health/details endpoint over
// HTTP (no mTLS) and returns the raw body bytes.
func (c *E2EClient) GetEnsembleDetailedHealth(ctx context.Context, ensembleURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ensembleURL+"/health/details", nil)
	if err != nil {
		return nil, fmt.Errorf("build ensemble detailed health request: %w", err)
	}
	body, _, err := doRequest(c.publicClient, req, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("ensemble detailed health: %w", err)
	}
	return body, nil
}

// GetDashboardIndex fetches the dashboard index page over HTTP (no mTLS) and
// returns the raw body bytes. The dashboard is a Node.js/Express service on
// a separate port.
func (c *E2EClient) GetDashboardIndex(ctx context.Context, dashboardURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardURL+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("build dashboard index request: %w", err)
	}
	body, _, err := doRequest(c.publicClient, req, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("dashboard index: %w", err)
	}
	return body, nil
}

// newBareMTLSRequest builds an HTTP GET request targeting the given URL
// without setting the CLI session header. It is used by tests that verify the
// session header is enforced: the mTLS handshake completes (the client cert
// is valid) but the authenticated route rejects the request for a missing
// header. The request is executed by the caller via the E2EClient's mtlsClient
// so the same strict TLS configuration applies.
func newBareMTLSRequest(ctx context.Context, reqURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build bare mTLS request: %w", err)
	}
	return req, nil
}

// GetAuditReceipts fetches the authenticated audit receipts list and returns
// the typed response. The optional txID parameter filters by transaction ID;
// when empty, all receipts are listed up to the server default limit.
func (c *E2EClient) GetAuditReceipts(ctx context.Context, txID string) (models.AuditReceiptsResponse, error) {
	path := constants.APIPaths.AuditReceipts
	if txID != "" {
		path += "?tx_id=" + txID
	}
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return models.AuditReceiptsResponse{}, err
	}
	body, _, err := doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return models.AuditReceiptsResponse{}, fmt.Errorf("fetch audit receipts: %w", err)
	}
	return decodeJSON[models.AuditReceiptsResponse](body, "audit receipts")
}

// GetAuditSummary fetches the authenticated audit summary and returns the
// typed response with event and receipt counts broken down by type.
func (c *E2EClient) GetAuditSummary(ctx context.Context) (models.AuditSummaryResponse, error) {
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, constants.APIPaths.AuditSummary, nil)
	if err != nil {
		return models.AuditSummaryResponse{}, err
	}
	body, _, err := doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return models.AuditSummaryResponse{}, fmt.Errorf("fetch audit summary: %w", err)
	}
	return decodeJSON[models.AuditSummaryResponse](body, "audit summary")
}

// GetAuditEvents fetches the authenticated audit events list and returns the
// typed response.
func (c *E2EClient) GetAuditEvents(ctx context.Context) (models.AuditEventsResponse, error) {
	req, err := c.newAuthenticatedRequest(ctx, http.MethodGet, constants.APIPaths.AuditEvents, nil)
	if err != nil {
		return models.AuditEventsResponse{}, err
	}
	body, _, err := doRequest(c.mtlsClient, req, http.StatusOK)
	if err != nil {
		return models.AuditEventsResponse{}, fmt.Errorf("fetch audit events: %w", err)
	}
	return decodeJSON[models.AuditEventsResponse](body, "audit events")
}