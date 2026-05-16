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
	"errors"
)

// L2/L3 verification errors. Each error is distinct so dispatcher
// logs can distinguish between different failure modes.
var (
	ErrL2KeyNotConfigured   = errors.New("L2: trusted ED25519 key not configured")
	ErrL2KeyIDMissing       = errors.New("L2: key_id missing from envelope")
	ErrL2SignatureMissing   = errors.New("L2: tribunal_signature missing from envelope")
	ErrL2AsymmetricInvalid  = errors.New("L2: tribunal_signature failed ED25519 verification")
	ErrL3ProofMissing       = errors.New("L3: governance.l3.proof missing from envelope")
	ErrL3ProofInvalid       = errors.New("L3: governance.l3.proof failed verification")
	ErrStateRootMissing     = errors.New("Protocol: state_merkle_root missing")
	ErrStateRootUnavailable = errors.New("Protocol: current state merkle root unavailable")
	ErrStateRootMismatch    = errors.New("Protocol: state_merkle_root does not match current state")
	ErrTransactionExpired   = errors.New("Protocol: transaction has expired")
	ErrTransactionReplay    = errors.New("Protocol: transaction replay detected")
)

// L2 and L3 verification for UAP JSON envelopes is handled by the Tribunal and Warden services.
// This file保留 error definitions for consistency but the actual verification logic
// is in components/g8eo/services/governance/tribunal.go and warden.go.
