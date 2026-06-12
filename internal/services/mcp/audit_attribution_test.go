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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// TestGatewayAuditAttribution tests that audit attribution (operator_id and operator_session_id)
// is correctly extracted from context and injected into the governance envelope.
func TestGatewayAuditAttribution(t *testing.T) {
	t.Parallel()

	processor := &fakeEnvelopeProcessor{
		receipt: &operatorv1.ActionReceipt{
			TransactionId: "tx-audit-1",
			Status:        operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		},
	}

	capture := &auditAttributionCaptureProcessor{
		delegate: processor,
	}

	g := newTestGatewayService(t, withEnvProc(capture))

	t.Run("attribution from operator session context", func(t *testing.T) {
		capture.lastEnvelope = nil
		opID := "op-123"
		opSessionID := "opsess-456"

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sys_info","arguments":{}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))

		// Simulate what the auth middleware does
		ctx := req.Context()
		ctx = context.WithValue(ctx, constants.ContextKeyOperatorID, opID)
		ctx = context.WithValue(ctx, constants.ContextKeyOperatorSessionID, opSessionID)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		g.HandleToolsCall(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, capture.lastEnvelope)
		assert.Equal(t, opID, capture.lastEnvelope.OperatorId)
		assert.Equal(t, opSessionID, capture.lastEnvelope.OperatorSessionId)
	})

	t.Run("attribution from app identity context (priority)", func(t *testing.T) {
		capture.lastEnvelope = nil
		appID := "spiffe://g8e.local/app/my-app"
		opID := "op-123"
		opSessionID := "opsess-456"

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sys_info","arguments":{}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))

		// Simulate app auth
		ctx := req.Context()
		ctx = context.WithValue(ctx, constants.ContextKeyAppID, appID)
		ctx = context.WithValue(ctx, constants.ContextKeyOperatorID, opID)
		ctx = context.WithValue(ctx, constants.ContextKeyOperatorSessionID, opSessionID)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		g.HandleToolsCall(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, capture.lastEnvelope)
		// App identity should take precedence and be used for both ID and SessionID
		assert.Equal(t, appID, capture.lastEnvelope.OperatorId)
		assert.Equal(t, appID, capture.lastEnvelope.OperatorSessionId)
	})

	t.Run("attribution empty when no context present", func(t *testing.T) {
		capture.lastEnvelope = nil
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sys_info","arguments":{}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", strings.NewReader(reqBody))

		w := httptest.NewRecorder()
		g.HandleToolsCall(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, capture.lastEnvelope)
		assert.Empty(t, capture.lastEnvelope.OperatorId)
		assert.Empty(t, capture.lastEnvelope.OperatorSessionId)
	})
}

// auditAttributionCaptureProcessor stores the last envelope for audit attribution testing
type auditAttributionCaptureProcessor struct {
	delegate     governance.EnvelopeProcessor
	lastEnvelope *commonv1.GovernanceEnvelope
}

func (e *auditAttributionCaptureProcessor) ProcessEnvelope(ctx context.Context, payload []byte) (*operatorv1.ActionReceipt, error) {
	var envelope commonv1.GovernanceEnvelope
	if err := protojson.Unmarshal(payload, &envelope); err != nil {
		// Log the error for debugging
		fmt.Printf("DEBUG: Failed to unmarshal envelope: %v\nPayload: %s\n", err, string(payload))
	} else {
		e.lastEnvelope = &envelope
	}
	return e.delegate.ProcessEnvelope(ctx, payload)
}
