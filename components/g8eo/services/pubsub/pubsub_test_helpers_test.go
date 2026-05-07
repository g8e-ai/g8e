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

package pubsub

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
)

// mustMarshalJSON marshals v to json.RawMessage, fatally failing the test on error.
func mustMarshalJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshalJSON: %v", err)
	}
	return json.RawMessage(b)
}

// mustMarshalProtobufCommandRequested marshals a CommandRequested protobuf to bytes.
func mustMarshalProtobufCommandRequested(t *testing.T, cmd string, execID string, justification string, sentinelMode string, timeout int) []byte {
	t.Helper()
	protoCmd := &operatorv1.CommandRequested{
		Command:        cmd,
		ExecutionId:    execID,
		Justification:  justification,
		SentinelMode:   sentinelMode,
		TimeoutSeconds: int32(timeout),
	}
	b, err := proto.Marshal(protoCmd)
	if err != nil {
		t.Fatalf("failed to marshal protobuf CommandRequested: %v", err)
	}
	return b
}

// mustMarshalProtobufCommandCancelRequested marshals a CommandCancelRequested protobuf to bytes.
func mustMarshalProtobufCommandCancelRequested(t *testing.T, execID string) []byte {
	t.Helper()
	protoCancel := &operatorv1.CommandCancelRequested{
		ExecutionId: execID,
	}
	b, err := proto.Marshal(protoCancel)
	if err != nil {
		t.Fatalf("failed to marshal protobuf CommandCancelRequested: %v", err)
	}
	return b
}

// mustMarshalUniversalEnvelope creates a UniversalEnvelope protobuf with the given payload.
func mustMarshalUniversalEnvelope(t *testing.T, id string, eventType string, payload []byte, taskID string, operatorID string, caseID string, investigationID string, operatorSessionId string) []byte {
	t.Helper()
	env := &commonv1.UniversalEnvelope{
		Id:                id,
		Timestamp:         timestamppb.Now(),
		EventType:         eventType,
		SourceComponent:   commonv1.Component_COMPONENT_G8EE,
		OperatorId:        operatorID,
		OperatorSessionId: operatorSessionId,
		CaseId:            caseID,
		InvestigationId:   investigationID,
		TaskId:            taskID,
		Payload:           payload,
	}
	b, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal protobuf UniversalEnvelope: %v", err)
	}
	return b
}

// mustMarshalProtobufFileEditRequested marshals a FileEditRequested protobuf to bytes.
func mustMarshalProtobufFileEditRequested(t *testing.T, filePath string, operation string, execID string, justification string, content string, createIfMissing bool) []byte {
	t.Helper()
	protoFileEdit := &operatorv1.FileEditRequested{
		FilePath:        filePath,
		Operation:       operation,
		ExecutionId:     execID,
		Justification:   justification,
		Content:         content,
		CreateIfMissing: createIfMissing,
	}
	b, err := proto.Marshal(protoFileEdit)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FileEditRequested: %v", err)
	}
	return b
}

// mustMarshalProtobufFsListRequested marshals a FsListRequested protobuf to bytes.
func mustMarshalProtobufFsListRequested(t *testing.T, path string, execID string, maxEntries int32) []byte {
	t.Helper()
	protoFsList := &operatorv1.FsListRequested{
		Path:        path,
		ExecutionId: execID,
		MaxEntries:  maxEntries,
	}
	b, err := proto.Marshal(protoFsList)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FsListRequested: %v", err)
	}
	return b
}

// mustMarshalProtobufFsReadRequested marshals a FsReadRequested protobuf to bytes.
func mustMarshalProtobufFsReadRequested(t *testing.T, path string, execID string, maxSize int32) []byte {
	t.Helper()
	protoFsRead := &operatorv1.FsReadRequested{
		Path:        path,
		ExecutionId: execID,
		MaxSize:     maxSize,
	}
	b, err := proto.Marshal(protoFsRead)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FsReadRequested: %v", err)
	}
	return b
}

// mustMarshalProtobufHeartbeatRequested marshals an empty HeartbeatRequested protobuf to bytes.
func mustMarshalProtobufHeartbeatRequested(t *testing.T) []byte {
	t.Helper()
	protoHb := &operatorv1.HeartbeatRequested{}
	b, err := proto.Marshal(protoHb)
	if err != nil {
		t.Fatalf("failed to marshal protobuf HeartbeatRequested: %v", err)
	}
	return b
}
