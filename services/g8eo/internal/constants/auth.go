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
)
