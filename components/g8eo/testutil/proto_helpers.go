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

package testutil

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	operatorv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
)

// MustMarshalJSON marshals v to json.RawMessage, fatally failing the test on error.
func MustMarshalJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("MustMarshalJSON: %v", err)
	}
	return json.RawMessage(b)
}

// MustBuildCommandRequestedPayload builds a CommandRequested payload bytes.
func MustBuildCommandRequestedPayload(t *testing.T, cmd string, execID string, justification string, sentinelMode string, timeout int) []byte {
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

// MustBuildCommandCancelRequestedPayload builds a CommandCancelRequested payload bytes.
func MustBuildCommandCancelRequestedPayload(t *testing.T, execID string) []byte {
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

// MustUnmarshalPayload unmarshals bytes to a specific proto.Message, fatally failing the test on error.
func MustUnmarshalPayload(t *testing.T, data []byte, m proto.Message) {
	t.Helper()
	if err := proto.Unmarshal(data, m); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
}

// FileEditRequestFields is a helper struct for MustBuildFileEditRequestedPayload
type FileEditRequestFields struct {
	FilePath        string
	Operation       string
	ExecutionId     string
	Justification   string
	Content         string
	OldContent      string
	NewContent      string
	InsertContent   string
	InsertPosition  int32
	StartLine       int32
	EndLine         int32
	PatchContent    string
	CreateBackup    bool
	CreateIfMissing bool
}

// MustBuildFileEditRequestedPayload builds a FileEditRequested payload bytes.
func MustBuildFileEditRequestedPayload(t *testing.T, f FileEditRequestFields) []byte {
	t.Helper()
	protoFileEdit := &operatorv1.FileEditRequested{
		FilePath:        f.FilePath,
		Operation:       f.Operation,
		ExecutionId:     f.ExecutionId,
		Justification:   f.Justification,
		Content:         f.Content,
		OldContent:      f.OldContent,
		NewContent:      f.NewContent,
		InsertContent:   f.InsertContent,
		InsertPosition:  f.InsertPosition,
		StartLine:       f.StartLine,
		EndLine:         f.EndLine,
		PatchContent:    f.PatchContent,
		CreateBackup:    f.CreateBackup,
		CreateIfMissing: f.CreateIfMissing,
	}
	b, err := proto.Marshal(protoFileEdit)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FileEditRequested: %v", err)
	}
	return b
}

// MustBuildFsListRequestedPayload builds a FsListRequested payload bytes.
func MustBuildFsListRequestedPayload(t *testing.T, path string, execID string, maxEntries int32, maxDepth int32) []byte {
	t.Helper()
	protoFsList := &operatorv1.FsListRequested{
		Path:        path,
		ExecutionId: execID,
		MaxEntries:  maxEntries,
		MaxDepth:    maxDepth,
	}
	b, err := proto.Marshal(protoFsList)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FsListRequested: %v", err)
	}
	return b
}

// MustBuildCheckPortRequestedPayload builds a CheckPortRequested payload bytes.
func MustBuildCheckPortRequestedPayload(t *testing.T, host string, port int32, protocol string, execID string) []byte {
	t.Helper()
	p := &operatorv1.CheckPortRequested{
		ExecutionId: execID,
		Host:        host,
		Port:        port,
		Protocol:    protocol,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf CheckPortRequested: %v", err)
	}
	return b
}

// MustBuildFsReadRequestedPayload builds a FsReadRequested payload bytes.
func MustBuildFsReadRequestedPayload(t *testing.T, path string, execID string, maxSize int32) []byte {
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

// MustBuildFetchLogsRequestedPayload builds a FetchLogsRequested payload bytes.
func MustBuildFetchLogsRequestedPayload(t *testing.T, execID string) []byte {
	t.Helper()
	p := &operatorv1.FetchLogsRequested{
		ExecutionId: execID,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchLogsRequested: %v", err)
	}
	return b
}

// MustBuildFetchHistoryRequestedPayload builds a FetchHistoryRequested payload bytes.
func MustBuildFetchHistoryRequestedPayload(t *testing.T, execID string, sessionID string, limit int32, offset int32) []byte {
	t.Helper()
	p := &operatorv1.FetchHistoryRequested{
		ExecutionId:       execID,
		OperatorSessionId: sessionID,
		Limit:             limit,
		Offset:            offset,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchHistoryRequested: %v", err)
	}
	return b
}

// MustBuildFetchFileHistoryRequestedPayload builds a FetchFileHistoryRequested payload bytes.
func MustBuildFetchFileHistoryRequestedPayload(t *testing.T, execID string, filePath string, limit int32) []byte {
	t.Helper()
	p := &operatorv1.FetchFileHistoryRequested{
		ExecutionId: execID,
		FilePath:    filePath,
		Limit:       limit,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchFileHistoryRequested: %v", err)
	}
	return b
}

// MustBuildFetchFileDiffRequestedPayload builds a FetchFileDiffRequested payload bytes.
func MustBuildFetchFileDiffRequestedPayload(t *testing.T, execID string, filePath string) []byte {
	t.Helper()
	p := &operatorv1.FetchFileDiffRequested{
		ExecutionId: execID,
		FilePath:    filePath,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchFileDiffRequested: %v", err)
	}
	return b
}

// MustBuildRestoreFileRequestedPayload builds a RestoreFileRequested payload bytes.
func MustBuildRestoreFileRequestedPayload(t *testing.T, execID string, filePath string, commitHash string, sessionID string) []byte {
	t.Helper()
	p := &operatorv1.RestoreFileRequested{
		ExecutionId:       execID,
		FilePath:          filePath,
		CommitHash:        commitHash,
		OperatorSessionId: sessionID,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf RestoreFileRequested: %v", err)
	}
	return b
}

// MustBuildAuditMsgRequestedPayload builds an AuditMsgRequested payload bytes.
func MustBuildAuditMsgRequestedPayload(t *testing.T, content string) []byte {
	t.Helper()
	p := &operatorv1.AuditMsgRequested{
		Content: content,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf AuditMsgRequested: %v", err)
	}
	return b
}

// MustBuildDirectCommandAuditRequestedPayload builds a DirectCommandAuditRequested payload bytes.
func MustBuildDirectCommandAuditRequestedPayload(t *testing.T, cmd string, execID string, sessionID string, typeStr string) []byte {
	t.Helper()
	p := &operatorv1.DirectCommandAuditRequested{
		Command:           cmd,
		ExecutionId:       execID,
		OperatorSessionId: sessionID,
		Type:              typeStr,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf DirectCommandAuditRequested: %v", err)
	}
	return b
}

// MustBuildDirectCommandResultAuditRequestedPayload builds a DirectCommandResultAuditRequested payload bytes.
func MustBuildDirectCommandResultAuditRequestedPayload(t *testing.T, cmd string, execID string, output string, stderr string, exitCode int32, duration float32) []byte {
	t.Helper()
	p := &operatorv1.DirectCommandResultAuditRequested{
		Command:              cmd,
		ExecutionId:          execID,
		Output:               output,
		Stderr:               stderr,
		ExitCode:             exitCode,
		ExecutionTimeSeconds: duration,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf DirectCommandResultAuditRequested: %v", err)
	}
	return b
}

// MustBuildHeartbeatRequestedPayload builds an empty HeartbeatRequested payload bytes.
func MustBuildHeartbeatRequestedPayload(t *testing.T) []byte {
	t.Helper()
	protoHb := &operatorv1.HeartbeatRequested{}
	b, err := proto.Marshal(protoHb)
	if err != nil {
		t.Fatalf("failed to marshal protobuf HeartbeatRequested: %v", err)
	}
	return b
}

// MustBuildShutdownRequestedPayload builds a ShutdownRequested payload bytes.
func MustBuildShutdownRequestedPayload(t *testing.T, reason string) []byte {
	t.Helper()
	p := &operatorv1.ShutdownRequested{
		Reason: reason,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf ShutdownRequested: %v", err)
	}
	return b
}

// =============================================================================
// UAP Envelope Helpers (Phase 3 - JSON-first protocol)
// =============================================================================

// MustMarshalUAPEnvelope creates a UAPEnvelope JSON with the given fields.
func MustMarshalUAPEnvelope(t *testing.T, messageID string, protocolVersion string, senderID string, actionType string, targetResource string, dataFormat string, dataBlob string, requiredVotes int, caseID string, investigationID string, taskID *string) []byte {
	t.Helper()
	env := &uap.UAPEnvelope{
		ProtocolVersion: protocolVersion,
		Id:              messageID,
		OperatorId:      senderID,
		Timestamp:       timestamppb.Now(),
		ActionType:      actionType,
		TargetResource:  targetResource,
		Payload:         []byte(dataBlob),
		CaseId:          caseID,
		InvestigationId: investigationID,
	}
	if taskID != nil {
		env.TaskId = *taskID
	}

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal UAPEnvelope JSON: %v", err)
	}
	return b
}

// MustMarshalUAPEnvelopeWithVotes creates a UAPEnvelope JSON with pre-populated votes.
func MustMarshalUAPEnvelopeWithVotes(t *testing.T, messageID string, protocolVersion string, senderID string, actionType string, targetResource string, dataFormat string, dataBlob string, requiredVotes int, agentIDs []string, tribunalSig string) []byte {
	t.Helper()
	env := &uap.UAPEnvelope{
		ProtocolVersion: protocolVersion,
		Id:              messageID,
		OperatorId:      senderID,
		Timestamp:       timestamppb.Now(),
		ActionType:      actionType,
		TargetResource:  targetResource,
		Payload:         []byte(dataBlob),
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				AgentIds:          agentIDs,
				TribunalSignature: tribunalSig,
			},
		},
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal UAPEnvelope JSON with votes: %v", err)
	}
	return b
}

// MustUnmarshalUAPEnvelope unmarshals bytes to a UAPEnvelope, fatally failing the test on error.
func MustUnmarshalUAPEnvelope(t *testing.T, data []byte) *uap.UAPEnvelope {
	t.Helper()
	env := &uap.UAPEnvelope{}
	if err := json.Unmarshal(data, env); err != nil {
		t.Fatalf("failed to unmarshal UAPEnvelope JSON: %v", err)
	}
	return env
}

// MustGenerateUAPMessageID generates a deterministic MessageID from intent and context.
func MustGenerateUAPMessageID(t *testing.T, actionType string, targetResource string, dataFormat string, dataBlob string) string {
	t.Helper()
	env := &uap.UAPEnvelope{
		ActionType:     actionType,
		TargetResource: targetResource,
		Payload:        []byte(dataBlob),
		ExpiresAt:      timestamppb.New(time.Now().Add(5 * time.Minute)),
	}
	id, err := uap.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("failed to generate UAP MessageID: %v", err)
	}
	return id
}

// MustMarshalUAPEnvelopeWithNonce creates a UAPEnvelope JSON with the given fields and a nonce.
func MustMarshalUAPEnvelopeWithNonce(t *testing.T, messageID string, protocolVersion string, senderID string, actionType string, targetResource string, dataFormat string, dataBlob string, requiredVotes int, caseID string, investigationID string, taskID *string, nonce string) []byte {
	t.Helper()
	env := &uap.UAPEnvelope{
		ProtocolVersion: protocolVersion,
		Id:              messageID,
		OperatorId:      senderID,
		Timestamp:       timestamppb.Now(),
		ActionType:      actionType,
		TargetResource:  targetResource,
		Payload:         []byte(dataBlob),
		CaseId:          caseID,
		InvestigationId: investigationID,
		Nonce:           nonce,
	}
	if taskID != nil {
		env.TaskId = *taskID
	}

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal UAPEnvelope JSON: %v", err)
	}
	return b
}

// MustCreateUAPVote creates a slice of agent IDs for testing.
func MustCreateUAPVote(t *testing.T, nodeID string, signature string, decision bool) []string {
	t.Helper()
	return []string{nodeID}
}
