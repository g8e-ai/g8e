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

package constants

// Passkey purpose constants.
const (
	PasskeyPurposeRegister = "register"
	PasskeyPurposeAuth     = "auth"
)

// WebAuthn algorithm and type constants.
const (
	WebAuthnTypePublicKey = "public-key"
	WebAuthnAlgES256      = -7
	WebAuthnAlgRS256      = -257
)

// WebAuthn attestation and selection constants.
const (
	WebAuthnAttestationNone          = "none"
	WebAuthnResidentKeyRequired      = "required"
	WebAuthnUserVerificationRequired = "required"
)

// PKI leaf type constants.
const (
	LeafTypeOperator = "operator"
	LeafTypeApp      = "app"
	LeafTypeHub      = "hub"
	LeafTypeCLI      = "cli"
)

// HTTP header constants.
const (
	HeaderAccept                        = "Accept"
	HeaderAcceptLanguage                = "Accept-Language"
	HeaderAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	HeaderAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	HeaderAccessControlRequestHeaders   = "Access-Control-Request-Headers"
	HeaderAccessControlRequestMethod    = "Access-Control-Request-Method"
	HeaderAuthorization                 = "Authorization"
	HeaderBoundOperators                = "X-G8E-Bound-Operators"
	HeaderCLISessionID                  = "X-G8E-CLI-Session-ID"
	HeaderCacheControl                  = "Cache-Control"
	HeaderCaseID                        = "X-G8E-Case-ID"
	HeaderContentDisposition            = "Content-Disposition"
	HeaderContentLanguage               = "Content-Language"
	HeaderContentLength                 = "Content-Length"
	HeaderContentType                   = "Content-Type"
	HeaderCookie                        = "Cookie"
	HeaderDeviceToken                   = "X-G8E-Device-Token"
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
	HeaderWebSessionID                  = "X-G8E-Web-Session-ID"
	HeaderXAccelBuffering               = "X-Accel-Buffering"
	HeaderXForwardedFor                 = "X-Forwarded-For"
	HeaderXForwardedHost                = "X-Forwarded-Host"
	HeaderXForwardedProto               = "X-Forwarded-Proto"
	HeaderXProxyOrganizationID          = "X-Proxy-Organization-Id"
	HeaderXProxyUserID                  = "X-Proxy-User-Id"
	HeaderXRequestTimestamp             = "X-Request-Timestamp"
)

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

const (
	ContextKeyUserID         ContextKey = "user_id"
	ContextKeyAppID          ContextKey = "app_id"
	ContextKeyTenantID       ContextKey = "tenant_id"
	ContextKeyBindingPersona ContextKey = "binding_persona"
)
