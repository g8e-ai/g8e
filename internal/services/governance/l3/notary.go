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

package l3

import (
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// Notary provides L3 (Authorization) verification for human-in-the-loop approval.
// L3 is the final gate that requires human presence before mutations execute.
type Notary interface {
	// VerifyL3Proof verifies an L3 proof for a transaction.
	// Returns true if the proof is valid and the transaction should be allowed.
	VerifyL3Proof(userID, transactionHash, cliSessionID string, proof *commonv1.L3Proof) (bool, error)
}
