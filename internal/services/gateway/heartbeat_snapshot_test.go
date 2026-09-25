// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestHeartbeatResultFromEnvelope_BinaryPayload(t *testing.T) {
	heartbeat := &operatorv1.HeartbeatResult{
		OperatorId:        "op-1",
		OperatorSessionId: "sess-1",
		Timestamp:         "2026-09-25T12:00:00Z",
		Status:            "automatic",
		SystemIdentity: &operatorv1.SystemIdentity{
			Hostname:     "worker-1",
			Os:           "linux",
			Architecture: "amd64",
			CurrentUser:  "root",
		},
	}
	payload, err := proto.Marshal(heartbeat)
	require.NoError(t, err)

	env := &commonv1.GovernanceEnvelope{
		OperatorId:        "op-1",
		OperatorSessionId: "sess-1",
		Payload:           payload,
	}

	decoded, err := heartbeatResultFromEnvelope(env)
	require.NoError(t, err)
	assert.Equal(t, "worker-1", decoded.SystemIdentity.Hostname)
	assert.Equal(t, "automatic", decoded.Status)
}

func TestHeartbeatResultFromEnvelope_IntentDataProtojson(t *testing.T) {
	heartbeat := &operatorv1.HeartbeatResult{
		OperatorId: "op-1",
		Status:     "automatic",
		SystemIdentity: &operatorv1.SystemIdentity{
			Hostname: "protojson-host",
		},
	}
	intentJSON, err := protojson.Marshal(heartbeat)
	require.NoError(t, err)

	intentStruct := &structpb.Struct{}
	require.NoError(t, protojson.Unmarshal(intentJSON, intentStruct))

	env := &commonv1.GovernanceEnvelope{
		OperatorId: "op-1",
		IntentData: intentStruct,
	}

	decoded, err := heartbeatResultFromEnvelope(env)
	require.NoError(t, err)
	assert.Equal(t, "protojson-host", decoded.SystemIdentity.Hostname)
}

func TestMarshalHeartbeatSnapshot_UsesProtoFieldNames(t *testing.T) {
	heartbeat := &operatorv1.HeartbeatResult{
		Status: "automatic",
		SystemIdentity: &operatorv1.SystemIdentity{
			Hostname: "stored-host",
		},
		PerformanceMetrics: &operatorv1.PerformanceMetrics{
			CpuPercent: 12.5,
		},
	}

	snapshot, err := marshalHeartbeatSnapshot(heartbeat)
	require.NoError(t, err)

	var stored map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(snapshot, &stored))
	assert.Contains(t, stored, "system_identity")
	assert.Contains(t, stored, "performance_metrics")
	assert.NotContains(t, stored, "systemIdentity")
	assert.NotContains(t, stored, "performanceMetrics")

	var systemIdentity map[string]string
	require.NoError(t, json.Unmarshal(stored["system_identity"], &systemIdentity))
	assert.Equal(t, "stored-host", systemIdentity["hostname"])
}

func TestCurrentHostnameFromHeartbeat(t *testing.T) {
	assert.Equal(t, "", currentHostnameFromHeartbeat(nil))
	assert.Equal(t, "", currentHostnameFromHeartbeat(&operatorv1.HeartbeatResult{}))
	assert.Equal(t, "edge-node", currentHostnameFromHeartbeat(&operatorv1.HeartbeatResult{
		SystemIdentity: &operatorv1.SystemIdentity{Hostname: "edge-node"},
	}))
}
