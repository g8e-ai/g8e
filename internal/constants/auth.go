// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package constants defines authentication and authorization constants used across the g8e platform.
// This includes WebAuthn/passkey constants, PKI leaf types, HTTP headers, and context keys.
package constants

import "time"

// Passkey purpose constants define the intended use of a passkey credential.
const (
	// PasskeyPurposeRegister indicates the passkey is being created/registered.
	PasskeyPurposeRegister = "register"
	// PasskeyPurposeAuth indicates the passkey is being used for authentication.
	PasskeyPurposeAuth = "auth"
)

// WebAuthn algorithm and type constants define COSE algorithm identifiers and credential types.
const (
	// WebAuthnTypePublicKey is the credential type for public key credentials.
	WebAuthnTypePublicKey = "public-key"
	// WebAuthnAlgES256 is the COSE algorithm identifier for ECDSA using P-256 and SHA-256 (COSE value -7).
	WebAuthnAlgES256 = -7
	// WebAuthnAlgRS256 is the COSE algorithm identifier for RSASSA-PKCS1-v1_5 using SHA-256 (COSE value -257).
	WebAuthnAlgRS256 = -257
)

// WebAuthn attestation and selection constants define credential policy requirements.
const (
	// WebAuthnAttestationNone indicates no attestation is required.
	WebAuthnAttestationNone = "none"
	// WebAuthnResidentKeyRequired indicates a resident key (discoverable credential) is required.
	WebAuthnResidentKeyRequired = "required"
	// WebAuthnUserVerificationRequired indicates user verification is required for credential usage.
	WebAuthnUserVerificationRequired = "required"
)

// PKI leaf type constants define the types of leaf certificates in the g8e PKI hierarchy.
const (
	// LeafTypeOperator identifies an operator node certificate.
	LeafTypeOperator = "operator"
	// LeafTypeApp identifies an application certificate.
	LeafTypeApp = "app"
	// LeafTypeHub identifies a hub/gateway certificate.
	LeafTypeHub = "hub"
	// LeafTypeCLI identifies a CLI client certificate.
	LeafTypeCLI = "cli"
)

// PKI common-name constants define the subject CNs for the g8e CA hierarchy.
// These are the SSOT for CN matching across gateway certificate generation,
// OS trust-store installation, and stale-anchor enumeration.
const (
	// RootCACommonName is the subject CN of the g8e self-signed root CA anchor.
	RootCACommonName = "g8e Root CA"
)

// HTTP header constants.
const (
	HeaderAccept                        = "Accept"
	HeaderAcceptLanguage                = "Accept-Language"
	HeaderAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	HeaderAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	HeaderAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	HeaderAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	HeaderAccessControlMaxAge           = "Access-Control-Max-Age"
	HeaderAccessControlRequestHeaders   = "Access-Control-Request-Headers"
	HeaderAccessControlRequestMethod    = "Access-Control-Request-Method"
	HeaderAuthorization                 = "Authorization"
	HeaderBoundOperators                = "X-G8E-Bound-Operators"
	HeaderCLISessionID                  = "X-G8E-CLI-Session-ID"
	HeaderCacheControl                  = "Cache-Control"
	HeaderCaseID                        = "X-G8E-Case-ID"
	HeaderConnection                    = "Connection"
	HeaderContentDisposition            = "Content-Disposition"
	HeaderContentLanguage               = "Content-Language"
	HeaderContentLength                 = "Content-Length"
	HeaderContentType                   = "Content-Type"
	HeaderCookie                        = "Cookie"
	HeaderExecutionID                   = "X-G8E-Execution-ID"
	HeaderInvestigationID               = "X-G8E-Investigation-ID"
	HeaderLastEventID                   = "Last-Event-ID"
	HeaderOperatorID                    = "X-G8E-Operator-ID"
	HeaderOperatorSessionID             = "X-G8E-Operator-Session-ID"
	HeaderOrganizationID                = "X-G8E-Organization-ID"
	HeaderPragma                        = "Pragma"
	HeaderRequestID                     = "X-G8E-Request-ID"
	HeaderRequestedWith                 = "X-Requested-With"
	HeaderSetCookie                     = "Set-Cookie"
	HeaderSourceComponent               = "X-G8E-Source-Component"
	HeaderSystemFingerprint             = "X-G8E-System-Fingerprint"
	HeaderTaskID                        = "X-G8E-Task-ID"
	HeaderUserAgent                     = "User-Agent"
	HeaderUserID                        = "X-G8E-User-ID"
	HeaderVary                          = "Vary"
	HeaderWebSessionID                  = "X-G8E-Web-Session-ID"
	HeaderXAccelBuffering               = "X-Accel-Buffering"
	HeaderXContentTypeOptions           = "X-Content-Type-Options"
	HeaderXForwardedFor                 = "X-Forwarded-For"
	HeaderXForwardedHost                = "X-Forwarded-Host"
	HeaderXForwardedProto               = "X-Forwarded-Proto"
	HeaderXFrameOptions                 = "X-Frame-Options"
	HeaderContentSecurityPolicy         = "Content-Security-Policy"
	HeaderXProxyOrganizationID          = "X-Proxy-Organization-Id"
	HeaderXProxyUserID                  = "X-Proxy-User-Id"
	HeaderXRequestTimestamp             = "X-Request-Timestamp"
)

// JSON-RPC 2.0 protocol constants.
const (
	JSONRPCVersion           = "2.0"
	JSONRPCFieldVersion      = "jsonrpc"
	JSONRPCFieldMethod       = "method"
	JSONRPCFieldParams       = "params"
	JSONRPCFieldID           = "id"
	JSONRPCFieldResult       = "result"
	JSONRPCFieldError        = "error"
	JSONRPCFieldCode         = "code"
	JSONRPCFieldMessage      = "message"
	JSONRPCFieldData         = "data"
	JSONRPCErrorCodeInternal = -32603
	// JSONRPCErrorCodeParseError is the standard JSON-RPC 2.0 parse error code
	// (https://www.jsonrpc.org/specification#error_object), returned when a
	// request body cannot be parsed as valid JSON.
	JSONRPCErrorCodeParseError = -32700
	// JSONRPCErrorMessageParseError is the standard JSON-RPC 2.0 parse error
	// message prefix emitted by the gateway when a request body is invalid JSON.
	JSONRPCErrorMessageParseError = "parse error"
)

// HTTP header value constants.
const (
	HeaderValueNoSniff             = "nosniff"
	HeaderValueDeny                = "DENY"
	HeaderValueCSPNone             = "default-src 'none'; frame-ancestors 'none'"
	HeaderValueKeepAlive           = "keep-alive"
	HeaderValueNoCache             = "no-cache"
	HeaderValueTextEvent           = "text/event-stream"
	HeaderValueApplicationJSON     = "application/json"
	HeaderValueXHTML               = "application/xhtml+xml"
	HeaderValueXML                 = "application/xml"
	HeaderValueOctetStream         = "application/octet-stream"
	HeaderValuePEM                 = "application/x-pem-file"
	HeaderValueCRL                 = "application/pkix-crl"
	HeaderValueShell               = "application/x-sh"
	HeaderValuePowerShell          = "application/x-powershell"
	HeaderValueCORSPreflightMaxAge = "3600"
)

// ContextKey is a custom type for context keys to avoid collisions with other packages.
// Use these keys to store and retrieve values from context.Context values.
type ContextKey string

// AuthErrorReason is a typed string for authentication error reasons.
type AuthErrorReason string

const (
	// AuthErrorReasonTTLExceeded indicates the session exceeded its time-to-live.
	AuthErrorReasonTTLExceeded AuthErrorReason = "ttl_exceeded"
	// AuthErrorReasonRetiredByRealLogin indicates the identity was retired by a real login.
	AuthErrorReasonRetiredByRealLogin AuthErrorReason = "retired_by_real_login"
	// AuthErrorReasonIdentityDisabled indicates the identity is disabled.
	AuthErrorReasonIdentityDisabled AuthErrorReason = "identity_disabled"
	// AuthErrorReasonInvalidSession indicates the session is invalid.
	AuthErrorReasonInvalidSession AuthErrorReason = "invalid_session"
	// AuthErrorReasonSessionExpired indicates the session has expired.
	AuthErrorReasonSessionExpired AuthErrorReason = "session_expired"
	// AuthErrorReasonCertificateRevoked indicates the certificate is revoked.
	AuthErrorReasonCertificateRevoked AuthErrorReason = "certificate_revoked"
	// AuthErrorReasonIdentityMismatch indicates the mTLS identity does not match.
	AuthErrorReasonIdentityMismatch AuthErrorReason = "identity_mismatch"
	// AuthErrorReasonAppPolicyNotFound indicates the app policy was not found.
	AuthErrorReasonAppPolicyNotFound AuthErrorReason = "app_policy_not_found"
	// AuthErrorReasonRateLimitExceeded indicates the rate limit was exceeded.
	AuthErrorReasonRateLimitExceeded AuthErrorReason = "rate_limit_exceeded"
	// AuthErrorReasonPayloadTooLarge indicates the payload exceeds the maximum allowed size.
	AuthErrorReasonPayloadTooLarge AuthErrorReason = "payload_too_large"
	// AuthErrorReasonCollectionNotAllowed indicates the collection is not in the allowed list.
	AuthErrorReasonCollectionNotAllowed AuthErrorReason = "collection_not_allowed"
	// AuthErrorReasonJWTInvalid indicates the JWT token is invalid.
	AuthErrorReasonJWTInvalid AuthErrorReason = "jwt_invalid"
	// AuthErrorReasonJWTMissingSubject indicates the JWT is missing the subject claim.
	AuthErrorReasonJWTMissingSubject AuthErrorReason = "jwt_missing_subject"
)

const (
	// ContextKeyUserID stores the authenticated user ID in context.
	ContextKeyUserID ContextKey = "user_id"
	// ContextKeyAppID stores the application ID in context.
	ContextKeyAppID ContextKey = "app_id"
	// ContextKeyTenantID stores the tenant/organization ID in context.
	ContextKeyTenantID ContextKey = "tenant_id"
	// ContextKeyBindingPersona stores the binding persona identifier in context.
	ContextKeyBindingPersona ContextKey = "binding_persona"
	// ContextKeyOperatorID stores the operator ID in context.
	ContextKeyOperatorID ContextKey = "operator_id"
	// ContextKeyOperatorSessionID stores the operator session ID in context.
	ContextKeyOperatorSessionID ContextKey = "operator_session_id"
	// ContextKeyCapability stores the JIT-minted execution capability in context.
	ContextKeyCapability ContextKey = "execution_capability"
	// ContextKeyWebSessionID stores the web session ID for cookie-authenticated requests.
	ContextKeyWebSessionID ContextKey = "web_session_id"
	// ContextKeyCLISessionID stores the CLI session ID for mTLS-authenticated CLI requests.
	ContextKeyCLISessionID ContextKey = "cli_session_id"
	// ContextKeyStateMerkleRoot stores a pre-fetched state merkle root so the
	// in-process gateway build-verify window sees the same root for the same
	// transaction. Operator-side verification (without this key in context)
	// still re-fetches the current root from the provider.
	ContextKeyStateMerkleRoot ContextKey = "state_merkle_root"
)

// Session constants
const (
	// WebSessionTTL defines the lifetime of a web session.
	WebSessionTTL = 24 * time.Hour
	// WebSessionCookieName is the name of the browser session cookie used by the unified auth middleware.
	WebSessionCookieName = "g8e_web_session_cookie"
	// CLISessionTTL defines the lifetime of a CLI session. It is aligned with
	// the CLI certificate TTL (7 days, per leafCertValidityDays in
	// gateway_certs.go) so the session does not expire while the cert is
	// still valid. The previous 1-hour TTL created a window where the cert
	// was valid but the session was expired, leaving no recovery path short
	// of nuking the gateway volume. When the session does expire (gateway
	// restart, manual deactivation, or cert expiry), the auth/cli/refresh
	// endpoint reissues a session using the still-valid cert as proof of
	// identity.
	CLISessionTTL = 7 * 24 * time.Hour
)

// App enrollment type constants define the valid app_type values for external app enrollment.
const (
	AppTypeMCPClient       = "mcp-client"
	AppTypeA2AGateway      = "a2a-gateway"
	AppTypeCustom          = "custom"
	AppTypeConsensusMember = "consensus-member"
)

// L3 notary constants
const (
	// L3ApprovalWindow is the maximum time between approval and L3 verification.
	L3ApprovalWindow = 30 * time.Minute
)

// Certificate renewal constants
const (
	// AppCertMinValidity is the minimum remaining validity an app certificate must
	// have to be considered valid for reuse without re-enrollment.
	AppCertMinValidity = 7 * 24 * time.Hour
)

// Auth scheme and default identity constants.
const (
	// BearerScheme is the HTTP authorization scheme prefix for bearer tokens.
	BearerScheme = "Bearer "
	// DefaultTenantID is the fallback tenant ID when none is specified.
	DefaultTenantID = "default"
	// DefaultBindingPersona is the fallback persona when role mapping fails.
	DefaultBindingPersona = "default"
)
