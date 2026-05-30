// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0.

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// Receipt is a lenient view of an Operator-signed ActionReceipt as exposed by
// /api/audit/receipts. The Operator is the source of truth; these are the real,
// signed records of what actually executed on the host.
type Receipt struct {
	TransactionHash string          `json:"transaction_hash"`
	ActionType      string          `json:"action_type"`
	TargetResource  string          `json:"target_resource"`
	Status          string          `json:"status"`
	StateRootBefore string          `json:"state_root_before"`
	StateRootAfter  string          `json:"state_root_after"`
	Signature       string          `json:"signature"`
	Raw             json.RawMessage `json:"-"`
}

// AuditReceipts pulls signed receipts from the Operator's local audit vault via
// the Gateway, optionally scoped to an operator session.
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
// operator session id when the user didn't pin one.
func (c *Client) DiscoverOperatorSession(ctx context.Context) string {
	_, body, err := c.do(ctx, Persona{ID: "phantom"}, http.MethodGet, c.cfg.MTLSBaseURL+"/api/operators", nil)
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
