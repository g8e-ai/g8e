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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/timesvc"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
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
// Timestamps are formatted as fixed-microsecond UTC strings. The caller's data fields are
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
	out["created_at"], _ = json.Marshal(timesvc.FormatTimestamp(d.CreatedAt))
	out["updated_at"], _ = json.Marshal(timesvc.FormatTimestamp(d.UpdatedAt))
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
	PID             int                   `json:"pid"`
	GovernanceReady bool                  `json:"governance_ready"`
	StateMerkleRoot string                `json:"state_merkle_root,omitempty"`
}

// StateResponse is the typed response for GET /state.
type StateResponse struct {
	StateMerkleRoot string `json:"state_merkle_root"`
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

// SSEPushPayload is the wire envelope for SSE push events. UserID is always
// required (ownership). Exactly one of WebSessionID or CliSessionID must be
// set as the delivery target. Event carries the typed event JSON. This is the
// shared typed model for the wire shape produced by the gateway and consumed
// by CLI clients.
type SSEPushPayload struct {
	UserID       string          `json:"user_id"`
	WebSessionID string          `json:"web_session_id"`
	CliSessionID string          `json:"cli_session_id"`
	Event        json.RawMessage `json:"event"`
}

// SSEEventRow is a single row from the sse_events table. UserID is always
// populated. Exactly one of WebSessionID or CLISessionID will be populated.
type SSEEventRow struct {
	ID           int64  `json:"id"`
	UserID       string `json:"user_id"`
	WebSessionID string `json:"web_session_id,omitempty"`
	CLISessionID string `json:"cli_session_id,omitempty"`
	EventType    string `json:"event_type"`
	Payload      string `json:"payload"`
	CreatedAt    string `json:"created_at"`
}

// SSEPublishedEvent is the internal pub/sub envelope for live SSE events.
// It pairs the persisted DB row ID with the raw event payload JSON so that
// the stream handler can deduplicate against replayed rows and emit an `id:`
// field on the live path (fixing the duplicate-delivery race and the
// missing Last-Event-ID cursor on reconnect).
type SSEPublishedEvent struct {
	ID      int64           `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// SSEErrorEvent is the sentinel payload emitted on the SSE stream when a
// replay query fails, signaling a gap to the client so it can react rather
// than silently missing backlog.
type SSEErrorEvent struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// SSETruncationEvent is the sentinel payload emitted on the SSE stream when
// a replay hits the row limit, signaling that more backlog may exist beyond
// the returned window.
type SSETruncationEvent struct {
	Type    string `json:"type"`
	SinceID int64  `json:"since_id"`
	Limit   int    `json:"limit"`
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

// SSEEventsCountResponse is the typed response for GET /api/internal/sse/events/count.
type SSEEventsCountResponse struct {
	Count int64 `json:"count"`
}

// SSEEventsWipeResponse is the typed response for DELETE /api/internal/sse/events.
type SSEEventsWipeResponse struct {
	Deleted int64 `json:"deleted"`
}

// ReauthResponse is the typed response for POST /api/operators/reauth.
type ReauthResponse struct {
	Success  bool                `json:"success"`
	Operator *OperatorDocumentGo `json:"operator"`
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

// PasskeyRegisterChallengeResponse is the typed response for POST /api/auth/passkeys/register/challenge.
type PasskeyRegisterChallengeResponse struct {
	Success bool                         `json:"success"`
	UserID  string                       `json:"user_id,omitempty"`
	Options *protocol.CredentialCreation `json:"options,omitempty"`
}

// PasskeyAuthVerifyResponse is the typed response for POST /api/auth/passkeys/authenticate/verify.
type PasskeyAuthVerifyResponse struct {
	Success    bool               `json:"success"`
	UserID     string             `json:"user_id,omitempty"`
	Credential *PasskeyCredential `json:"credential,omitempty"`
	Error      string             `json:"error,omitempty"`
	WebSession *WebSessionInfo    `json:"web_session,omitempty"`
}

// WebSessionInfo contains web session details.
type WebSessionInfo struct {
	ID              string `json:"id"`
	ExpiresAtUnixMs int64  `json:"expires_at_unix_ms"`
}

// UserCreateResponse is the typed response for POST /api/users.
type UserCreateResponse struct {
	Success bool   `json:"success"`
	UserID  string `json:"user_id"`
}

// SuspendedTransactionsResponse is the typed response for GET /api/approvals.
type SuspendedTransactionsResponse struct {
	Transactions []SuspendedTxResponse `json:"transactions"`
}

// SuspendedTxResponse represents a suspended transaction.
type SuspendedTxResponse struct {
	TransactionHash string    `json:"transaction_hash"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	ToolName        string    `json:"tool_name"`
	UserID          string    `json:"user_id"`
	OperatorID      string    `json:"operator_id"`
}

// ApprovalStatusResponse is the typed response for GET /api/v1/approvals/status/{txHash}.
// The CLI polls this mTLS-authenticated endpoint after opening the browser approval page.
type ApprovalStatusResponse struct {
	Status   string `json:"status"`
	TxHash   string `json:"tx_hash,omitempty"`
	ToolName string `json:"tool_name,omitempty"`
}

// BootstrapResponse is the typed response for POST /api/auth/bootstrap.
type BootstrapResponse struct {
	Success           bool            `json:"success"`
	User              *User           `json:"user,omitempty"`
	UserID            string          `json:"user_id,omitempty"`
	WebSession        *WebSessionInfo `json:"web_session,omitempty"`
	OperatorCert      string          `json:"operator_cert,omitempty"`
	OperatorCertChain string          `json:"operator_cert_chain,omitempty"`
	OperatorSessionID string          `json:"operator_session_id,omitempty"`
	OperatorID        string          `json:"operator_id,omitempty"`
	CLISessionID      string          `json:"cli_session_id,omitempty"`
	CLICert           string          `json:"cli_cert,omitempty"`
	CLICertChain      string          `json:"cli_cert_chain,omitempty"`
	HubTrustBundle    string          `json:"hub_trust_bundle,omitempty"`
}

// CLIEnrollmentResponse is the typed response for POST /api/auth/cli/enroll.
type CLIEnrollmentResponse struct {
	Success           bool   `json:"success"`
	CLISessionID      string `json:"cli_session_id"`
	CLICert           string `json:"cli_cert"`
	CLICertChain      string `json:"cli_cert_chain"`
	HubTrustBundle    string `json:"hub_trust_bundle"`
	UserID            string `json:"user_id"`
	OperatorSessionID string `json:"operator_session_id,omitempty"`
	OperatorID        string `json:"operator_id,omitempty"`
}

// DeviceEnrollmentResponse is the typed response for POST /api/auth/device/enroll.
type DeviceEnrollmentResponse struct {
	Success           bool   `json:"success"`
	User              *User  `json:"user,omitempty"`
	OperatorCert      string `json:"operator_cert"`
	OperatorCertChain string `json:"operator_cert_chain"`
	HubTrustBundle    string `json:"hub_trust_bundle"`
	OperatorSessionID string `json:"operator_session_id"`
	OperatorID        string `json:"operator_id"`
	CLISessionID      string `json:"cli_session_id"`
	CLICert           string `json:"cli_cert"`
	CLICertChain      string `json:"cli_cert_chain"`
	UserID            string `json:"user_id"`
	ActuatorKeyID     string `json:"actuator_key_id,omitempty"`
	ActuatorPubKey    string `json:"actuator_pub_key,omitempty"`
	Error             string `json:"error,omitempty"`
}

// ActuatorPublicKeyExport is the typed JSON structure for exporting the Actuator's public key.
type ActuatorPublicKeyExport struct {
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
	Algorithm string `json:"algorithm"`
}

// PKIFingerprintResponse is the typed response for GET /.well-known/g8e/pki/fingerprint.
type PKIFingerprintResponse struct {
	RootCA string `json:"root_ca"`
}

// PKICSRSignResponse is the typed response for POST /.well-known/g8e/pki/csr/sign.
type PKICSRSignResponse struct {
	CertificatePEM      string `json:"certificate_pem"`
	CertificateChainPEM string `json:"certificate_chain_pem"`
}

// PlatformSettings represents the typed settings within platform_settings.
// Authority: protocol/models/platform_settings.json
type PlatformSettings struct {
	ActuatorKeyID             string `json:"actuator_key_id"`
	OperatorSessionID         string `json:"operator_session_id,omitempty"`
	LLMCommandGenEnabled      bool   `json:"llm_command_gen_enabled,omitempty"`
	LLMCommandGenVerifier     bool   `json:"llm_command_gen_verifier,omitempty"`
	LLMCommandGenPasses       int    `json:"llm_command_gen_passes,omitempty"`
	GoogleSearchEnabled       bool   `json:"google_search_enabled,omitempty"`
	GoogleSearchAPIKey        string `json:"google_search_api_key,omitempty"`
	GoogleSearchEngineID      string `json:"google_search_engine_id,omitempty"`
	VertexSearchEnabled       bool   `json:"vertex_search_enabled,omitempty"`
	VertexSearchProjectID     string `json:"vertex_search_project_id,omitempty"`
	VertexSearchEngineID      string `json:"vertex_search_engine_id,omitempty"`
	VertexSearchLocation      string `json:"vertex_search_location,omitempty"`
	VertexSearchAPIKey        string `json:"vertex_search_api_key,omitempty"`
	EnableCommandWhitelisting bool   `json:"enable_command_whitelisting,omitempty"`
	EnableCommandBlacklisting bool   `json:"enable_command_blacklisting,omitempty"`
	PasskeyRPName             string `json:"passkey_rp_name,omitempty"`
	PasskeyRPID               string `json:"passkey_rp_id,omitempty"`
	PasskeyOrigin             string `json:"passkey_origin,omitempty"`
	AppURL                    string `json:"app_url,omitempty"`
	AllowedOrigins            string `json:"allowed_origins,omitempty"`
	SupervisorPort            int    `json:"supervisor_port,omitempty"`
}

// SettingsDocument represents the platform_settings document structure.
// Authority: protocol/models/platform_settings.json
type SettingsDocument struct {
	Settings  *PlatformSettings `json:"settings"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// UserSettingsDocument represents the user_settings document structure.
// Authority: protocol/models/user_settings.json
type UserSettingsDocument struct {
	Settings  map[string]interface{} `json:"settings"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}
