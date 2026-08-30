// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// ActingAppG8ee is the acting_app_id the ensemble (g8ee) stamps on every
// governance envelope it builds (ensemble/app/constants/config.py
// G8EE_COMPONENT). The harness uses the same value for direct document
// envelope submission so receipts attribute the action to g8ee, matching
// what the ensemble would produce.
const ActingAppG8ee = "g8ee"

// DocumentUpdateRequest is the input for a governed DOCUMENT_UPDATE envelope
// submitted directly to the admission API (POST /api/v1/governance/envelopes).
// This bypasses the ensemble/AI layer so scenarios can deterministically
// create, merge-patch, and delete documents without relying on the LLM to
// choose the right tool call.
type DocumentUpdateRequest struct {
	OperatorID        string
	OperatorSessionID string
	RequestorUserID   string // human user who authorized the action
	Collection        string
	DocumentID        string
	Updates           map[string]any // field updates (merged when Merge=true, replaced when Merge=false)
	Merge             bool           // true=PATCH (merge), false=PUT (replace)
	StateRoot         string
	TTL               time.Duration
}

// DocumentDeleteRequest is the input for a governed DOCUMENT_DELETE envelope.
type DocumentDeleteRequest struct {
	OperatorID        string
	OperatorSessionID string
	RequestorUserID   string
	Collection        string
	DocumentID        string
	StateRoot         string
	TTL               time.Duration
}

// DocumentResponse is the wire shape returned by GET /api/v1/data/{collection}/{id}.
// The gateway serializes a stored Document via ForWire(), producing a
// map[string]json.RawMessage where system fields (id, created_at, updated_at)
// are merged with the document's data fields. Each value is a raw JSON token
// (string, number, bool, etc.) that the caller unmarshals as needed.
type DocumentResponse map[string]json.RawMessage

// GetString extracts a string field from a DocumentResponse. Returns "" if
// the field is absent or not a JSON string.
func (d DocumentResponse) GetString(field string) string {
	raw, ok := d[field]
	if !ok {
		return ""
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// GetBool extracts a bool field from a DocumentResponse. Returns false if the
// field is absent or not a JSON bool.
func (d DocumentResponse) GetBool(field string) bool {
	raw, ok := d[field]
	if !ok {
		return false
	}
	var b bool
	_ = json.Unmarshal(raw, &b)
	return b
}

// SubmitDocumentUpdate builds a GovernanceEnvelope wrapping a
// DocumentUpdateRequested payload, computes the canonical transaction hash
// with g8e's own hasher, and POSTs it to the admission API. Returns the
// computed hash and the HTTP response. The envelope carries the canonical
// EventAppDocumentUpdateRequested event type and DOCUMENT_UPDATE action type
// so dispatch is deterministic (A.2). Identity fields (RequestorUserID,
// ActingAppG8ee, OperatorID, OperatorSessionID, CLISessionID) are bound into
// the envelope and included in the transaction hash so they are
// cryptographically tamper-evident.
func (c *Client) SubmitDocumentUpdate(ctx context.Context, p Persona, req DocumentUpdateRequest) (txHash string, status int, body []byte, err error) {
	updates, err := structpb.NewStruct(req.Updates)
	if err != nil {
		return "", 0, nil, fmt.Errorf("submit document update: build updates struct: %w", err)
	}

	payload := &operatorv1.DocumentUpdateRequested{
		Collection: req.Collection,
		DocumentId: req.DocumentID,
		Updates:    updates,
		Merge:      req.Merge,
	}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return "", 0, nil, fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
	}

	env, err := buildDocumentEnvelope(
		payloadBytes,
		string(constants.EventAppDocumentUpdateRequested),
		string(constants.ActionTypeDocumentUpdate),
		req.Collection+"/"+req.DocumentID,
		req.OperatorID,
		req.OperatorSessionID,
		p.CLISessionID,
		req.RequestorUserID,
		ActingAppG8ee,
		req.StateRoot,
		req.TTL,
	)
	if err != nil {
		return "", 0, nil, err
	}
	txHash = env.Id

	wire, err := protojson.Marshal(env)
	if err != nil {
		return txHash, 0, nil, fmt.Errorf("%w: %w", constants.ErrPubSubMarshalEnvelope, err)
	}

	status, body, err = c.do(ctx, p, http.MethodPost, c.cfg.MTLSBaseURL+constants.APIPaths.GovernanceEnvelopes, wire)
	return txHash, status, body, err
}

// SubmitDocumentDelete builds a GovernanceEnvelope wrapping a
// DocumentDeleteRequested payload, computes the canonical transaction hash,
// and POSTs it to the admission API. The envelope carries the canonical
// EventAppDocumentDeleteRequested event type and DOCUMENT_DELETE action type.
func (c *Client) SubmitDocumentDelete(ctx context.Context, p Persona, req DocumentDeleteRequest) (txHash string, status int, body []byte, err error) {
	payload := &operatorv1.DocumentDeleteRequested{
		Collection: req.Collection,
		DocumentId: req.DocumentID,
	}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return "", 0, nil, fmt.Errorf("%w: %w", constants.ErrRequestMarshalFailed, err)
	}

	env, err := buildDocumentEnvelope(
		payloadBytes,
		string(constants.EventAppDocumentDeleteRequested),
		string(constants.ActionTypeDocumentDelete),
		req.Collection+"/"+req.DocumentID,
		req.OperatorID,
		req.OperatorSessionID,
		p.CLISessionID,
		req.RequestorUserID,
		ActingAppG8ee,
		req.StateRoot,
		req.TTL,
	)
	if err != nil {
		return "", 0, nil, err
	}
	txHash = env.Id

	wire, err := protojson.Marshal(env)
	if err != nil {
		return txHash, 0, nil, fmt.Errorf("%w: %w", constants.ErrPubSubMarshalEnvelope, err)
	}

	status, body, err = c.do(ctx, p, http.MethodPost, c.cfg.MTLSBaseURL+constants.APIPaths.GovernanceEnvelopes, wire)
	return txHash, status, body, err
}

// GetDocument retrieves a governed document from the gateway's document store
// via GET /api/v1/data/{collection}/{id}. This is the read-back path for
// document scenarios: the harness verifies the actual stored document, not
// just a receipt claiming the mutation occurred. Returns (nil, nil) when the
// document does not exist (HTTP 404), so callers can distinguish absence from
// an error.
func (c *Client) GetDocument(ctx context.Context, p Persona, collection, documentID string) (DocumentResponse, []byte, error) {
	u := c.cfg.MTLSBaseURL + constants.APIPaths.DataPrefix + collection + "/" + documentID
	status, body, err := c.do(ctx, p, http.MethodGet, u, nil)
	if err != nil {
		return nil, body, err
	}
	if status == http.StatusNotFound {
		return nil, body, nil
	}
	if status >= 400 {
		return nil, body, fmt.Errorf("get document: gateway returned status %d for %s/%s: %s", status, collection, documentID, string(body))
	}
	var doc DocumentResponse
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, body, fmt.Errorf("get document: decode response: %w", err)
	}
	return doc, body, nil
}

// buildDocumentEnvelope constructs a canonical GovernanceEnvelope for a
// document operation, computes the transaction hash via g8e's real hasher, and
// stamps Id/TransactionHash. The source component is COMPONENT_AGENT (the
// ensemble is an agent acting on behalf of the user). L1 governance metadata
// is marked validated; L2/L3 are left to the gateway's gauntlet.
func buildDocumentEnvelope(
	payloadBytes []byte,
	eventType, actionType, targetResource,
	operatorID, operatorSessionID, cliSessionID, requestorUserID, actingAppID,
	stateRoot string,
	ttl time.Duration,
) (*governance.GovernanceEnvelope, error) {
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)

	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	env := &governance.GovernanceEnvelope{
		ProtocolVersion:   ProtocolVersion,
		Timestamp:         timestamppb.Now(),
		ExpiresAt:         timestamppb.New(time.Now().Add(ttl)),
		SourceComponent:   commonv1.Component_COMPONENT_AGENT,
		OperatorId:        operatorID,
		OperatorSessionId: operatorSessionID,
		CliSessionId:      cliSessionID,
		ActionType:        actionType,
		TargetResource:    targetResource,
		EventType:         eventType,
		Payload:           payloadBytes,
		StateMerkleRoot:   stateRoot,
		Nonce:             hex.EncodeToString(nonce),
		RequestorUserId:   requestorUserID,
		ActingAppId:       actingAppID,
	}

	hash, err := governance.GenerateMessageID(env)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrTxTransactionHashMissing, err)
	}
	env.Id = hash
	env.TransactionHash = hash

	env.Governance = &commonv1.GovernanceMetadata{L1: &commonv1.L1Metadata{Validated: true}}
	return env, nil
}
