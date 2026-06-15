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

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRPCErrors(t *testing.T) {
	t.Run("verification error codes are in correct range", func(t *testing.T) {
		assert.Equal(t, -32000, ErrCodeInvalidEnvelope)
		assert.Equal(t, -32001, ErrCodeHashMismatch)
		assert.Equal(t, -32002, ErrCodeExpired)
		assert.Equal(t, -32003, ErrCodeReplay)
		assert.Equal(t, -32004, ErrCodeStateMismatch)
		assert.Equal(t, -32005, ErrCodeL1ValidationFailed)
		assert.Equal(t, -32006, ErrCodeL2SignatureInvalid)
		assert.Equal(t, -32007, ErrCodeL3ProofInvalid)
		assert.Equal(t, -32008, ErrCodePayloadDecodeFailed)
	})

	t.Run("resource/state error codes are in correct range", func(t *testing.T) {
		assert.Equal(t, -32100, ErrCodeResourceNotFound)
		assert.Equal(t, -32101, ErrCodeGatewayNotReady)
	})

	t.Run("all error codes are negative", func(t *testing.T) {
		assert.Less(t, ErrCodeInvalidEnvelope, 0)
		assert.Less(t, ErrCodeHashMismatch, 0)
		assert.Less(t, ErrCodeExpired, 0)
		assert.Less(t, ErrCodeReplay, 0)
		assert.Less(t, ErrCodeStateMismatch, 0)
		assert.Less(t, ErrCodeL1ValidationFailed, 0)
		assert.Less(t, ErrCodeL2SignatureInvalid, 0)
		assert.Less(t, ErrCodeL3ProofInvalid, 0)
		assert.Less(t, ErrCodePayloadDecodeFailed, 0)
		assert.Less(t, ErrCodeResourceNotFound, 0)
		assert.Less(t, ErrCodeGatewayNotReady, 0)
	})

	t.Run("all error codes are distinct", func(t *testing.T) {
		codes := []int{
			ErrCodeInvalidEnvelope,
			ErrCodeHashMismatch,
			ErrCodeExpired,
			ErrCodeReplay,
			ErrCodeStateMismatch,
			ErrCodeL1ValidationFailed,
			ErrCodeL2SignatureInvalid,
			ErrCodeL3ProofInvalid,
			ErrCodePayloadDecodeFailed,
			ErrCodeResourceNotFound,
			ErrCodeGatewayNotReady,
		}

		seen := make(map[int]bool)
		for _, code := range codes {
			assert.False(t, seen[code], "error code %d is duplicated", code)
			seen[code] = true
		}
	})

}
