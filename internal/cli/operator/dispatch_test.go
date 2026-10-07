// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operator

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestBuildExecuteBashDispatchRequest(t *testing.T) {
	request, err := BuildExecuteBashDispatchRequest(
		"4881d566-90a9-44c9-9e3e-c6bb51e07f5c",
		"echo hello",
		"exec-1",
		"cli-1",
	)
	require.NoError(t, err)
	assert.Equal(t, "4881d566-90a9-44c9-9e3e-c6bb51e07f5c", request.TargetOperatorSessionID)
	assert.Equal(t, string(constants.Event.Operator.Command.Requested), request.EventType)
	assert.Equal(t, "cli-1", request.CliSessionID)

	var payload operatorv1.CommandRequested
	require.NoError(t, proto.Unmarshal(request.Payload, &payload))
	assert.Equal(t, "echo hello", payload.Command)
	assert.Equal(t, "exec-1", payload.ExecutionId)
}

func TestBuildExecuteBashDispatchRequest_MissingFields(t *testing.T) {
	_, err := BuildExecuteBashDispatchRequest("", "echo", "exec-1", "cli-1")
	require.Error(t, err)
}

func TestParseCommandResult_EmptyPayload(t *testing.T) {
	_, err := ParseCommandResult(&DispatchResponse{
		Success:   true,
		EventType: "g8e.v1.operator.command.status.updated.running",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty command result payload")
}

func TestParseCommandResult_Success(t *testing.T) {
	payload, err := proto.Marshal(&operatorv1.CommandResult{
		Stdout:     "hello\n",
		ReturnCode: 0,
	})
	require.NoError(t, err)

	result, err := ParseCommandResult(&DispatchResponse{
		Success:       true,
		EventType:     "g8e.v1.operator.command.completed",
		ActionType:    string(constants.ActionTypeExecuteBash),
		ResultPayload: payload,
	})
	require.NoError(t, err)
	assert.Equal(t, "hello\n", result.Stdout)
	assert.Equal(t, int32(0), result.ReturnCode)
}

// truncatedProto is a length-delimited field whose declared length exceeds the
// bytes that follow, so proto.Unmarshal must reject it.
var truncatedProto = []byte{0x0a, 0x05, 'a'}

func TestMarshalExecuteBashPayload_RoundTripsEveryField(t *testing.T) {
	payload, err := MarshalExecuteBashPayload("ls -la /etc", "exec-9", "inspect config")
	require.NoError(t, err)

	var decoded operatorv1.CommandRequested
	require.NoError(t, proto.Unmarshal(payload, &decoded))

	assert.Equal(t, "ls -la /etc", decoded.Command)
	assert.Equal(t, "exec-9", decoded.ExecutionId)
	assert.Equal(t, "inspect config", decoded.Justification)
}

func TestBuildExecuteBashDispatchRequest_TrimsCommandAndStampsGovernedCLIMetadata(t *testing.T) {
	request, err := BuildExecuteBashDispatchRequest("session-1", "  \techo hi \n", "exec-1", "cli-1")
	require.NoError(t, err)

	var payload operatorv1.CommandRequested
	require.NoError(t, proto.Unmarshal(request.Payload, &payload))
	assert.Equal(t, "echo hi", payload.Command, "surrounding whitespace is not part of the command")
	assert.Equal(t, "g8e operator run", payload.Justification)
	assert.Equal(t, "cli", request.TargetResource)
}

func TestBuildExecuteBashDispatchRequest_RejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		command     string
		executionID string
	}{
		{name: "empty operator session", sessionID: "", command: "echo", executionID: "exec-1"},
		{name: "empty command", sessionID: "session-1", command: "", executionID: "exec-1"},
		{name: "whitespace-only command", sessionID: "session-1", command: " \t\n", executionID: "exec-1"},
		{name: "empty execution id", sessionID: "session-1", command: "echo", executionID: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := BuildExecuteBashDispatchRequest(tt.sessionID, tt.command, tt.executionID, "cli-1")

			require.ErrorIs(t, err, constants.ErrMissingRequiredField)
			assert.Equal(t, DispatchRequest{}, request, "no partial request may be returned on failure")
		})
	}
}

func TestBuildExecuteBashDispatchRequest_AllowsAnEmptyCLISessionAndOmitsItFromJSON(t *testing.T) {
	request, err := BuildExecuteBashDispatchRequest("session-1", "echo", "exec-1", "")
	require.NoError(t, err)

	encoded, err := json.Marshal(request)
	require.NoError(t, err)

	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	assert.NotContains(t, fields, "cli_session_id")
	assert.Equal(t, "session-1", fields["target_operator_session_id"])
	assert.Equal(t, "cli", fields["target_resource"])
	assert.Equal(t, string(constants.Event.Operator.Command.Requested), fields["event_type"])
	assert.NotEmpty(t, fields["payload"], "the protobuf payload travels base64 encoded")
}

func TestDecodeDispatchResponse_DecodesEveryField(t *testing.T) {
	raw := []byte(`{
		"success": true,
		"transaction_id": "tx-1",
		"event_type": "g8e.v1.operator.command.completed",
		"action_type": "EXECUTE_BASH",
		"result_payload": "AQID",
		"error": "none"
	}`)

	got, err := DecodeDispatchResponse(raw)

	require.NoError(t, err)
	assert.Equal(t, &DispatchResponse{
		Success:       true,
		TransactionID: "tx-1",
		EventType:     "g8e.v1.operator.command.completed",
		ActionType:    "EXECUTE_BASH",
		ResultPayload: []byte{1, 2, 3},
		Error:         "none",
	}, got)
}

func TestDecodeDispatchResponse_IgnoresFieldsTheCLIDoesNotModel(t *testing.T) {
	got, err := DecodeDispatchResponse([]byte(`{"success":false,"transaction_id":"tx-2","future_field":{"a":1}}`))

	require.NoError(t, err)
	assert.False(t, got.Success)
	assert.Equal(t, "tx-2", got.TransactionID)
}

func TestDecodeDispatchResponse_RejectsUnusableBodies(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		wantErr string
	}{
		{name: "nil body", raw: nil, wantErr: "empty dispatch response"},
		{name: "empty body", raw: []byte{}, wantErr: "empty dispatch response"},
		{name: "not json", raw: []byte("<html>502 Bad Gateway</html>"), wantErr: "decode dispatch response"},
		{name: "json array instead of object", raw: []byte(`[1,2]`), wantErr: "decode dispatch response"},
		{name: "wrong field type", raw: []byte(`{"success":"yes"}`), wantErr: "decode dispatch response"},
		{name: "payload that is not base64", raw: []byte(`{"result_payload":"***"}`), wantErr: "decode dispatch response"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeDispatchResponse(tt.raw)

			assert.Nil(t, got)
			require.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestDecodeCommandResult_ExtractsStdoutStderrAndReturnCode(t *testing.T) {
	payload, err := proto.Marshal(&operatorv1.CommandResult{
		ExecutionId: "exec-1",
		Stdout:      "out\n",
		Stderr:      "warn\n",
		ReturnCode:  2,
	})
	require.NoError(t, err)

	got, err := DecodeCommandResult(payload)

	require.NoError(t, err)
	assert.Equal(t, "exec-1", got.ExecutionId)
	assert.Equal(t, "out\n", got.Stdout)
	assert.Equal(t, "warn\n", got.Stderr)
	assert.Equal(t, int32(2), got.ReturnCode)
}

func TestDecodeCommandResult_RejectsEmptyAndCorruptPayloads(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		wantErr string
	}{
		{name: "nil payload", payload: nil, wantErr: "empty command result payload"},
		{name: "truncated protobuf", payload: truncatedProto, wantErr: "decode command result"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodeCommandResult(tt.payload)

			assert.Nil(t, got)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseCommandResult_NilResponseIsAMissingRequiredField(t *testing.T) {
	got, err := ParseCommandResult(nil)

	assert.Nil(t, got)
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestParseCommandResult_EmptyPayloadErrorIdentifiesTheOffendingEvent(t *testing.T) {
	_, err := ParseCommandResult(&DispatchResponse{
		Success:    true,
		EventType:  "g8e.v1.operator.command.status.updated.running",
		ActionType: string(constants.ActionTypeExecuteBash),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "event_type=g8e.v1.operator.command.status.updated.running")
	assert.Contains(t, err.Error(), "action_type="+string(constants.ActionTypeExecuteBash))
}

func TestParseCommandResult_CorruptPayloadWrapsTheProtobufError(t *testing.T) {
	_, err := ParseCommandResult(&DispatchResponse{ResultPayload: truncatedProto})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "operator dispatch: decode command result")
	assert.NotNil(t, errors.Unwrap(err), "the underlying protobuf error must stay reachable")
}

func TestSummarizeRunPercentilesUseSuccessfulDispatchesOnly(t *testing.T) {
	results := make([]RunResult, 0, 102)
	for i := 1; i <= 100; i++ {
		results = append(results, RunResult{Success: true, DurationMs: float64(i)})
	}
	results = append(results, RunResult{Success: false, DurationMs: 1}, RunResult{Success: false, DurationMs: 9000})

	summary := SummarizeRun(results, 2500*time.Millisecond, 16)

	assert.Equal(t, RunSummary{
		Targets:     102,
		Succeeded:   100,
		Failed:      2,
		WallMs:      2500,
		P50Ms:       50,
		P95Ms:       95,
		P99Ms:       99,
		MaxMs:       100,
		Concurrency: 16,
	}, summary)
}

func TestSummarizeRunWithoutSuccessReportsZeroLatency(t *testing.T) {
	summary := SummarizeRun([]RunResult{{DurationMs: 40}}, time.Second, 1)

	assert.Equal(t, 1, summary.Failed)
	assert.Zero(t, summary.Succeeded)
	assert.Zero(t, summary.P99Ms)
	assert.Zero(t, summary.MaxMs)
}
