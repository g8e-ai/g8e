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

// ApprovalCompletedEvent is the typed SSE event payload for L3 transaction
// approval. It is emitted by the gateway when a user completes the WebAuthn
// approval ceremony and is consumed by CLI clients (stdio proxy and approve
// command) to detect approval completion without polling.
type ApprovalCompletedEvent struct {
	Type   string `json:"type"`
	UserID string `json:"user_id"`
	TxHash string `json:"tx_hash"`
}
