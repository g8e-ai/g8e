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

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/governance"
)

type fakeEnvelopeProcessor struct {
	receipt *operatorv1.ActionReceipt
	err     error
}

func (f *fakeEnvelopeProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	return f.receipt, f.err
}

func TestGatewayService_HandleToolsCall_ErrorMapping(t *testing.T) {
	cases := []struct {
		name         string
		substrateErr error
		expectedCode int
	}{
		{
			name:         "invalid envelope",
			substrateErr: governance.ErrInvalidEnvelope,
			expectedCode: ErrCodeInvalidEnvelope,
		},
		{
			name:         "hash mismatch",
			substrateErr: governance.ErrTransactionHashMismatch,
			expectedCode: ErrCodeHashMismatch,
		},
		{
			name:         "expired",
			substrateErr: governance.ErrTransactionExpired,
			expectedCode: ErrCodeExpired,
		},
		{
			name:         "replay",
			substrateErr: governance.ErrTransactionReplay,
			expectedCode: ErrCodeReplay,
		},
		{
			name:         "state root mismatch",
			substrateErr: governance.ErrStateRootMismatch,
			expectedCode: ErrCodeStateMismatch,
		},
		{
			name:         "L1 validation failed",
			substrateErr: governance.ErrL1ValidationFailed,
			expectedCode: ErrCodeL1ValidationFailed,
		},
		{
			name:         "L2 signature invalid",
			substrateErr: governance.ErrL2SignatureInvalid,
			expectedCode: ErrCodeL2SignatureInvalid,
		},
		{
			name:         "L3 proof invalid",
			substrateErr: governance.ErrL3ProofInvalid,
			expectedCode: ErrCodeL3ProofInvalid,
		},
		{
			name:         "payload decode failed",
			substrateErr: governance.ErrPayloadDecodeFailed,
			expectedCode: ErrCodePayloadDecodeFailed,
		},
		{
			name:         "internal error fallback",
			substrateErr: errors.New("some internal error"),
			expectedCode: -32603,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proc := &fakeEnvelopeProcessor{err: tc.substrateErr}
			g := &GatewayService{
				envProc: proc,
			}

			// Valid MCP tools/call request
			reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test-tool","arguments":{}}}`
			req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))
			w := httptest.NewRecorder()

			g.HandleToolsCall(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var resp JSONRPCResponse
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			require.NoError(t, err)

			require.NotNil(t, resp.Error)
			require.Equal(t, tc.expectedCode, resp.Error.Code)
			require.Equal(t, tc.substrateErr.Error(), resp.Error.Message)
		})
	}
}
