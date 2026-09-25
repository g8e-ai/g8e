// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

var heartbeatSnapshotMarshalOptions = protojson.MarshalOptions{UseProtoNames: true}

// heartbeatResultFromEnvelope decodes the canonical HeartbeatResult carried on a
// GovernanceEnvelope. The binary payload field is authoritative; intent_data is a
// protojson fallback for envelopes that omit payload bytes.
func heartbeatResultFromEnvelope(env *commonv1.GovernanceEnvelope) (*operatorv1.HeartbeatResult, error) {
	if env == nil {
		return nil, fmt.Errorf("heartbeat envelope is required")
	}

	heartbeat := &operatorv1.HeartbeatResult{}
	if len(env.Payload) > 0 {
		if err := proto.Unmarshal(env.Payload, heartbeat); err != nil {
			return nil, fmt.Errorf("heartbeat payload decode failed: %w", err)
		}
		return heartbeat, nil
	}

	if env.IntentData == nil || len(env.IntentData.Fields) == 0 {
		return heartbeat, nil
	}

	intentJSON, err := protojson.Marshal(env.IntentData)
	if err != nil {
		return nil, fmt.Errorf("heartbeat intent_data marshal failed: %w", err)
	}
	if err := protojson.Unmarshal(intentJSON, heartbeat); err != nil {
		return nil, fmt.Errorf("heartbeat intent_data decode failed: %w", err)
	}
	return heartbeat, nil
}

func marshalHeartbeatSnapshot(heartbeat *operatorv1.HeartbeatResult) (json.RawMessage, error) {
	if heartbeat == nil {
		return nil, fmt.Errorf("heartbeat result is required")
	}
	snapshotBytes, err := heartbeatSnapshotMarshalOptions.Marshal(heartbeat)
	if err != nil {
		return nil, fmt.Errorf("heartbeat snapshot marshal failed: %w", err)
	}
	return json.RawMessage(snapshotBytes), nil
}

func currentHostnameFromHeartbeat(heartbeat *operatorv1.HeartbeatResult) string {
	if heartbeat == nil || heartbeat.SystemIdentity == nil {
		return ""
	}
	return heartbeat.SystemIdentity.Hostname
}
