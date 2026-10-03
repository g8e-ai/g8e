// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

// ReputationSignRequest is an application reputation claim, not an execution
// approval or governance proof. The Gateway keeps the HMAC key private.
type ReputationSignRequest struct {
	MerkleRoot        string `json:"merkle_root"`
	PrevRoot          string `json:"prev_root"`
	TribunalCommandID string `json:"tribunal_command_id"`
}

type ReputationSignResponse struct {
	Signature string `json:"signature"`
}
