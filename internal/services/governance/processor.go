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

package governance

import (
	"context"

	"github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// EnvelopeProcessor verifies and executes GovernanceEnvelopes synchronously,
// returning a signed ActionReceipt or a governance verification error.
// It is the primary entry point for the g8e Gateway's fail-closed mutation gate.
type EnvelopeProcessor interface {
	// ProcessEnvelope validates and executes a GovernanceEnvelope payload through
	// the 5-layer verification gauntlet, returning a signed ActionReceipt on success.
	ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error)
}
