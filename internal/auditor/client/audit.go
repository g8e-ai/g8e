// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
)

// Receipt is a lenient view of an Operator-signed ActionReceipt as exposed by
// /api/audit/receipts. The Operator is the source of truth; these are the real,
// signed records of what actually executed on the host.
type Receipt struct {
	TransactionID   string          `json:"transaction_id"`
	TransactionHash string          `json:"transaction_hash"`
	ActionType      string          `json:"action_type"`
	TargetResource  string          `json:"target_resource"`
	Status          string          `json:"status"`
	StateRootBefore string          `json:"state_root_before"`
	StateRootAfter  string          `json:"state_root_after"`
	Signature       string          `json:"signature"`
	Raw             json.RawMessage `json:"-"`
}

// GetReceipt retrieves a single receipt by transaction ID.
func (c *Client) GetReceipt(ctx context.Context, transactionID string, persona ...Persona) (*Receipt, []byte, error) {
	p := Persona{ID: "phantom-auditor"}
	if len(persona) > 0 {
		p = persona[0]
	}
	u := c.cfg.MTLSBaseURL + "/api/audit/receipts?tx_id=" + url.QueryEscape(transactionID)
	status, body, err := c.do(ctx, p, http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	if status == http.StatusNotFound {
		return nil, body, nil
	}
	if status >= 400 {
		return nil, body, fmt.Errorf("gateway returned status %d for transaction %s: %s", status, transactionID, string(body))
	}
	var rec Receipt
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, body, fmt.Errorf("failed to unmarshal receipt: %w", err)
	}
	rec.Raw = body
	return &rec, body, nil
}

// AuditReceipts pulls signed receipts from the Operator's local audit vault via
// the Gateway, optionally scoped to an Operator session.
func (c *Client) AuditReceipts(ctx context.Context, operatorSessionID string) ([]Receipt, []byte, error) {
	u := c.cfg.MTLSBaseURL + "/api/audit/receipts"
	if operatorSessionID != "" {
		u += "?" + url.Values{"operator_session_id": {operatorSessionID}}.Encode()
	}
	_, body, err := c.do(ctx, Persona{ID: "phantom-auditor"}, http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	receipts := parseReceipts(body)
	return receipts, body, nil
}

// ExportReceipts pulls the full export bundle for archival alongside the report.
func (c *Client) ExportReceipts(ctx context.Context, operatorSessionID string) ([]byte, error) {
	u := c.cfg.MTLSBaseURL + "/api/audit/receipts/export"
	if operatorSessionID != "" {
		u += "?" + url.Values{"operator_session_id": {operatorSessionID}}.Encode()
	}
	_, body, err := c.do(ctx, Persona{ID: "phantom-auditor"}, http.MethodGet, u, nil)
	return body, err
}

// DiscoverOperatorSession best-effort reads /api/operators to find a live
// Operator session id when the user didn't pin one.
func (c *Client) DiscoverOperatorSession(ctx context.Context) string {
	// If Operator session ID is already pinned in config, use it
	if c.cfg.OperatorSessionID != "" {
		return c.cfg.OperatorSessionID
	}

	// Try to load user_id and operator_session_id from CLI credentials
	userID := ""
	operatorSessionID := ""
	if c.cfg.UseCLIConfig {
		cliCfg, err := config.Load("")
		if err == nil && cliCfg != nil {
			creds, err := auth.LoadCredentials(cliCfg)
			if err == nil && creds != nil {
				userID = creds.UserID
				operatorSessionID = creds.OperatorSessionID
			}
		}
	}

	// If we already have the Operator session ID from credentials, return it directly
	if operatorSessionID != "" {
		return operatorSessionID
	}

	url := c.cfg.MTLSBaseURL + "/api/operators"
	if userID != "" {
		url += "?user_id=" + userID
	}

	_, body, err := c.do(ctx, Persona{ID: "phantom"}, http.MethodGet, url, nil)
	if err != nil || !json.Valid(body) {
		return ""
	}
	// Tolerate {"operators":[...]} or a bare array.
	var wrap struct {
		Operators []map[string]any `json:"operators"`
	}
	if json.Unmarshal(body, &wrap) == nil {
		for _, o := range wrap.Operators {
			if s, _ := o["operator_session_id"].(string); s != "" {
				return s
			}
		}
	}
	var arr []map[string]any
	if json.Unmarshal(body, &arr) == nil {
		for _, o := range arr {
			if s, _ := o["operator_session_id"].(string); s != "" {
				return s
			}
		}
	}
	return ""
}

// parseReceipts tolerates {"receipts":[...]} or a bare array of receipts.
func parseReceipts(body []byte) []Receipt {
	if !json.Valid(body) {
		return nil
	}
	var wrap struct {
		Receipts []json.RawMessage `json:"receipts"`
	}
	var rows []json.RawMessage
	if json.Unmarshal(body, &wrap) == nil && len(wrap.Receipts) > 0 {
		rows = wrap.Receipts
	} else {
		_ = json.Unmarshal(body, &rows)
	}
	out := make([]Receipt, 0, len(rows))
	for _, r := range rows {
		var rec Receipt
		_ = json.Unmarshal(r, &rec)
		rec.Raw = r
		out = append(out, rec)
	}
	return out
}
