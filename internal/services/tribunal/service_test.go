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

package tribunal

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/response"
	govsvc "github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/pkg/governance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func bytesReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}

func newTestResponder() *response.Writer {
	return response.NewWriter(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func makeEnvelope(t *testing.T, actionType string, payload []byte) *governance.GovernanceEnvelope {
	t.Helper()
	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		ActionType:      actionType,
		TargetResource:  "localhost",
		Payload:         payload,
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
	}
	id, err := governance.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = id
	return env
}

func makeMembers(t *testing.T, n int) []TribunalMember {
	t.Helper()
	members := make([]TribunalMember, 0, n)
	for i := 0; i < n; i++ {
		_, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		members = append(members, TribunalMember{
			AppID:      "member-" + string(rune('1'+i)),
			PrivateKey: priv,
		})
	}
	return members
}

func TestTribunalService_Deliberate_HappyPath(t *testing.T) {
	t.Parallel()
	doctrine := govsvc.NewL1Doctrine()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))

	result, err := svc.Deliberate(env)
	require.NoError(t, err)

	require.NotNil(t, result.Envelope.Governance)
	require.NotNil(t, result.Envelope.Governance.L2)
	assert.Equal(t, "test-tribunal", result.Envelope.Governance.L2.TribunalId)
	assert.Len(t, result.Envelope.Governance.L2.Votes, 1)
	assert.Equal(t, "member-1", result.Envelope.Governance.L2.Votes[0].SignerKeyId)
	assert.NotEmpty(t, result.Envelope.Governance.L2.Votes[0].ConsensusSignature)
	assert.True(t, result.Envelope.Governance.L2.Votes[0].Decision)
}

func TestTribunalService_Deliberate_HashMismatch(t *testing.T) {
	t.Parallel()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))
	env.Id = "wrong-id"

	_, err := svc.Deliberate(env)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrTribunalHashMismatch))
}

func TestTribunalService_Deliberate_UnsafeCommand(t *testing.T) {
	t.Parallel()
	doctrine := govsvc.NewL1Doctrine()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeExecuteBash), []byte("rm -rf /"))

	result, err := svc.Deliberate(env)
	require.NoError(t, err)

	assert.Len(t, result.Envelope.Governance.L2.Votes, 1)
	assert.False(t, result.Envelope.Governance.L2.Votes[0].Decision)
}

func TestTribunalService_Deliberate_MultipleMembers(t *testing.T) {
	t.Parallel()
	doctrine := govsvc.NewL1Doctrine()
	members := makeMembers(t, 3)
	svc := NewTribunalService("test-tribunal", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))

	result, err := svc.Deliberate(env)
	require.NoError(t, err)

	assert.Len(t, result.Envelope.Governance.L2.Votes, 3)
	for i, vote := range result.Envelope.Governance.L2.Votes {
		assert.Equal(t, "member-"+string(rune('1'+i)), vote.SignerKeyId)
		assert.NotEmpty(t, vote.ConsensusSignature)
		assert.True(t, vote.Decision)
	}
}

func TestTribunalService_Deliberate_NilDoctrine_FailClosed(t *testing.T) {
	t.Parallel()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))

	result, err := svc.Deliberate(env)
	require.NoError(t, err)

	assert.False(t, result.Envelope.Governance.L2.Votes[0].Decision, "Expected fail-closed when Doctrine is nil")
}

func TestTribunalService_Deliberate_SignatureVerifiable(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	doctrine := govsvc.NewL1Doctrine()
	members := []TribunalMember{
		{AppID: "verifiable-member", PrivateKey: priv},
	}
	svc := NewTribunalService("test-tribunal", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))

	result, err := svc.Deliberate(env)
	require.NoError(t, err)

	vote := result.Envelope.Governance.L2.Votes[0]
	sigBytes, err := hex.DecodeString(vote.ConsensusSignature)
	require.NoError(t, err)

	payload := env.Id + "|true"
	assert.True(t, ed25519.Verify(pub, []byte(payload), sigBytes))
}

func TestTribunalService_Deliberate_WithIntentData(t *testing.T) {
	t.Parallel()
	doctrine := govsvc.NewL1Doctrine()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	intentData, err := structpb.NewStruct(map[string]interface{}{
		string(constants.ApprovalTypeIntent): "test-intent",
		"other_field":                        "value",
	})
	require.NoError(t, err)

	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		ActionType:      string(constants.ActionTypeExecuteBash),
		TargetResource:  "localhost",
		IntentData:      intentData,
		Nonce:           "nonce-123",
		StateMerkleRoot: "root-abc",
	}
	id, err := governance.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = id

	result, err := svc.Deliberate(env)
	require.NoError(t, err)
	assert.True(t, result.Envelope.Governance.L2.Votes[0].Decision)
}

func TestTribunalService_Deliberate_NilPrivateKey_Skipped(t *testing.T) {
	t.Parallel()
	doctrine := govsvc.NewL1Doctrine()
	members := []TribunalMember{
		{AppID: "keyless-member", PrivateKey: nil},
	}
	svc := NewTribunalService("test-tribunal", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))

	result, err := svc.Deliberate(env)
	require.NoError(t, err)
	assert.Empty(t, result.Envelope.Governance.L2.Votes, "nil-key members should be skipped, producing zero votes")
}

func TestTribunalService_Deliberate_InitializesGovernance(t *testing.T) {
	t.Parallel()
	doctrine := govsvc.NewL1Doctrine()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))
	require.Nil(t, env.Governance)

	result, err := svc.Deliberate(env)
	require.NoError(t, err)

	require.NotNil(t, result.Envelope.Governance)
	require.NotNil(t, result.Envelope.Governance.L1)
	require.NotNil(t, result.Envelope.Governance.L2)
	require.NotNil(t, result.Envelope.Governance.L3)
}

func TestTribunalService_HandleDeliberate_HTTP_HappyPath(t *testing.T) {
	t.Parallel()
	doctrine := govsvc.NewL1Doctrine()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))
	body, err := protojson.Marshal(env)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tribunal/v1/deliberate", bytesReader(body))
	rr := httptest.NewRecorder()

	svc.HandleDeliberate(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resultEnv governance.GovernanceEnvelope
	err = protojson.Unmarshal(rr.Body.Bytes(), &resultEnv)
	require.NoError(t, err)

	require.NotNil(t, resultEnv.Governance)
	require.NotNil(t, resultEnv.Governance.L2)
	assert.Equal(t, "test-tribunal", resultEnv.Governance.L2.TribunalId)
	assert.Len(t, resultEnv.Governance.L2.Votes, 1)
}

func TestTribunalService_HandleDeliberate_HTTP_HashMismatch(t *testing.T) {
	t.Parallel()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))
	env.Id = "wrong-id"
	body, err := protojson.Marshal(env)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/tribunal/v1/deliberate", bytesReader(body))
	rr := httptest.NewRecorder()

	svc.HandleDeliberate(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), constants.ErrTribunalHashMismatch.Error())
}

func TestTribunalService_HandleDeliberate_HTTP_InvalidJSON(t *testing.T) {
	t.Parallel()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	req := httptest.NewRequest(http.MethodPost, "/tribunal/v1/deliberate", bytesReader([]byte("not json")))
	rr := httptest.NewRecorder()

	svc.HandleDeliberate(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestTribunalService_HandleDeliberate_HTTP_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	members := makeMembers(t, 1)
	svc := NewTribunalService("test-tribunal", members, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	req := httptest.NewRequest(http.MethodGet, "/tribunal/v1/deliberate", nil)
	rr := httptest.NewRecorder()

	svc.HandleDeliberate(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestTribunalService_TribunalID(t *testing.T) {
	t.Parallel()
	svc := NewTribunalService("my-tribunal", nil, nil, nil, nil)
	assert.Equal(t, "my-tribunal", svc.TribunalID())
}
