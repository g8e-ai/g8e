// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestGatewayModeService_HandleHeartbeatPublish(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	t.Run("Valid heartbeat updates operator document", func(t *testing.T) {
		opDoc := models.OperatorDocumentGo{
			ID:     "op-123",
			Status: constants.OperatorStatusActive,
		}
		opBytes, err := json.Marshal(opDoc)
		require.NoError(t, err)
		err = ls.GetDocStore().DocSet("operators", "op-123", opBytes)
		require.NoError(t, err)

		heartbeat := &operatorv1.HeartbeatResult{
			OperatorId: "op-123",
			Status:     "automatic",
			SystemIdentity: &operatorv1.SystemIdentity{
				Hostname: "worker-1",
			},
		}
		payload, err := proto.Marshal(heartbeat)
		require.NoError(t, err)

		envelope := &commonv1.GovernanceEnvelope{
			OperatorId: "op-123",
			Payload:    payload,
		}
		heartbeatBytes, err := protojson.Marshal(envelope)
		require.NoError(t, err)

		ls.handleHeartbeatPublish("test-channel", heartbeatBytes)

		updatedDoc, err := ls.GetDocStore().DocGet("operators", "op-123")
		require.NoError(t, err)
		assert.NotNil(t, updatedDoc)
		assert.Contains(t, updatedDoc.Data, "latest_heartbeat_snapshot")
		assert.Contains(t, updatedDoc.Data, "current_hostname")
		assert.Contains(t, updatedDoc.Data, "worker-1")
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
