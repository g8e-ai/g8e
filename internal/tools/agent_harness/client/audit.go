// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// Receipt is a lenient view of an Operator-signed ActionReceipt as exposed by
// /api/audit/receipts. The Operator is the source of truth; these are the real,
// signed records of what actually executed on the host.
type Receipt struct {
	ExecutionID       string          `json:"execution_id"`
	TransactionID     string          `json:"transaction_id"`
	TransactionHash   string          `json:"transaction_hash"`
	InvestigationID   string          `json:"investigation_id"`
	OperatorID        string          `json:"operator_id"`
	OperatorSessionID string          `json:"operator_session_id"`
	RequestorUserID   string          `json:"requestor_user_id"`
	ActingAppID       string          `json:"acting_app_id"`
	ActionType        string          `json:"action_type"`
	TargetResource    string          `json:"target_resource"`
	Status            string          `json:"status"`
	StateRootBefore   string          `json:"state_root_before"`
	StateRootAfter    string          `json:"state_root_after"`
	ExecutedAt        time.Time       `json:"executed_at"`
	SignerKeyID       string          `json:"signer_key_id"`
	Signature         string          `json:"signature"`
	Raw               json.RawMessage `json:"-"`
}

type receiptAlias Receipt

// UnmarshalJSON unmarshals a Receipt, tolerating both string and numeric status.
func (r *Receipt) UnmarshalJSON(data []byte) error {
	type rawReceipt struct {
		receiptAlias
		RawStatus json.RawMessage `json:"status"`
	}
	var aux rawReceipt
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*r = Receipt(aux.receiptAlias)
	r.Raw = data

	if len(aux.RawStatus) > 0 {
		var str string
		if err := json.Unmarshal(aux.RawStatus, &str); err == nil {
			r.Status = str
		} else {
			var num int
			if err := json.Unmarshal(aux.RawStatus, &num); err == nil {
				r.Status = fmt.Sprintf("%d", num)
			}
		}
	}
	return nil
}

// GetReceipt retrieves a single receipt by transaction ID.
func (c *Client) GetReceipt(ctx context.Context, transactionID string, persona ...Persona) (*Receipt, []byte, error) {
	p := c.auditorPersona()
	if len(persona) > 0 {
		p = persona[0]
	}
	u := c.cfg.MTLSBaseURL + constants.APIPaths.AuditReceipts + "?tx_id=" + url.QueryEscape(transactionID)
	status, body, err := c.doWithCLI(ctx, p, http.MethodGet, u, nil)
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

func (c *Client) GetActionReceipt(ctx context.Context, transactionID string, persona ...Persona) (*operatorv1.ActionReceipt, []byte, error) {
	p := c.auditorPersona()
	if len(persona) > 0 {
		p = persona[0]
	}
	u := c.cfg.MTLSBaseURL + constants.APIPaths.AuditReceipts + "?tx_id=" + url.QueryEscape(transactionID)
	status, body, err := c.doWithCLI(ctx, p, http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	if status == http.StatusNotFound {
		return nil, body, nil
	}
	if status >= 400 {
		return nil, body, fmt.Errorf("gateway returned status %d for transaction %s: %s", status, transactionID, string(body))
	}
	receipt := &operatorv1.ActionReceipt{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(body, receipt); err != nil {
		return nil, body, fmt.Errorf("decode canonical action receipt: %w", err)
	}
	return receipt, body, nil
}

// GetTrustedSignerPublicKey fetches the Ed25519 public key for a governance
// trusted signer by key ID from the Gateway's signer endpoint. It is used by
// the harness to independently verify receipt and persistence attestation
// signatures outside the Gateway relay trust boundary. Returns
// constants.ErrTrustedSignerKeyNotFound when the gateway has no record of the
// requested key ID.
func (c *Client) GetTrustedSignerPublicKey(ctx context.Context, keyID string) (ed25519.PublicKey, error) {
	u := c.cfg.MTLSBaseURL + constants.APIPaths.GovernanceSignersByID + url.PathEscape(keyID)
	status, body, err := c.do(ctx, c.auditorPersona(), http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch trusted signer %s: %w", keyID, err)
	}
	if status == http.StatusNotFound {
		return nil, constants.ErrTrustedSignerKeyNotFound
	}
	if status >= 400 {
		return nil, fmt.Errorf("gateway returned status %d for signer %s: %s", status, keyID, string(body))
	}

	var doc struct {
		ID        string `json:"id"`
		PublicKey string `json:"public_key_hex"`
		Enabled   bool   `json:"enabled"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode trusted signer response: %w", err)
	}
	if doc.ID == "" || !doc.Enabled || doc.PublicKey == "" {
		return nil, constants.ErrTrustedSignerKeyNotFound
	}
	pubBytes, err := hex.DecodeString(doc.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode signer public key hex: %w", err)
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: invalid public key size: %d", constants.ErrTrustedSignerKeyNotFound, len(pubBytes))
	}
	return ed25519.PublicKey(pubBytes), nil
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
	_, body, err := c.doWithCLI(ctx, c.auditorPersona(), http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	receipts := parseReceipts(body)
	return receipts, body, nil
}

func (c *Client) AuditReceiptRecords(ctx context.Context, operatorSessionID string) ([]*models.ActionReceiptRecord, []byte, error) {
	if operatorSessionID == "" {
		return nil, nil, constants.ErrMissingRequiredField
	}
	u := c.cfg.MTLSBaseURL + constants.APIPaths.AuditReceipts + "?" + url.Values{"operator_session_id": {operatorSessionID}}.Encode()
	status, body, err := c.doWithCLI(ctx, c.auditorPersona(), http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return nil, body, fmt.Errorf("%w: audit receipt list returned status %d", constants.ErrHTTPStatusError, status)
	}
	response := &models.AuditReceiptsResponse{}
	if err := json.Unmarshal(body, response); err != nil {
		return nil, body, fmt.Errorf("%w: decode audit receipt records: %v", constants.ErrInvalidJSONResponse, err)
	}
	if !response.Success || response.Receipts == nil {
		return nil, body, fmt.Errorf("%w: audit receipt list is incomplete", constants.ErrInvalidJSONResponse)
	}
	return response.Receipts, body, nil
}

// ExportReceipts pulls the full export bundle for archival alongside the report.
func (c *Client) ExportReceipts(ctx context.Context, operatorSessionID string) ([]byte, error) {
	u := c.cfg.MTLSBaseURL + constants.APIPaths.AuditReceiptsExport
	if operatorSessionID != "" {
		u += "?" + url.Values{"operator_session_id": {operatorSessionID}}.Encode()
	}
	_, body, err := c.doWithCLI(ctx, c.auditorPersona(), http.MethodGet, u, nil)
	return body, err
}

// DiscoverOperator resolves one active operator identity from the canonical registry.
// Explicit ID/session constraints must match the same record. Without constraints,
// only data workers qualify; an ambiguous target must be selected explicitly.
func (c *Client) DiscoverOperator(ctx context.Context) (string, string, error) {
	operators, _, err := c.ListOperators(ctx)
	if err != nil {
		return "", "", fmt.Errorf("discover operator: %w", err)
	}
	var matches []models.OperatorDocumentGo
	for _, op := range operators {
		if op.ID == "" || op.OperatorSessionID == "" || op.Status != constants.OperatorStatusActive {
			continue
		}
		if c.cfg.OperatorID != "" && op.ID != c.cfg.OperatorID {
			continue
		}
		if c.cfg.OperatorSessionID != "" && op.OperatorSessionID != c.cfg.OperatorSessionID {
			continue
		}
		if c.cfg.OperatorID == "" && c.cfg.OperatorSessionID == "" && !operatorcapability.IsDataOperator(op) {
			continue
		}
		matches = append(matches, op)
	}
	switch len(matches) {
	case 0:
		return "", "", fmt.Errorf("%w: operator id=%q session=%q", constants.ErrEvaluationTargetUnavailable, c.cfg.OperatorID, c.cfg.OperatorSessionID)
	case 1:
		return matches[0].ID, matches[0].OperatorSessionID, nil
	default:
		return "", "", fmt.Errorf("%w: %d active operators; specify --operator-id or --operator-session", constants.ErrEvaluationTargetAmbiguous, len(matches))
	}
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
