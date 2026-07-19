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

import "time"

//go:generate mockery --name ReplayStore --output ./mocks --dir .

// ReplayStore defines the interface for nonce replay protection.
type ReplayStore interface {
	// ReserveNonce atomically reserves a nonce for early replay protection.
	// Returns true if the nonce was already reserved/used (replay detected).
	// If not used, it reserves the nonce and returns false.
	// This allows early durable commitment before expensive L2/L3 checks.
	ReserveNonce(nonce string, expiresAt time.Time) (bool, error)

	// FinalizeNonce marks a reserved nonce as fully consumed.
	// This is called after successful execution to prevent reuse.
	FinalizeNonce(nonce string) error

	// ReleaseNonce removes a reservation for a failed transaction.
	// This allows the nonce to be reused for retry.
	ReleaseNonce(nonce string) error

	// Close shuts down the replay store and releases resources.
	Close() error
}
