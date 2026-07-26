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

//go:build integration

package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

func TestGatewayModeService_HandleHeartbeatPublish(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	t.Run("Valid heartbeat updates operator document", func(t *testing.T) {
		opDoc := map[string]interface{}{
			"id":     "op-123",
			"status": "active",
		}
		opBytes, err := json.Marshal(opDoc)
		require.NoError(t, err)
		err = ls.stores.DocStore.DocSet("operators", "op-123", opBytes)
		require.NoError(t, err)

		envelope := &commonv1.GovernanceEnvelope{
			OperatorId: "op-123",
			IntentData: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"uptime": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"seconds": structpb.NewNumberValue(12345),
						},
					}),
				},
			},
		}
		heartbeatBytes, err := protojson.Marshal(envelope)
		require.NoError(t, err)

		ls.handleHeartbeatPublish("test-channel", heartbeatBytes)

		updatedDoc, err := ls.stores.DocStore.DocGet("operators", "op-123")
		require.NoError(t, err)
		assert.NotNil(t, updatedDoc)
		assert.Contains(t, updatedDoc.Data, "latest_heartbeat_snapshot")
	})

	t.Run("Malformed JSON logs and returns", func(t *testing.T) {
		ls.handleHeartbeatPublish("test-channel", []byte("{invalid json"))
	})

	t.Run("Missing operator_id returns without write", func(t *testing.T) {
		envelope := &commonv1.GovernanceEnvelope{
			IntentData: &structpb.Struct{},
		}
		heartbeatBytes, err := protojson.Marshal(envelope)
		require.NoError(t, err)

		ls.handleHeartbeatPublish("test-channel", heartbeatBytes)
	})
}
