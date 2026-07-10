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

// WebAuthnAttestationResponse is the shared client response for WebAuthn
// registration verification. It is used by both the CLI auth package and the
// gateway passkey service to avoid duplicate type definitions.
type WebAuthnAttestationResponse struct {
	ID                string   `json:"id"`
	RawID             string   `json:"rawId"`
	ClientDataJSON    string   `json:"clientDataJSON"`
	AttestationObject string   `json:"attestationObject"`
	Transports        []string `json:"transports,omitempty"`
}

// WebAuthnAssertionResponse is the shared client response for WebAuthn
// authentication verification. It is used by both the CLI auth package and the
// gateway passkey service to avoid duplicate type definitions.
type WebAuthnAssertionResponse struct {
	ID                string `json:"id"`
	RawID             string `json:"rawId"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle,omitempty"`
}

// ParsedAssertionResponse is the nested JSON format that go-webauthn expects
// when verifying an assertion. The response fields are nested under "response"
// rather than flat.
type ParsedAssertionResponse struct {
	ID       string                      `json:"id"`
	RawID    string                      `json:"rawId"`
	Type     string                      `json:"type"`
	Response ParsedAssertionResponseBody `json:"response"`
}

// ParsedAssertionResponseBody is the nested response object within
// ParsedAssertionResponse.
type ParsedAssertionResponseBody struct {
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
}
