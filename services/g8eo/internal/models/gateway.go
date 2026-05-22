// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"encoding/json"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/go-webauthn/webauthn/protocol"
)

// Document is the internal representation of a stored document.
// Timestamps are native time.Time - convert to wire format via ForWire().
type Document struct {
	ID         string
	Collection string
	Data       map[string]json.RawMessage
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ForWire serializes Document to a JSON-encodable map for the HTTP response boundary.
// Timestamps are formatted as RFC3339Nano UTC strings. The caller's data fields are
// merged with system fields (id, created_at, updated_at).
func (d *Document) ForWire() map[string]json.RawMessage {
	// The +3 is for system fields: id, created_at, updated_at.
	const systemFields = 3
	const maxTotalCapacity = 1000000

	allocSize := maxTotalCapacity
	if len(d.Data) < maxTotalCapacity-systemFields {
		allocSize = len(d.Data) + systemFields
	}

	out := make(map[string]json.RawMessage, allocSize)
	for k, v := range d.Data {
		out[k] = v
	}
	out["id"], _ = json.Marshal(d.ID)
	out["created_at"], _ = json.Marshal(d.CreatedAt.UTC().Format(time.RFC3339Nano))
	out["updated_at"], _ = json.Marshal(d.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return out
}

// DocFilter represents a single filter condition for DocQuery.
// Op must be one of: ==, !=, <, >, <=, >=
type DocFilter struct {
	Field string          `json:"field"`
	Op    string          `json:"op"`
	Value json.RawMessage `json:"value"`
}

// DocQueryRequest is the typed body for POST /db/{collection}/_query.
type DocQueryRequest struct {
	Filters []DocFilter `json:"filters,omitempty"`
	OrderBy string      `json:"order_by,omitempty"`
	Limit   int         `json:"limit,omitempty"`
}

// KVSetRequest is the typed body for PUT /kv/{key}.
type KVSetRequest struct {
	Value string `json:"value"`
	TTL   int    `json:"ttl,omitempty"`
}

// KVExpireRequest is the typed body for PUT /kv/{key}/_expire.
type KVExpireRequest struct {
	TTL int `json:"ttl"`
}

// KVPatternRequest is the typed body for POST /kv/_keys, /kv/_scan, /kv/_delete_pattern.
type KVPatternRequest struct {
	Pattern string `json:"pattern,omitempty"`
	Cursor  int    `json:"cursor,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// PubSubPublishRequest is the typed body for POST /pubsub/publish.
type PubSubPublishRequest struct {
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

// HealthResponse is the typed response for GET /health.
type HealthResponse struct {
	Status          constants.GatewayMode `json:"status"`
	Mode            constants.GatewayMode `json:"mode"`
	Version         string                `json:"version"`
	GovernanceReady bool                  `json:"governance_ready"`
	StateMerkleRoot string                `json:"state_merkle_root,omitempty"`
}

// StatusResponse is the typed response for simple ok/error replies.
type StatusResponse struct {
	Status constants.GatewayMode `json:"status"`
}

// KVGetResponse is the typed response for GET /kv/{key}.
type KVGetResponse struct {
	Value string `json:"value"`
}

// KVTTLResponse is the typed response for GET /kv/{key}/_ttl.
type KVTTLResponse struct {
	TTL int `json:"ttl"`
}

// KVKeysResponse is the typed response for POST /kv/_keys.
type KVKeysResponse struct {
	Keys []string `json:"keys"`
}

// KVScanResponse is the typed response for POST /kv/_scan.
type KVScanResponse struct {
	Cursor int      `json:"cursor"`
	Keys   []string `json:"keys"`
}

// KVDeletePatternResponse is the typed response for POST /kv/_delete_pattern.
type KVDeletePatternResponse struct {
	Deleted int64 `json:"deleted"`
}

// PubSubPublishResponse is the typed response for POST /pubsub/publish.
type PubSubPublishResponse struct {
	Receivers int `json:"receivers"`
}

type ActionReceiptRecord struct {
	TransactionID     string                     `json:"transaction_id"`
	TransactionHash   string                     `json:"transaction_hash"`
	OperatorID        string                     `json:"operator_id"`
	OperatorSessionID string                     `json:"operator_session_id"`
	ActionType        constants.ActionType       `json:"action_type"`
	TargetResource    string                     `json:"target_resource"`
	Status            operatorv1.ExecutionStatus `json:"status"`
	ResultSummary     string                     `json:"result_summary"`
	StateRootBefore   string                     `json:"state_root_before"`
	StateRootAfter    string                     `json:"state_root_after"`
	ExecutedAt        time.Time                  `json:"executed_at"`
	SignerKeyID       string                     `json:"signer_key_id"`
	Signature         string                     `json:"signature"`
	ImplicitL2        bool                       `json:"implicit_l2_signature"`
	L2Valid           bool                       `json:"l2_valid"`
	L3Valid           bool                       `json:"l3_valid"`
	Timestamp         time.Time                  `json:"timestamp"`
}

// BlobMetaResponse is the typed response for GET /blob/{namespace}/{id}/meta.
type BlobMetaResponse struct {
	ID          string    `json:"id"`
	Namespace   string    `json:"namespace"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}

// BlobDeleteResponse is the typed response for DELETE /blob/{namespace}/{id} and DELETE /blob/{namespace}.
type BlobDeleteResponse struct {
	Deleted int64 `json:"deleted"`
}

// SSEEventRow is a single row from the sse_events table. Exactly one of the
// three routing id fields will be populated per row.
type SSEEventRow struct {
	ID           int64  `json:"id"`
	WebSessionID string `json:"web_session_id,omitempty"`
	CLISessionID string `json:"cli_session_id,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	EventType    string `json:"event_type"`
	Payload      string `json:"payload"`
	CreatedAt    string `json:"created_at"`
}

// SSEPushResponse is the typed response for POST /api/internal/sse/push.
type SSEPushResponse struct {
	Success   bool `json:"success"`
	Delivered int  `json:"delivered"`
}

// SSEEventsResponse is the typed response for GET /api/internal/sse/events.
type SSEEventsResponse struct {
	Events []SSEEventRow `json:"events"`
	Count  int           `json:"count"`
}

// ReauthResponse is the typed response for POST /api/operators/reauth.
type ReauthResponse struct {
	Success  bool                `json:"success"`
	Operator *OperatorDocumentGo `json:"operator"`
}

// AuditReceiptsResponse is the typed response for GET /api/audit/receipts.
type AuditReceiptsResponse struct {
	Success  bool                   `json:"success"`
	Receipts []*ActionReceiptRecord `json:"receipts"`
}

// TrustedSignersResponse is the typed response for GET /api/governance/signers.
type TrustedSignersResponse struct {
	Success bool            `json:"success"`
	Signers []TrustedSigner `json:"signers"`
}

// PasskeyChallengeResponse is the typed response for passkey challenge endpoints.
type PasskeyChallengeResponse struct {
	Success    bool                          `json:"success"`
	Options    *protocol.CredentialAssertion `json:"options,omitempty"`
	NeedsSetup bool                          `json:"needs_setup,omitempty"`
	Error      string                        `json:"error,omitempty"`
}

// PasskeyVerifyResponse is the typed response for passkey verify endpoints.
type PasskeyVerifyResponse struct {
	Success      bool               `json:"success"`
	Credential   *PasskeyCredential `json:"credential,omitempty"`
	UserID       string             `json:"user_id,omitempty"`
	WebSessionID string             `json:"web_session_id,omitempty"`
	Error        string             `json:"error,omitempty"`
}

// PasskeyCredentialsResponse is the typed response for GET /api/auth/passkey/credentials.
type PasskeyCredentialsResponse struct {
	Success     bool                `json:"success"`
	Credentials []PasskeyCredential `json:"credentials"`
}

// PasskeyRevokeResponse is the typed response for DELETE /api/auth/passkey/credentials/{id}.
type PasskeyRevokeResponse struct {
	Success   bool `json:"success"`
	Found     bool `json:"found"`
	Remaining int  `json:"remaining"`
}

// AuthLoginChallengeResponse is the typed response for POST /api/auth/login/challenge.
type AuthLoginChallengeResponse struct {
	Success bool                          `json:"success"`
	UserID  string                        `json:"user_id,omitempty"`
	Options *protocol.CredentialAssertion `json:"options,omitempty"`
}

// AuthLoginVerifyResponse is the typed response for POST /api/auth/login/verify.
type AuthLoginVerifyResponse struct {
	Success      bool   `json:"success"`
	UserID       string `json:"user_id,omitempty"`
	WebSessionID string `json:"web_session_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

// BootstrapStatusResponse is the typed response for GET /api/auth/bootstrap/status.
type BootstrapStatusResponse struct {
	Bootstrapped bool `json:"bootstrapped"`
}

// UserMeResponse is the typed response for GET /api/user/me.
type UserMeResponse struct {
	Success bool  `json:"success"`
	User    *User `json:"user"`
}

// WebSessionResponse is the typed response for GET /api/auth/web-session.
type WebSessionResponse struct {
	Success      bool   `json:"success"`
	UserID       string `json:"user_id"`
	WebSessionID string `json:"web_session_id"`
}

// SettingsDocument represents the platform_settings document structure.
// Authority: protocol/models/platform_settings.json
type SettingsDocument struct {
	Settings  map[string]interface{} `json:"settings"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// UserSettingsDocument represents the user_settings document structure.
// Authority: protocol/models/user_settings.json
type UserSettingsDocument struct {
	Settings  map[string]interface{} `json:"settings"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}
