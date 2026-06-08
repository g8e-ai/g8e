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
