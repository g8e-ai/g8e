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

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/stretchr/testify/assert"
)

func TestDocument(t *testing.T) {
	t.Run("creates valid document", func(t *testing.T) {
		now := time.Now().UTC()
		data := map[string]json.RawMessage{
			"name":  json.RawMessage(`"test"`),
			"value": json.RawMessage(`"123"`),
		}

		doc := &Document{
			ID:         "doc-123",
			Collection: "test_collection",
			Data:       data,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		assert.Equal(t, "doc-123", doc.ID)
		assert.Equal(t, "test_collection", doc.Collection)
		assert.Len(t, doc.Data, 2)
	})

	t.Run("forWire serializes document correctly", func(t *testing.T) {
		now := time.Now().UTC()
		data := map[string]json.RawMessage{
			"name": json.RawMessage(`"test"`),
		}

		doc := &Document{
			ID:         "doc-123",
			Collection: "test_collection",
			Data:       data,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		wire := doc.ForWire()

		assert.Contains(t, wire, "id")
		assert.Contains(t, wire, "created_at")
		assert.Contains(t, wire, "updated_at")
		assert.Contains(t, wire, "name")
	})
}

func TestDocFilter(t *testing.T) {
	t.Run("creates valid filter", func(t *testing.T) {
		filter := &DocFilter{
			Field: "status",
			Op:    "==",
			Value: json.RawMessage(`"active"`),
		}

		assert.Equal(t, "status", filter.Field)
		assert.Equal(t, "==", filter.Op)
	})
}

func TestDocQueryRequest(t *testing.T) {
	t.Run("creates valid query request", func(t *testing.T) {
		req := &DocQueryRequest{
			Filters: []DocFilter{
				{Field: "status", Op: "==", Value: json.RawMessage(`"active"`)},
			},
			OrderBy: "created_at",
			Limit:   100,
		}

		assert.Len(t, req.Filters, 1)
		assert.Equal(t, "created_at", req.OrderBy)
		assert.Equal(t, 100, req.Limit)
	})
}

func TestKVSetRequest(t *testing.T) {
	t.Run("creates valid set request", func(t *testing.T) {
		req := &KVSetRequest{
			Value: "test-value",
			TTL:   3600,
		}

		assert.Equal(t, "test-value", req.Value)
		assert.Equal(t, 3600, req.TTL)
	})
}

func TestKVExpireRequest(t *testing.T) {
	t.Run("creates valid expire request", func(t *testing.T) {
		req := &KVExpireRequest{
			TTL: 7200,
		}

		assert.Equal(t, 7200, req.TTL)
	})
}

func TestKVPatternRequest(t *testing.T) {
	t.Run("creates valid pattern request", func(t *testing.T) {
		req := &KVPatternRequest{
			Pattern: "test:*",
			Cursor:  0,
			Count:   100,
		}

		assert.Equal(t, "test:*", req.Pattern)
		assert.Equal(t, 0, req.Cursor)
		assert.Equal(t, 100, req.Count)
	})
}

func TestPubSubPublishRequest(t *testing.T) {
	t.Run("creates valid publish request", func(t *testing.T) {
		req := &PubSubPublishRequest{
			Channel: "test-channel",
			Data:    json.RawMessage(`{"message":"test"}`),
		}

		assert.Equal(t, "test-channel", req.Channel)
		assert.NotNil(t, req.Data)
	})
}

func TestHealthResponse(t *testing.T) {
	t.Run("creates valid health response", func(t *testing.T) {
		resp := &HealthResponse{
			Status:          constants.GatewayModeMode,
			Mode:            constants.GatewayModeMode,
			Version:         "v0.1.0",
			GovernanceReady: true,
			StateMerkleRoot: "root123",
		}

		assert.Equal(t, constants.GatewayModeMode, resp.Status)
		assert.True(t, resp.GovernanceReady)
		assert.Equal(t, "root123", resp.StateMerkleRoot)
	})
}

func TestStatusResponse(t *testing.T) {
	t.Run("creates valid status response", func(t *testing.T) {
		resp := &StatusResponse{
			Status: constants.GatewayModeStatusOK,
		}

		assert.Equal(t, constants.GatewayModeStatusOK, resp.Status)
	})
}

func TestKVGetResponse(t *testing.T) {
	t.Run("creates valid get response", func(t *testing.T) {
		resp := &KVGetResponse{
			Value: "test-value",
		}

		assert.Equal(t, "test-value", resp.Value)
	})
}

func TestKVTTLResponse(t *testing.T) {
	t.Run("creates valid TTL response", func(t *testing.T) {
		resp := &KVTTLResponse{
			TTL: 3600,
		}

		assert.Equal(t, 3600, resp.TTL)
	})
}

func TestKVKeysResponse(t *testing.T) {
	t.Run("creates valid keys response", func(t *testing.T) {
		resp := &KVKeysResponse{
			Keys: []string{"key1", "key2", "key3"},
		}

		assert.Len(t, resp.Keys, 3)
	})
}

func TestKVScanResponse(t *testing.T) {
	t.Run("creates valid scan response", func(t *testing.T) {
		resp := &KVScanResponse{
			Cursor: 100,
			Keys:   []string{"key1", "key2"},
		}

		assert.Equal(t, 100, resp.Cursor)
		assert.Len(t, resp.Keys, 2)
	})
}

func TestKVDeletePatternResponse(t *testing.T) {
	t.Run("creates valid delete pattern response", func(t *testing.T) {
		resp := &KVDeletePatternResponse{
			Deleted: 10,
		}

		assert.Equal(t, int64(10), resp.Deleted)
	})
}

func TestPubSubPublishResponse(t *testing.T) {
	t.Run("creates valid publish response", func(t *testing.T) {
		resp := &PubSubPublishResponse{
			Receivers: 5,
		}

		assert.Equal(t, 5, resp.Receivers)
	})
}

func TestActionReceiptRecord(t *testing.T) {
	t.Run("creates valid action receipt", func(t *testing.T) {
		now := time.Now().UTC()
		receipt := &ActionReceiptRecord{
			TransactionID:     "txn-123",
			TransactionHash:   "hash123",
			OperatorID:        "operator-123",
			OperatorSessionID: "session-123",
			ActionType:        constants.ActionTypeExecuteBash,
			TargetResource:    "/tmp",
			Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:     "success",
			StateRootBefore:   "root-before",
			StateRootAfter:    "root-after",
			ExecutedAt:        now,
			SignerKeyID:       "signer-123",
			Signature:         "sig123",
			GatewaySigned:     true,
			L2Valid:           true,
			L3Valid:           true,
			Timestamp:         now,
		}

		assert.Equal(t, "txn-123", receipt.TransactionID)
		assert.Equal(t, constants.ActionTypeExecuteBash, receipt.ActionType)
		assert.True(t, receipt.GatewaySigned)
		assert.True(t, receipt.L2Valid)
		assert.True(t, receipt.L3Valid)
	})
}

func TestBlobMetaResponse(t *testing.T) {
	t.Run("creates valid blob meta response", func(t *testing.T) {
		now := time.Now().UTC()
		resp := &BlobMetaResponse{
			ID:          "blob-123",
			Namespace:   "test-ns",
			Size:        1024,
			ContentType: "text/plain",
			CreatedAt:   now,
		}

		assert.Equal(t, "blob-123", resp.ID)
		assert.Equal(t, int64(1024), resp.Size)
	})
}

func TestBlobDeleteResponse(t *testing.T) {
	t.Run("creates valid blob delete response", func(t *testing.T) {
		resp := &BlobDeleteResponse{
			Deleted: 5,
		}

		assert.Equal(t, int64(5), resp.Deleted)
	})
}

func TestSSEEventRow(t *testing.T) {
	t.Run("creates valid SSE event row with web session", func(t *testing.T) {
		row := &SSEEventRow{
			ID:           123,
			WebSessionID: "session-123",
			EventType:    "test-event",
			Payload:      `{"data":"test"}`,
			CreatedAt:    "2026-01-01T00:00:00Z",
		}

		assert.Equal(t, int64(123), row.ID)
		assert.Equal(t, "session-123", row.WebSessionID)
	})

	t.Run("creates valid SSE event row with CLI session", func(t *testing.T) {
		row := &SSEEventRow{
			ID:           124,
			CLISessionID: "cli-session-123",
			EventType:    "test-event",
			Payload:      `{"data":"test"}`,
			CreatedAt:    "2026-01-01T00:00:00Z",
		}

		assert.Equal(t, "cli-session-123", row.CLISessionID)
	})

	t.Run("creates valid SSE event row with user ID", func(t *testing.T) {
		row := &SSEEventRow{
			ID:        125,
			UserID:    "user-123",
			EventType: "test-event",
			Payload:   `{"data":"test"}`,
			CreatedAt: "2026-01-01T00:00:00Z",
		}

		assert.Equal(t, "user-123", row.UserID)
	})
}

func TestSSEPushResponse(t *testing.T) {
	t.Run("creates valid push response", func(t *testing.T) {
		resp := &SSEPushResponse{
			Success:   true,
			Delivered: 5,
		}

		assert.True(t, resp.Success)
		assert.Equal(t, 5, resp.Delivered)
	})
}

func TestSSEEventsResponse(t *testing.T) {
	t.Run("creates valid events response", func(t *testing.T) {
		resp := &SSEEventsResponse{
			Events: []SSEEventRow{
				{ID: 1, EventType: "event1"},
				{ID: 2, EventType: "event2"},
			},
			Count: 2,
		}

		assert.Len(t, resp.Events, 2)
		assert.Equal(t, 2, resp.Count)
	})
}

func TestReauthResponse(t *testing.T) {
	t.Run("creates valid reauth response", func(t *testing.T) {
		resp := &ReauthResponse{
			Success: true,
			Operator: &OperatorDocumentGo{
				ID: "operator-123",
			},
		}

		assert.True(t, resp.Success)
		assert.NotNil(t, resp.Operator)
	})
}

func TestAuditReceiptsResponse(t *testing.T) {
	t.Run("creates valid receipts response", func(t *testing.T) {
		now := time.Now().UTC()
		resp := &AuditReceiptsResponse{
			Success: true,
			Receipts: []*ActionReceiptRecord{
				{
					TransactionID: "txn-1",
					ExecutedAt:    now,
				},
			},
		}

		assert.True(t, resp.Success)
		assert.Len(t, resp.Receipts, 1)
	})
}

func TestTrustedSignersResponse(t *testing.T) {
	t.Run("creates valid signers response", func(t *testing.T) {
		resp := &TrustedSignersResponse{
			Success: true,
			Signers: []TrustedSigner{
				{ID: "signer-1", PublicKey: "key1"},
				{ID: "signer-2", PublicKey: "key2"},
			},
		}

		assert.True(t, resp.Success)
		assert.Len(t, resp.Signers, 2)
	})
}

func TestPasskeyChallengeResponse(t *testing.T) {
	t.Run("creates valid challenge response", func(t *testing.T) {
		resp := &PasskeyChallengeResponse{
			Success:    true,
			NeedsSetup: false,
		}

		assert.True(t, resp.Success)
		assert.False(t, resp.NeedsSetup)
	})

	t.Run("creates error response", func(t *testing.T) {
		resp := &PasskeyChallengeResponse{
			Success: false,
			Error:   "user not found",
		}

		assert.False(t, resp.Success)
		assert.Equal(t, "user not found", resp.Error)
	})
}

func TestPasskeyVerifyResponse(t *testing.T) {
	t.Run("creates valid verify response", func(t *testing.T) {
		resp := &PasskeyVerifyResponse{
			Success:      true,
			UserID:       "user-123",
			WebSessionID: "session-123",
		}

		assert.True(t, resp.Success)
		assert.Equal(t, "user-123", resp.UserID)
	})
}

func TestPasskeyCredentialsResponse(t *testing.T) {
	t.Run("creates valid credentials response", func(t *testing.T) {
		resp := &PasskeyCredentialsResponse{
			Success: true,
			Credentials: []PasskeyCredential{
				{ID: []byte("cred-1")},
			},
		}

		assert.True(t, resp.Success)
		assert.Len(t, resp.Credentials, 1)
	})
}

func TestPasskeyRevokeResponse(t *testing.T) {
	t.Run("creates valid revoke response", func(t *testing.T) {
		resp := &PasskeyRevokeResponse{
			Success:   true,
			Found:     true,
			Remaining: 2,
		}

		assert.True(t, resp.Success)
		assert.True(t, resp.Found)
		assert.Equal(t, 2, resp.Remaining)
	})
}

func TestAuthLoginChallengeResponse(t *testing.T) {
	t.Run("creates valid challenge response", func(t *testing.T) {
		resp := &AuthLoginChallengeResponse{
			Success: true,
			UserID:  "user-123",
		}

		assert.True(t, resp.Success)
		assert.Equal(t, "user-123", resp.UserID)
	})
}

func TestAuthLoginVerifyResponse(t *testing.T) {
	t.Run("creates valid verify response", func(t *testing.T) {
		resp := &AuthLoginVerifyResponse{
			Success:      true,
			UserID:       "user-123",
			WebSessionID: "session-123",
		}

		assert.True(t, resp.Success)
		assert.Equal(t, "user-123", resp.UserID)
	})
}

func TestBootstrapStatusResponse(t *testing.T) {
	t.Run("creates valid bootstrap status response", func(t *testing.T) {
		resp := &BootstrapStatusResponse{
			Bootstrapped: true,
		}

		assert.True(t, resp.Bootstrapped)
	})
}

func TestUserMeResponse(t *testing.T) {
	t.Run("creates valid user me response", func(t *testing.T) {
		resp := &UserMeResponse{
			Success: true,
			User: &User{
				ID: "user-123",
			},
		}

		assert.True(t, resp.Success)
		assert.NotNil(t, resp.User)
	})
}

func TestWebSessionResponse(t *testing.T) {
	t.Run("creates valid web session response", func(t *testing.T) {
		resp := &WebSessionResponse{
			Success:      true,
			UserID:       "user-123",
			WebSessionID: "session-123",
		}

		assert.True(t, resp.Success)
		assert.Equal(t, "user-123", resp.UserID)
	})
}

func TestSettingsDocument(t *testing.T) {
	t.Run("creates valid settings document", func(t *testing.T) {
		now := time.Now().UTC()
		doc := &SettingsDocument{
			Settings: map[string]interface{}{
				"key": "value",
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		assert.Equal(t, "value", doc.Settings["key"])
	})
}

func TestUserSettingsDocument(t *testing.T) {
	t.Run("creates valid user settings document", func(t *testing.T) {
		now := time.Now().UTC()
		doc := &UserSettingsDocument{
			Settings: map[string]interface{}{
				"theme": "dark",
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		assert.Equal(t, "dark", doc.Settings["theme"])
	})
}
