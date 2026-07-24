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

import "github.com/g8e-ai/g8e/internal/constants"

// Exported aliases for governance transaction errors, allowing callers outside
// this package to use errors.Is against the canonical sentinel values.
var (
	ErrInvalidEnvelope            = constants.ErrTxInvalidEnvelope
	ErrTransactionIDMissing       = constants.ErrTxTransactionIDMissing
	ErrPayloadMissing             = constants.ErrTxPayloadMissing
	ErrUnknownActionType          = constants.ErrTxUnknownActionType
	ErrPayloadDecodeFailed        = constants.ErrTxPayloadDecodeFailed
	ErrTransactionHashMissing     = constants.ErrTxTransactionHashMissing
	ErrTransactionHashMismatch    = constants.ErrTxTransactionHashMismatch
	ErrTransactionExpired         = constants.ErrTxTransactionExpired
	ErrTransactionReplay          = constants.ErrTxTransactionReplay
	ErrNonceMissing               = constants.ErrTxNonceMissing
	ErrReplayStoreMissing         = constants.ErrTxReplayStoreMissing
	ErrStateRootMissing           = constants.ErrTxStateRootMissing
	ErrStateRootRequired          = constants.ErrTxStateRootRequired
	ErrStateRootMismatch          = constants.ErrTxStateRootMismatch
	ErrL1ValidationFailed         = constants.ErrTxL1ValidationFailed
	ErrL2SignatureMissing         = constants.ErrTxL2SignatureMissing
	ErrL2SignatureInvalid         = constants.ErrTxL2SignatureInvalid
	ErrL2ConsensusNotConfigured   = constants.ErrTxL2ConsensusNotConfigured
	ErrL2SignerStoreNotConfigured = constants.ErrTxL2SignerStoreNotConfigured
	ErrL2QuorumNotMet             = constants.ErrTxL2QuorumNotMet
	ErrL2DuplicateSigner          = constants.ErrTxL2DuplicateSigner
	ErrL3ProofMissing             = constants.ErrTxL3ProofMissing
	ErrL3ProofInvalid             = constants.ErrTxL3ProofInvalid
	ErrL3NotaryNotConfigured      = constants.ErrTxL3NotaryNotConfigured
	ErrTxInFlight                 = constants.ErrTxInFlight
)
