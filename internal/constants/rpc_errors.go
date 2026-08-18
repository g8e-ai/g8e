// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// Protocol-specific error codes for g8e (reserved range -32000 to -32099)
const (
	// Verification Errors (-32000 range)
	ErrCodeInvalidEnvelope     = -32000
	ErrCodeHashMismatch        = -32001
	ErrCodeExpired             = -32002
	ErrCodeReplay              = -32003
	ErrCodeStateMismatch       = -32004
	ErrCodeL1ValidationFailed  = -32005
	ErrCodeL2SignatureInvalid  = -32006
	ErrCodeL3ProofInvalid      = -32007
	ErrCodePayloadDecodeFailed = -32008

	// Resource/State Errors (-32100 range)
	ErrCodeResourceNotFound = -32100
	ErrCodeGatewayNotReady  = -32101
)
