// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"encoding/json"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/uuid"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// OperatorRegistrationRequest is the inbound body for /api/pki/device-enroll (CSR-based enrollment).
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

// BootstrapRequest is the inbound body for /api/v1/auth/bootstrap.
type BootstrapRequest struct {
	CSR               string       `json:"csr_pem"`
	CLICSR            string       `json:"cli_csr_pem,omitempty"`
	SystemFingerprint string       `json:"system_fingerprint"`
	LocalOSUser       *LocalOSUser `json:"local_os_user,omitempty"`
}

// CLIEnrollRequest is the inbound body for /api/v1/auth/cli/enroll.
type CLIEnrollRequest struct {
	CLICSR            string       `json:"cli_csr_pem"`
	SystemFingerprint string       `json:"system_fingerprint"`
	LocalOSUser       *LocalOSUser `json:"local_os_user,omitempty"`
}

// AppEnrollRequest is the request body for external app enrollment via /api/v1/pki/apps/delegated.
type AppEnrollRequest struct {
	CSR            string `json:"csr_pem"`
	AppName        string `json:"app_name"`
	AppType        string `json:"app_type"`
	OrganizationID string `json:"organization_id,omitempty"`
}

// AppEnrollResponse is the response body for external app enrollment.
type AppEnrollResponse struct {
	Success     bool   `json:"success"`
	AppCert     string `json:"app_cert"`
	CertChain   string `json:"cert_chain"`
	TrustBundle string `json:"trust_bundle"`
	AppID       string `json:"app_id"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

// PasskeyChallengeRequest is the inbound body for passkey authentication challenge endpoints.
type PasskeyChallengeRequest struct {
	UserID string `json:"user_id"`
}

// PasskeyRegisterChallengeRequest is the inbound body for passkey registration challenge endpoints.
type PasskeyRegisterChallengeRequest struct {
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
}

// OperatorRegistrationResponse is the response for /api/pki/device-enroll (CSR-based enrollment).
//
// OperatorSessionID and CLISessionID are strictly disjoint session types:
//   - operator_session_id authenticates the host agent and is bound to the
//     mTLS certificate URI SAN (see protocol.WorkloadIdentity.OperatorSPIFFEID).
//   - cli_session_id is the routing namespace the BYO/CLI client uses to
//     receive SessionEvents (SSE) and embed in outbound request bodies.
//     The CLI has its own distinct mTLS certificate with SPIFFE ID
//     spiffe://g8e.local/cli/<user_id>/<cli_session_id> (see protocol.WorkloadIdentity.CLISPIFFEID).
//
// Conflating the two would let an Operator session drain another client's
// event stream - the Gateway refuses to do so.
type OperatorRegistrationResponse struct {
	Success                bool            `json:"success"`
	UserID                 string          `json:"user_id,omitempty"`
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

// SessionSummary provides a brief overview of the created Operator session.
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
// to eliminate the need for coercion logic in downstream consumers (e.g., Python agent).
func (o *OperatorDocumentGo) MarshalJSON() ([]byte, error) {
	type Alias OperatorDocumentGo
	defaulted := &struct {
		*Alias
	}{
		Alias: (*Alias)(o),
	}

	// Apply defaults for enum fields
	if defaulted.OperatorType == "" {
		defaulted.OperatorType = constants.OperatorTypeSystem
	}
	// CloudSubtype defaults to empty string (no default subtype)

	return json.Marshal(defaulted)
}

type OperatorSlotResponse struct {
	Success   bool                 `json:"success"`
	Operators []OperatorDocumentGo `json:"operators"`
}

type OperatorResponse struct {
	Success  bool                `json:"success"`
	Operator *OperatorDocumentGo `json:"operator,omitempty"`
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
// between a web session and one or more Operator sessions.
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

// validAttestationTypes is the set of WebAuthn attestation conveyance preferences
// that may be stored on a credential.
var validAttestationTypes = map[string]bool{
	"none":       true,
	"indirect":   true,
	"direct":     true,
	"enterprise": true,
}

// Validate checks that a PasskeyCredential has well-formed fields before it is
// persisted to disk. It verifies:
//   - ID is non-empty and within the WebAuthn spec limit of 1024 bytes
//   - PublicKey is non-empty and parses as a valid CBOR-encoded COSE key
//   - AttestationType is one of the known values
//   - CreatedAtUnixMs is non-zero
func (c PasskeyCredential) Validate() error {
	if len(c.ID) == 0 {
		return constants.ErrPasskeyCredentialInvalidID
	}
	if len(c.ID) > 1024 {
		return constants.ErrPasskeyCredentialIDTooLong
	}
	if len(c.PublicKey) == 0 {
		return constants.ErrPasskeyCredentialInvalidPublicKey
	}
	var coseKey map[int]any
	if err := cbor.Unmarshal(c.PublicKey, &coseKey); err != nil {
		return constants.ErrPasskeyCredentialInvalidPublicKey
	}
	if len(coseKey) == 0 {
		return constants.ErrPasskeyCredentialInvalidPublicKey
	}
	if !validAttestationTypes[c.AttestationType] {
		return constants.ErrPasskeyCredentialInvalidAttestation
	}
	if c.CreatedAtUnixMs == 0 {
		return constants.ErrPasskeyCredentialInvalidTimestamp
	}
	return nil
}

// Authenticator represents the internal WebAuthn authenticator state.
type Authenticator struct {
	AAGUID       []byte `json:"aaguid"`
	SignCount    uint32 `json:"sign_count"`
	CloneWarning bool   `json:"clone_warning"`
}

// WebAuthnUser implements webauthn.User interface.
func (u *User) WebAuthnID() []byte {
	guidBytes, err := uuid.Parse(u.WebAuthnUserID)
	if err != nil {
		return nil
	}
	return guidBytes[:]
}

func (u *User) WebAuthnName() string {
	// For Windows Hello, use the OS username as the WebAuthn identifier
	if u.LocalOSUser != nil && u.LocalOSUser.Username != "" {
		return u.LocalOSUser.Username
	}
	// Zero-PII: Use user ID as the WebAuthn identifier instead of email
	return u.ID
}

func (u *User) WebAuthnDisplayName() string {
	// For Windows Hello, use the OS username as the display name
	if u.LocalOSUser != nil && u.LocalOSUser.Username != "" {
		return u.LocalOSUser.Username
	}
	// Zero-PII: Use user ID as the display name instead of name
	return u.ID
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

// WebSession represents an authenticated web session.
// Can be created via passkey verification or mTLS certificate (e.g., Windows Certificate Store).
type WebSession struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	CreatedAtUnixMs int64  `json:"created_at_unix_ms"`
	ExpiresAtUnixMs int64  `json:"expires_at_unix_ms"`
	// mTLS certificate fields for Windows Certificate Store enrollment
	OperatorSessionID string `json:"operator_session_id,omitempty"` // Bind to Operator session for mTLS cert auth
	CertFingerprint   string `json:"cert_fingerprint,omitempty"`    // SHA-256 fingerprint of mTLS certificate
	CertSerial        string `json:"cert_serial,omitempty"`         // Serial number for revocation checking
	UserAgent         string `json:"user_agent,omitempty"`          // Browser user agent for tracking
	LoginMethod       string `json:"login_method,omitempty"`        // "passkey", "windows_cert_store", "p12_import", etc.
}

// OperatorSession represents an authenticated Operator session.
// Operator sessions authenticate the host agent via mTLS URI SAN and are used
// by g8e-compatible agentic ensembles to look up sessions by ID.
// Authority: protocol/constants/collections.json (operator_sessions)
type OperatorSession struct {
	ID                string `json:"id"`
	SessionType       string `json:"session_type"`
	UserID            string `json:"user_id"`
	OrganizationID    string `json:"organization_id"`
	OperatorID        string `json:"operator_id"`
	IsActive          bool   `json:"is_active"`
	CreatedAt         string `json:"created_at"`
	AbsoluteExpiresAt string `json:"absolute_expires_at"`
	IdleExpiresAt     string `json:"idle_expires_at"`
	LastActivity      string `json:"last_activity"`
	LoginMethod       string `json:"login_method"`
}

// CLISession represents an authenticated CLI/BYO session.
// Strictly disjoint from operator_session_id.
type CLISession struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	OperatorSessionID string    `json:"operator_session_id"` // Bind to the specific Operator session that created it
	SystemFingerprint string    `json:"system_fingerprint,omitempty"`
	CertFingerprint   string    `json:"cert_fingerprint,omitempty"` // SHA-256 fingerprint of the mTLS certificate
	CertSerial        string    `json:"cert_serial,omitempty"`      // Serial number for revocation checking
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	AbsoluteExpiresAt time.Time `json:"absolute_expires_at"`
	IdleExpiresAt     time.Time `json:"idle_expires_at"`
	SessionType       string    `json:"session_type"`
	IsActive          bool      `json:"is_active"`
	LoginMethod       string    `json:"login_method"`
}

// LocalOSUser represents local OS user account information.
type LocalOSUser struct {
	Domain   string `json:"domain,omitempty"`
	Username string `json:"username,omitempty"`
	UID      string `json:"uid,omitempty"`
	GID      string `json:"gid,omitempty"`
	SID      string `json:"sid,omitempty"`
}

// User represents a platform user with passkey credentials.
//
// Zero-PII Architecture: This struct contains NO personally identifiable information.
// The platform persists only the public key (passkey credentials).
// Email and name are NOT stored - users are identified solely by their
// cryptographic credentials.
//
// The first user created via `auth enroll user` (POST /api/v1/auth/bootstrap)
// is the gateway owner and admin. Admin authorization is enforced via
// UserService.IsFirstUser (the first human enrollee is the admin); there is
// no ephemeral bootstrap-user concept.
type User struct {
	ID                 string              `json:"id"`
	PasskeyCredentials []PasskeyCredential `json:"passkey_credentials,omitempty"`
	Provider           string              `json:"provider,omitempty"`

	OrganizationID string   `json:"organization_id,omitempty"`
	Roles          []string `json:"roles,omitempty"`

	Status constants.UserStatus `json:"status,omitempty"`

	LocalOSUser    *LocalOSUser `json:"local_os_user,omitempty"`
	WebAuthnUserID string       `json:"webauthn_user_id,omitempty"` // GUID for WebAuthn v4 compliance (Windows Hello)
}

// IsActive reports whether the user is permitted to authenticate.
func (u *User) IsActive() bool {
	if u == nil {
		return false
	}
	return u.Status == constants.UserStatusActive
}

// AdminAuditEntry is a single row in the `auth_admin_audit` collection.
// New admin-side state changes (retire, disable, role mutation, etc.) MUST
// append a row here so the lifecycle is auditable from the protocol Gateway.
type AdminAuditEntry struct {
	ID         string             `json:"id"`
	At         time.Time          `json:"at"`
	Action     string             `json:"action"`
	Actor      string             `json:"actor,omitempty"`
	Target     string             `json:"target,omitempty"`
	OperatorID string             `json:"operator_id,omitempty"`
	Details    *AdminAuditDetails `json:"details,omitempty"`
}

// AdminAuditDetails represents the typed details field for AdminAuditEntry.
type AdminAuditDetails struct {
	Reason  string `json:"reason,omitempty"`
	Noop    bool   `json:"noop,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// Admin audit action constants. Keep these stable - downstream tooling and
// receipts join on the string value.
const (
	AdminAuditActionRetireLocalOwner = "retire_local_owner"
)

// TrustedSigner represents an external L2 signer public key stored in the database.
type TrustedSigner struct {
	ID        string    `json:"id"` // Unique ID for the signer (e.g., agent ID or name)
	PublicKey string    `json:"public_key_hex"`
	AddedAt   time.Time `json:"added_at"`
	Enabled   bool      `json:"enabled"`
}

// AppPolicy defines the authorization rules for an external application identity.
// Under the Phase 1 fail-closed model, any app lacking an active policy gets deny-all.
type AppPolicy struct {
	AppID                  string    `json:"app_id"`
	OwnerUserID            string    `json:"owner_user_id,omitempty"`
	ApprovedByUserID       string    `json:"approved_by_user_id,omitempty"`
	EnrollmentRequestID    string    `json:"enrollment_request_id,omitempty"`
	CertificateSerial      string    `json:"certificate_serial,omitempty"`
	CertificateFingerprint string    `json:"certificate_fingerprint,omitempty"`
	AllowedCollections     []string  `json:"allowed_collections"`
	AllowedEventTypes      []string  `json:"allowed_event_types"`
	AllowedIntents         []string  `json:"allowed_intents"`
	RateLimitRPS           int       `json:"rate_limit_rps"`
	MaxPayloadBytes        int64     `json:"max_payload_bytes"`
	RequireL3Approval      bool      `json:"require_l3_approval"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// Persona defines a declarative persona manifest for role-based access control.
// Personas map JWT roles to binding personas used in GovernanceEnvelope.
type Persona struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Roles       []string  `json:"roles"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ConsensusPolicy defines a named consensus body (Consensus) for L2 governance.
// A Consensus consists of N member identities (enrolled agentic apps) and
// requires K affirmative distinct signatures to reach quorum.
type ConsensusPolicy struct {
	ID              string    `json:"id"`               // consensus name/id
	MemberAppIDs    []string  `json:"member_app_ids"`   // == TrustedSigner.ID per member
	Quorum          int       `json:"quorum"`           // K affirmative distinct signers required
	RequireDistinct bool      `json:"require_distinct"` // reject duplicate signer keys
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// EnrollmentToken represents a one-time use token for secure passkey enrollment.
// The token is generated by the gateway after successful CLI enrollment and
// is used to authenticate the browser-based passkey registration flow without
// exposing raw user_id and cli_session_id in the URL.
type EnrollmentToken struct {
	Token        string     `json:"token"`                 // Random 32-byte token (hex-encoded)
	UserID       string     `json:"user_id"`               // Associated user ID
	CLISessionID string     `json:"cli_session_id"`        // Associated CLI session ID
	CreatedAt    time.Time  `json:"created_at"`            // Token creation timestamp
	ExpiresAt    time.Time  `json:"expires_at"`            // Token expiration (5 minutes)
	Consumed     bool       `json:"consumed"`              // Whether the token has been used
	ConsumedAt   *time.Time `json:"consumed_at,omitempty"` // When the token was consumed
}

// CLIRecoveryState represents the lifecycle state of a CLI recovery request.
type CLIRecoveryState string

const (
	CLIRecoveryStatePending   CLIRecoveryState = "pending"
	CLIRecoveryStateApproved  CLIRecoveryState = "approved"
	CLIRecoveryStateCompleted CLIRecoveryState = "completed"
	CLIRecoveryStateDenied    CLIRecoveryState = "denied"
	CLIRecoveryStateExpired   CLIRecoveryState = "expired"
)

// CLIRecoveryRequest is the persisted state of a CLI recovery request.
// The gateway stores a hash of the opaque token (never the raw token),
// the CSR public-key fingerprint for proof-of-possession at completion,
// and the approving user ID after the browser approval step.
type CLIRecoveryRequest struct {
	ID                string           `json:"request_id"`                   // Request ID (UUID)
	TokenHash         string           `json:"token_hash"`                   // SHA-256 hash of the opaque token (hex)
	CLICSRPEM         string           `json:"cli_csr_pem"`                  // CSR submitted by the new CLI
	CSRFingerprint    string           `json:"csr_fingerprint"`              // SHA-256 of the CSR public key (hex)
	SystemFingerprint string           `json:"system_fingerprint,omitempty"` // OS fingerprint metadata
	LocalOSUser       *LocalOSUser     `json:"local_os_user,omitempty"`      // OS user metadata
	State             CLIRecoveryState `json:"state"`                        // pending/approved/completed/denied/expired
	ApprovingUserID   string           `json:"approving_user_id,omitempty"`  // User who approved (set on approval)
	CreatedAt         time.Time        `json:"created_at"`                   // Request creation timestamp
	ExpiresAt         time.Time        `json:"expires_at"`                   // Request expiration
	ApprovedAt        *time.Time       `json:"approved_at,omitempty"`        // When approved
	CompletedAt       *time.Time       `json:"completed_at,omitempty"`       // When completed
	DeniedAt          *time.Time       `json:"denied_at,omitempty"`          // When denied
}

// CLIRecoveryRequestRequest is the wire request for POST /api/v1/auth/cli/recovery/request.
type CLIRecoveryRequestRequest struct {
	CLICSRPEM         string       `json:"cli_csr_pem"`
	SystemFingerprint string       `json:"system_fingerprint,omitempty"`
	LocalOSUser       *LocalOSUser `json:"local_os_user,omitempty"`
}

// CLIRecoveryRequestResponse is the wire response for POST /api/v1/auth/cli/recovery/request.
type CLIRecoveryRequestResponse struct {
	Success     bool      `json:"success"`
	RequestID   string    `json:"request_id"`
	Token       string    `json:"token"`        // Opaque one-time token (only returned once)
	ApprovalURL string    `json:"approval_url"` // Console URL for browser approval
	ExpiresAt   time.Time `json:"expires_at"`
}

// CLIRecoveryStatusRequest is the wire request for GET /api/v1/auth/cli/recovery/status.
// The token is passed as a query parameter.
type CLIRecoveryStatusResponse struct {
	Success bool             `json:"success"`
	State   CLIRecoveryState `json:"state"`
}

// CLIRecoveryApproveRequest is the wire request for POST /api/v1/auth/cli/recovery/approve.
// This is called from the browser console by an authenticated user.
type CLIRecoveryApproveRequest struct {
	Token   string `json:"token"`   // The opaque token from the URL fragment
	Approve bool   `json:"approve"` // true to approve, false to deny
}

// CLIRecoveryApproveResponse is the wire response for POST /api/v1/auth/cli/recovery/approve.
type CLIRecoveryApproveResponse struct {
	Success bool             `json:"success"`
	State   CLIRecoveryState `json:"state"`
}

// CLIRecoveryCompleteRequest is the wire request for POST /api/v1/auth/cli/recovery/complete.
// The CLI polls this endpoint with the opaque token and a proof-of-possession signature
// over the request ID using the CSR private key.
type CLIRecoveryCompleteRequest struct {
	Token     string `json:"token"`     // Opaque one-time token
	Signature string `json:"signature"` // Base64-encoded signature over the request ID using the CSR private key
}

// CLIRecoveryCompleteResponse is the wire response for POST /api/v1/auth/cli/recovery/complete.
// Only returned after successful approval + proof-of-possession verification.
type CLIRecoveryCompleteResponse struct {
	Success           bool   `json:"success"`
	CLISessionID      string `json:"cli_session_id"`
	CLICert           string `json:"cli_cert"`
	CLICertChain      string `json:"cli_cert_chain"`
	HubTrustBundle    string `json:"hub_trust_bundle"`
	UserID            string `json:"user_id"`
	OperatorSessionID string `json:"operator_session_id,omitempty"`
	OperatorID        string `json:"operator_id,omitempty"`
}

// CLIRotationRequest is the wire request for POST /api/v1/auth/cli/rotate.
// This is an mTLS-protected endpoint; the current user and CLI session are
// derived from the authenticated certificate context.
type CLIRotationRequest struct {
	CLICSRPEM string `json:"cli_csr_pem"`
}

// CLIRotationResponse is the wire response for POST /api/v1/auth/cli/rotate.
type CLIRotationResponse struct {
	Success        bool   `json:"success"`
	CLISessionID   string `json:"cli_session_id"`
	CLICert        string `json:"cli_cert"`
	CLICertChain   string `json:"cli_cert_chain"`
	HubTrustBundle string `json:"hub_trust_bundle"`
	UserID         string `json:"user_id"`
}
