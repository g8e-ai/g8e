// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
