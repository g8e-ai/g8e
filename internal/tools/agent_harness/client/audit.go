// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
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
	p := c.auditorPersona()
	if len(persona) > 0 {
		p = persona[0]
	}
	u := c.cfg.MTLSBaseURL + constants.APIPaths.AuditReceipts + "?tx_id=" + url.QueryEscape(transactionID)
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

// auditorPersona builds a persona carrying the config's CLI session and user
// identity so authenticated audit endpoints accept the request. The harness
// dials these endpoints with the owner CLI cert, so the gateway requires the
// X-CLI-Session-ID header to bind the mTLS cert to an active session.
func (c *Client) auditorPersona() Persona {
	return Persona{
		ID:           "agent-harness-auditor",
		CLISessionID: c.cfg.CLISessionID,
		UserID:       c.cfg.UserID,
	}
}

// AuditReceipts pulls signed receipts from the Operator's local audit vault via
// the Gateway, optionally scoped to an Operator session.
func (c *Client) AuditReceipts(ctx context.Context, operatorSessionID string) ([]Receipt, []byte, error) {
	u := c.cfg.MTLSBaseURL + constants.APIPaths.AuditReceipts
	if operatorSessionID != "" {
		u += "?" + url.Values{"operator_session_id": {operatorSessionID}}.Encode()
	}
	_, body, err := c.do(ctx, c.auditorPersona(), http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	receipts := parseReceipts(body)
	return receipts, body, nil
}

// ExportReceipts pulls the full export bundle for archival alongside the report.
func (c *Client) ExportReceipts(ctx context.Context, operatorSessionID string) ([]byte, error) {
	u := c.cfg.MTLSBaseURL + constants.APIPaths.AuditReceiptsExport
	if operatorSessionID != "" {
		u += "?" + url.Values{"operator_session_id": {operatorSessionID}}.Encode()
	}
	_, body, err := c.do(ctx, c.auditorPersona(), http.MethodGet, u, nil)
	return body, err
}

// DiscoverOperatorSession best-effort reads /api/operators to find a live
// Operator session id when the user didn't pin one.
func (c *Client) DiscoverOperatorSession(ctx context.Context) string {
	_, sid := c.DiscoverOperator(ctx)
	return sid
}

// DiscoverOperator reads /api/operators to find a live Operator's ID and session ID.
// Returns ("", "") if none found.
func (c *Client) DiscoverOperator(ctx context.Context) (operatorID, operatorSessionID string) {
	// If Operator session ID is already pinned in config, use it
	if c.cfg.OperatorSessionID != "" {
		return "", c.cfg.OperatorSessionID
	}

	// Try to extract directly from the client cert SAN first
	if c.cfg.Auth.ClientCert != "" {
		if certBytes, err := os.ReadFile(c.cfg.Auth.ClientCert); err == nil {
			if block, _ := pem.Decode(certBytes); block != nil && block.Type == "CERTIFICATE" {
				if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
					for _, u := range cert.URIs {
						if u.Scheme == "spiffe" && strings.HasPrefix(u.Path, "/operator/") {
							parts := strings.Split(strings.TrimPrefix(u.Path, "/operator/"), "/")
							if len(parts) >= 3 {
								return parts[1], parts[2]
							}
						}
					}
				}
			}
		}
	}

	// Try to load user_id and operator_session_id from CLI credentials
	userID := ""
	if c.cfg.UseCLIConfig {
		cliCfg, err := config.Load("")
		if err == nil && cliCfg != nil {
			fileSvc, err := fs.NewRuntimeFileService("", slog.Default())
			if err == nil {
				creds, err := auth.LoadCredentials(fileSvc, cliCfg)
				if err == nil && creds != nil {
					userID = creds.UserID
					operatorSessionID = creds.OperatorSessionID
				}
			}
		}
	}

	// If we already have the Operator session ID from credentials, return it directly
	if operatorSessionID != "" {
		return "", operatorSessionID
	}

	url := c.cfg.MTLSBaseURL + constants.APIPaths.Operators
	// Prefer the user_id from explicit config flags (--user-id) over the
	// CLI credentials lookup, since the harness may be pointed at a
	// gateway whose CLI credentials differ from the local .g8e/ tree.
	effectiveUserID := c.cfg.UserID
	if effectiveUserID == "" {
		effectiveUserID = userID
	}
	if effectiveUserID != "" {
		url += "?user_id=" + effectiveUserID
	}

	_, body, err := c.do(ctx, Persona{
		ID:           "agent-harness",
		CLISessionID: c.cfg.CLISessionID,
		UserID:       effectiveUserID,
	}, http.MethodGet, url, nil)
	if err != nil || !json.Valid(body) {
		return "", ""
	}
	// Tolerate {"operators":[...]} or a bare array.
	var wrap struct {
		Operators []map[string]any `json:"operators"`
	}
	if json.Unmarshal(body, &wrap) == nil {
		for _, o := range wrap.Operators {
			if s, _ := o["operator_session_id"].(string); s != "" {
				id, _ := o["id"].(string)
				return id, s
			}
		}
	}
	var arr []map[string]any
	if json.Unmarshal(body, &arr) == nil {
		for _, o := range arr {
			if s, _ := o["operator_session_id"].(string); s != "" {
				id, _ := o["id"].(string)
				return id, s
			}
		}
	}
	return "", ""
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
