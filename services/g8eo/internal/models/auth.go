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
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// DeviceLinkData represents a device link for operator enrollment.
// Authority: services/client/models/auth_models.js::DeviceLinkData
type DeviceLinkData struct {
	Token          string                     `json:"token"`
	UserID         string                     `json:"user_id"`
	OrganizationID string                     `json:"organization_id,omitempty"`
	OperatorID     string                     `json:"operator_id,omitempty"`
	WebSessionID   string                     `json:"web_session_id,omitempty"`
	Name           string                     `json:"name,omitempty"`
	MaxUses        int                        `json:"max_uses"`
	Uses           int                        `json:"uses"`
	Status         constants.DeviceLinkStatus `json:"status"`
	CreatedAt      time.Time                  `json:"created_at"`
	ExpiresAt      time.Time                  `json:"expires_at"`
	UsedAt         *time.Time                 `json:"used_at,omitempty"`
	RevokedAt      *time.Time                 `json:"revoked_at,omitempty"`
	DeviceInfo     *DeviceLinkInfo            `json:"device_info,omitempty"`
	Claims         []DeviceLinkClaim          `json:"claims,omitempty"`
}

type CreateDeviceLinkRequest struct {
	UserID         string `json:"user_id"`
	Email          string `json:"email,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`
	OperatorID     string `json:"operator_id,omitempty"`
	WebSessionID   string `json:"web_session_id,omitempty"`
	Name           string `json:"name,omitempty"`
	MaxUses        int    `json:"max_uses,omitempty"`
	TTLSeconds     int    `json:"ttl_seconds,omitempty"`
}

type DeviceLinkResponse struct {
	Success         bool      `json:"success"`
	Token           string    `json:"token"`
	UserID          string    `json:"user_id,omitempty"`
	OperatorCommand string    `json:"operator_command"`
	Name            string    `json:"name,omitempty"`
	MaxUses         int       `json:"max_uses,omitempty"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type DeviceLinkListItem struct {
	Token     string                     `json:"token"`
	Name      string                     `json:"name,omitempty"`
	MaxUses   int                        `json:"max_uses"`
	Uses      int                        `json:"uses"`
	Status    constants.DeviceLinkStatus `json:"status"`
	CreatedAt time.Time                  `json:"created_at"`
	ExpiresAt time.Time                  `json:"expires_at"`
}

type DeviceLinkListResponse struct {
	Success bool                 `json:"success"`
	Links   []DeviceLinkListItem `json:"links"`
}

// DeviceLinkInfo captures the device details of the first user of a link.
type DeviceLinkInfo struct {
	SystemFingerprint string `json:"system_fingerprint"`
	Hostname          string `json:"hostname"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Username          string `json:"username"`
}

// DeviceLinkClaim records which operator ID claimed a slot via a multi-use link.
type DeviceLinkClaim struct {
	SystemFingerprint string    `json:"system_fingerprint"`
	Hostname          string    `json:"hostname"`
	OperatorID        string    `json:"operator_id"`
	ClaimedAt         time.Time `json:"claimed_at"`
}

// OperatorRegistrationRequest is the inbound body for /api/auth/device-link/register.
type OperatorRegistrationRequest struct {
	CSR               string `json:"csr_pem"`
	CLICSR            string `json:"cli_csr_pem,omitempty"`
	SystemFingerprint string `json:"system_fingerprint"`
	Hostname          string `json:"hostname"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Username          string `json:"username"`
	IPAddress         string `json:"ip_address,omitempty"`
}

// OperatorRegistrationResponse is the response for /api/auth/device-link/register.
//
// OperatorSessionID and CLISessionID are strictly disjoint session types:
//   - operator_session_id authenticates the host agent and is bound to the
//     mTLS certificate URI SAN (see protocol.WorkloadIdentity.OperatorSPIFFEID).
//   - cli_session_id is the routing namespace the BYO/CLI client uses to
//     receive SessionEvents (SSE) and embed in outbound request bodies.
//     The CLI has its own distinct mTLS certificate with SPIFFE ID
//     spiffe://g8e.local/cli/<user_id>/<cli_session_id> (see protocol.WorkloadIdentity.CLISPIFFEID).
//
// Conflating the two would let an operator session drain another client's
// event stream — the substrate refuses to do so.
type OperatorRegistrationResponse struct {
	Success                bool            `json:"success"`
	OperatorSessionID      string          `json:"operator_session_id,omitempty"`
	CLISessionID           string          `json:"cli_session_id,omitempty"`
	OperatorID             string          `json:"operator_id,omitempty"`
	OperatorCert           string          `json:"operator_cert,omitempty"`
	OperatorCertChain      string          `json:"operator_cert_chain,omitempty"`
	CLICert                string          `json:"cli_cert,omitempty"`
	CLICertChain           string          `json:"cli_cert_chain,omitempty"`
	HubTrustBundle         string          `json:"hub_trust_bundle,omitempty"`
	OperatorSessionSummary *SessionSummary `json:"operator_session_summary,omitempty"`
	Config                 json.RawMessage `json:"config,omitempty"`
	Error                  string          `json:"error,omitempty"`
}

// SessionSummary provides a brief overview of the created operator session.
type SessionSummary struct {
	OperatorSessionID string    `json:"operator_session_id"`
	ExpiresAt         time.Time `json:"expires_at"`
	CreatedAt         time.Time `json:"created_at"`
}

// OperatorDocumentGo is a Go representation of the canonical OperatorDocument.
// Authority: protocol/models/operator_document.json
type OperatorDocumentGo struct {
	ID                   string                   `json:"id"`
	UserID               string                   `json:"user_id"`
	OrganizationID       string                   `json:"organization_id,omitempty"`
	Component            constants.ComponentName  `json:"component"`
	Name                 string                   `json:"name,omitempty"`
	Status               constants.OperatorStatus `json:"status"`
	OperatorSessionID    string                   `json:"operator_session_id,omitempty"`
	BoundWebSessionID    string                   `json:"bound_web_session_id,omitempty"`
	APIKey               string                   `json:"api_key,omitempty"`
	OperatorAPIKey       string                   `json:"operator_api_key,omitempty"`
	OperatorCert         string                   `json:"operator_cert,omitempty"`
	OperatorCertSerial   string                   `json:"operator_cert_serial,omitempty"`
	SlotNumber           int                      `json:"slot_number,omitempty"`
	IsSlot               bool                     `json:"is_slot"`
	Claimed              bool                     `json:"claimed"`
	OperatorType         constants.OperatorType   `json:"operator_type,omitempty"`
	CloudSubtype         constants.CloudSubtype   `json:"cloud_subtype,omitempty"`
	SystemFingerprint    string                   `json:"system_fingerprint,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
	StartedAt            *time.Time               `json:"started_at,omitempty"`
	ClaimedAt            *time.Time               `json:"claimed_at,omitempty"`
	LatestHeartbeat      json.RawMessage          `json:"latest_heartbeat_snapshot,omitempty"`
	RuntimeConfig        *RuntimeConfig           `json:"runtime_config,omitempty"`
	ConsumedByOperatorID string                   `json:"consumed_by_operator_id,omitempty"`
}

// MarshalJSON implements json.Marshaler with default enum values.
// Ensures OperatorType and CloudSubtype are defaulted before serialization
// to eliminate the need for coercion logic in downstream consumers (e.g., Python engine).
func (o *OperatorDocumentGo) MarshalJSON() ([]byte, error) {
	type Alias OperatorDocumentGo
	defaulted := &struct {
		*Alias
	}{
		Alias: (*Alias)(o),
	}

	// Apply defaults for enum fields
	if defaulted.OperatorType == "" {
		defaulted.OperatorType = constants.Status.OperatorType.System
	}
	// CloudSubtype defaults to empty string (no default subtype)

	return json.Marshal(defaulted)
}

type OperatorSlotResponse struct {
	Success   bool                 `json:"success"`
	Operators []OperatorDocumentGo `json:"operators"`
}

type RotateAPIKeyRequest struct {
	OperatorID string `json:"operator_id"`
}

type RotateAPIKeyResponse struct {
	Success bool `json:"success"`
}

type TerminateOperatorRequest struct {
	OperatorID string `json:"operator_id"`
	UserID     string `json:"user_id"`
	Reason     string `json:"reason,omitempty"`
}

type TerminateOperatorResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// BindOperatorsRequest is the inbound body for /api/operators/bind
type BindOperatorsRequest struct {
	OperatorIDs  []string `json:"operator_ids"`
	UserID       string   `json:"user_id"`
	WebSessionID string   `json:"web_session_id"`
}

// BindOperatorsResponse is the response for /api/operators/bind
type BindOperatorsResponse struct {
	Success           bool     `json:"success"`
	BoundCount        int      `json:"bound_count"`
	FailedCount       int      `json:"failed_count"`
	BoundOperatorIDs  []string `json:"bound_operator_ids"`
	FailedOperatorIDs []string `json:"failed_operator_ids"`
	Error             string   `json:"error,omitempty"`
}

// UnbindOperatorsRequest is the inbound body for /api/operators/unbind
type UnbindOperatorsRequest struct {
	OperatorIDs  []string `json:"operator_ids"`
	UserID       string   `json:"user_id"`
	WebSessionID string   `json:"web_session_id"`
}

// UnbindOperatorsResponse is the response for /api/operators/unbind
type UnbindOperatorsResponse struct {
	Success            bool     `json:"success"`
	UnboundCount       int      `json:"unbound_count"`
	FailedCount        int      `json:"failed_count"`
	UnboundOperatorIDs []string `json:"unbound_operator_ids"`
	FailedOperatorIDs  []string `json:"failed_operator_ids"`
	Error              string   `json:"error,omitempty"`
}

// SetTargetContextRequest is the inbound body for /api/operators/target
type SetTargetContextRequest struct {
	OperatorID   string `json:"operator_id"`
	UserID       string `json:"user_id"`
	WebSessionID string `json:"web_session_id"`
}

// SetTargetContextResponse is the response for /api/operators/target
type SetTargetContextResponse struct {
	Success    bool   `json:"success"`
	OperatorID string `json:"operator_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BoundSessionsDocumentGo represents the persisted record of the bidirectional binding
// between a web session and one or more operator sessions.
type BoundSessionsDocumentGo struct {
	ID                 string                   `json:"id"`
	WebSessionID       string                   `json:"web_session_id"`
	UserID             string                   `json:"user_id"`
	OperatorSessionIDs []string                 `json:"operator_session_ids"`
	OperatorIDs        []string                 `json:"operator_ids"`
	BoundAt            time.Time                `json:"bound_at"`
	LastUpdatedAt      time.Time                `json:"last_updated_at"`
	Status             constants.OperatorStatus `json:"status"`
}

// ============================================================================
// Passkey / WebAuthn Models
// ============================================================================

// PasskeyCredential represents a stored WebAuthn credential for a user.
type PasskeyCredential struct {
	ID               []byte                            `json:"id"`
	PublicKey        []byte                            `json:"public_key"`
	AttestationType  string                            `json:"attestation_type"`
	Transport        []protocol.AuthenticatorTransport `json:"transport,omitempty"`
	Authenticator    Authenticator                     `json:"authenticator"`
	CreatedAtUnixMs  int64                             `json:"created_at_unix_ms"`
	LastUsedAtUnixMs int64                             `json:"last_used_at_unix_ms,omitempty"`
}

// Authenticator represents the internal WebAuthn authenticator state.
type Authenticator struct {
	AAGUID       []byte `json:"aaguid"`
	SignCount    uint32 `json:"sign_count"`
	CloneWarning bool   `json:"clone_warning"`
}

// WebAuthnUser implements webauthn.User interface.
func (u *User) WebAuthnID() []byte {
	return []byte(u.ID)
}

func (u *User) WebAuthnName() string {
	return u.Email
}

func (u *User) WebAuthnDisplayName() string {
	return u.Name
}

func (u *User) WebAuthnIcon() string {
	return ""
}

func (u *User) WebAuthnCredentials() []webauthn.Credential {
	res := make([]webauthn.Credential, len(u.PasskeyCredentials))
	for i, c := range u.PasskeyCredentials {
		res[i] = webauthn.Credential{
			ID:              c.ID,
			PublicKey:       c.PublicKey,
			AttestationType: c.AttestationType,
			Transport:       c.Transport,
			Authenticator: webauthn.Authenticator{
				AAGUID:       c.Authenticator.AAGUID,
				SignCount:    c.Authenticator.SignCount,
				CloneWarning: c.Authenticator.CloneWarning,
			},
		}
	}
	return res
}

// WebSession represents an authenticated web session after passkey verification.
type WebSession struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	CreatedAtUnixMs int64  `json:"created_at_unix_ms"`
	ExpiresAtUnixMs int64  `json:"expires_at_unix_ms"`
}

// CLISession represents an authenticated CLI/BYO session.
// Strictly disjoint from operator_session_id.
type CLISession struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	OperatorSessionID string    `json:"operator_session_id"` // Bind to the specific operator session that created it
	SystemFingerprint string    `json:"system_fingerprint,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// User represents a platform user with passkey credentials.
//
// IsBootstrap identifies the ephemeral local-superadmin identity created by
// `./g8e platform start -a` over loopback. It is *not* a privilege tier — it
// marks an identity that exists purely to make a fresh local install usable
// without ceremony, and that is retired automatically the first time a real
// device-link login completes.
type User struct {
	ID                 string              `json:"id"`
	Email              string              `json:"email"`
	Name               string              `json:"name,omitempty"`
	PasskeyCredentials []PasskeyCredential `json:"passkey_credentials,omitempty"`
	Provider           string              `json:"provider,omitempty"`

	Status      constants.UserStatus `json:"status,omitempty"`
	IsBootstrap bool                 `json:"is_bootstrap,omitempty"`
}

// IsActive reports whether the user is permitted to authenticate. Treats the
// zero value as active so pre-existing user docs (written before Status was
// introduced) keep working without a migration step.
func (u *User) IsActive() bool {
	if u == nil {
		return false
	}
	return u.Status == "" || u.Status == constants.UserStatusActive
}

// AdminAuditEntry is a single row in the `auth_admin_audit` collection.
// New admin-side state changes (retire, disable, role mutation, etc.) MUST
// append a row here so the lifecycle is auditable from the protocol substrate.
type AdminAuditEntry struct {
	ID         string                 `json:"id"`
	At         time.Time              `json:"at"`
	Action     string                 `json:"action"`
	Actor      string                 `json:"actor,omitempty"`
	Target     string                 `json:"target,omitempty"`
	OperatorID string                 `json:"operator_id,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// Admin audit action constants. Keep these stable — downstream tooling and
// receipts join on the string value.
const (
	AdminAuditActionRetireLocalSuperadmin = "retire_local_superadmin"
)

// TrustedSigner represents an external L2 signer public key stored in the database.
type TrustedSigner struct {
	ID        string    `json:"id"` // Unique ID for the signer (e.g., agent ID or name)
	PublicKey string    `json:"public_key_hex"`
	AddedAt   time.Time `json:"added_at"`
	Enabled   bool      `json:"enabled"`
}
