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

// L2Consensus/L3Notary verification errors. Each error is distinct so dispatcher
// logs can distinguish between different failure modes.
var (
	ErrL2KeyNotConfigured   = errors.New("L2Consensus: trusted ED25519 key not configured")
	ErrL2KeyIDMissing       = errors.New("L2Consensus: key_id missing from envelope")
	ErrL2SignatureMissing   = errors.New("L2Consensus: consensus_signature missing from envelope")
	ErrL2AsymmetricInvalid  = errors.New("L2Consensus: consensus_signature failed ED25519 verification")
	ErrL3ProofMissing       = errors.New("L3Notary: governance.l3.proof missing from envelope")
	ErrL3ProofInvalid       = errors.New("L3Notary: governance.l3.proof failed verification")
	ErrStateRootMissing     = errors.New("protocol: state_merkle_root missing")
	ErrStateRootUnavailable = errors.New("protocol: current state merkle root unavailable")
	ErrStateRootMismatch    = errors.New("protocol: state_merkle_root does not match current state")
	ErrTransactionExpired   = errors.New("protocol: transaction has expired")
	ErrTransactionReplay    = errors.New("protocol: transaction replay detected")
)

// L2Consensus and L3Notary verification for UAP JSON envelopes is handled by the L2Consensus and L5Actuator services.
// This file retains error definitions for consistency but the actual verification logic
// is in internal/services/governance/l2_consensus.go and l5_actuator.go.
