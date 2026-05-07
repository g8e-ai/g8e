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

package pubsub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
)

// L2 Tribunal verification errors. Each error is distinct so dispatcher
// logs can distinguish between "no signature present" and "signature
// present but invalid" — operationally important during rollout.
var (
	ErrL2KeyNotConfigured = errors.New("L2: auditor HMAC key not configured")
	ErrL2SignatureMissing = errors.New("L2: tribunal_signature missing from envelope")
	ErrL2SignatureInvalid = errors.New("L2: tribunal_signature does not match computed HMAC")
)

// computeL2Signature produces the canonical HMAC-SHA256 over
// (event_type || '\n' || payload_bytes). The input layout MUST stay
// byte-for-byte identical to the publisher-side signer in
// components/g8ee/app/utils/envelope_builder.py::sign_l2_tribunal.
func computeL2Signature(eventType string, payload []byte, stringKey string) string {
	key := []byte(stringKey)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(eventType))
	mac.Write([]byte{'\n'})
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyL2Governance enforces the L2 Tribunal signature on an inbound
// UniversalEnvelope. Returns nil when the signature is present and
// matches. The caller is expected to reject the command on any
// non-nil return.
func VerifyL2Governance(env *commonv1.UniversalEnvelope, stringKey string) error {
	if stringKey == "" {
		return ErrL2KeyNotConfigured
	}
	if env == nil || env.Governance == nil || env.Governance.L2 == nil {
		return ErrL2SignatureMissing
	}
	got := env.Governance.L2.TribunalSignature
	if got == "" {
		return ErrL2SignatureMissing
	}
	want := computeL2Signature(env.EventType, env.Payload, stringKey)
	if !hmac.Equal([]byte(got), []byte(want)) {
		return fmt.Errorf("%w: event_type=%s", ErrL2SignatureInvalid, env.EventType)
	}
	return nil
}
