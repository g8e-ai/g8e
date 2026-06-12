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

package governance

import (
	"crypto/ed25519"
	"encoding/hex"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/pkg/governance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestL2Consensus_EvaluatePayload_HappyPath(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	doctrine := NewL1Doctrine()
	consensus := NewL2Consensus("test-node", doctrine, priv)

	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		ActionType:      string(constants.ActionTypeFetchLogs),
		TargetResource:  "localhost",
		Payload:         []byte("fetch logs"),
	}

	id, err := governance.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = id

	err = consensus.EvaluatePayload(env)
	require.NoError(t, err)

	require.NotNil(t, env.Governance)
	require.NotNil(t, env.Governance.L2)
	assert.Equal(t, []string{"test-node"}, env.Governance.L2.AgentIds)
	assert.NotEmpty(t, env.Governance.L2.ConsensusSignature)
	// Validated is not set by appendVote on safe path; callers set it before L5
	assert.Empty(t, env.Governance.L1.Violations)

	// Verify signature is valid
	sigBytes, err := hex.DecodeString(env.Governance.L2.ConsensusSignature)
	require.NoError(t, err)
	payload := env.Id + "|true"
	assert.True(t, ed25519.Verify(pub, []byte(payload), sigBytes))
}

func TestL2Consensus_EvaluatePayload_HashMismatch(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	consensus := NewL2Consensus("test-node", nil, priv)

	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		ActionType:      string(constants.ActionTypeFetchLogs),
		TargetResource:  "localhost",
		Payload:         []byte("fetch logs"),
		Id:              "wrong-id",
	}

	err = consensus.EvaluatePayload(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payload hash mismatch")
}

func TestL2Consensus_EvaluatePayload_UnsafeCommand(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	doctrine := NewL1Doctrine()
	consensus := NewL2Consensus("test-node", doctrine, priv)

	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		ActionType:      string(constants.ActionTypeExecuteBash),
		TargetResource:  "localhost",
		Payload:         []byte("rm -rf /"),
	}

	id, err := governance.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = id

	err = consensus.EvaluatePayload(env)
	require.NoError(t, err)

	require.NotNil(t, env.Governance)
	assert.False(t, env.Governance.L1.Validated)
	assert.Contains(t, env.Governance.L1.Violations, "MITRE_CHECK_FAILED")
}

func TestL2Consensus_RunMITREChecks_NilDoctrine(t *testing.T) {
	t.Parallel()
	consensus := NewL2Consensus("test-node", nil, nil)
	isSafe := consensus.RunMITREChecks("test", "echo hello")
	assert.False(t, isSafe, "Expected fail-closed when Doctrine is nil")
}

func TestL2Consensus_RunMITREChecks_SafeCommand(t *testing.T) {
	t.Parallel()
	doctrine := NewL1Doctrine()
	consensus := NewL2Consensus("test-node", doctrine, nil)
	isSafe := consensus.RunMITREChecks("test", "ls -la")
	assert.True(t, isSafe, "Expected safe command to pass MITRE checks")
}

func TestL2Consensus_RunMITREChecks_UnsafeCommand(t *testing.T) {
	t.Parallel()
	doctrine := NewL1Doctrine()
	consensus := NewL2Consensus("test-node", doctrine, nil)
	isSafe := consensus.RunMITREChecks("test", "rm -rf /")
	assert.False(t, isSafe, "Expected unsafe command to fail MITRE checks")
}

func TestL2Consensus_SignDecision_ValidKey(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	consensus := NewL2Consensus("test-node", nil, priv)
	sig, err := consensus.SignDecision("test-msg-id", true)
	require.NoError(t, err)
	assert.NotEmpty(t, sig)

	sigBytes, err := hex.DecodeString(sig)
	require.NoError(t, err)
	assert.True(t, ed25519.Verify(pub, []byte("test-msg-id|true"), sigBytes))
}

func TestL2Consensus_SignDecision_NilKey(t *testing.T) {
	t.Parallel()
	consensus := NewL2Consensus("test-node", nil, nil)
	_, err := consensus.SignDecision("test-msg-id", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private key missing")
}

func TestL2Consensus_extractCommandData_WithIntentData(t *testing.T) {
	t.Parallel()
	consensus := NewL2Consensus("test-node", nil, nil)

	intentData, err := structpb.NewStruct(map[string]interface{}{
		string(constants.ApprovalTypeIntent): "test-intent",
		"other_field":                        "value",
	})
	require.NoError(t, err)

	env := &governance.GovernanceEnvelope{
		ActionType: string(constants.ActionTypeGrantIntent),
		IntentData: intentData,
	}

	cmdData, intent, err := consensus.extractCommandData(env)
	require.NoError(t, err)
	assert.Contains(t, cmdData, "test-intent")
	assert.Equal(t, constants.CloudIntent("test-intent"), intent)
}

func TestL2Consensus_extractCommandData_WithPayload(t *testing.T) {
	t.Parallel()
	consensus := NewL2Consensus("test-node", nil, nil)

	env := &governance.GovernanceEnvelope{
		Payload: []byte("plain payload"),
	}

	cmdData, intent, err := consensus.extractCommandData(env)
	require.NoError(t, err)
	assert.Equal(t, "plain payload", cmdData)
	assert.Empty(t, intent)
}

func TestL2Consensus_evaluateSafety_WithInvalidIntent(t *testing.T) {
	t.Parallel()
	doctrine := NewL1Doctrine()
	consensus := NewL2Consensus("test-node", doctrine, nil)

	// ValidateIntent currently returns true for all intents (temporary bridge),
	// so this test documents current behavior. If intent validation is tightened,
	// this test should be updated.
	isSafe := consensus.evaluateSafety("test", "ls -la", constants.CloudIntent("any-intent"))
	assert.True(t, isSafe)
}

func TestL2Consensus_evaluateSafety_WithoutIntent(t *testing.T) {
	t.Parallel()
	doctrine := NewL1Doctrine()
	consensus := NewL2Consensus("test-node", doctrine, nil)

	isSafe := consensus.evaluateSafety("test", "ls -la", "")
	assert.True(t, isSafe)
}

func TestL2Consensus_appendVote_UnsafeSetsViolations(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	consensus := NewL2Consensus("test-node", nil, priv)
	env := &governance.GovernanceEnvelope{Id: "test-id"}

	err = consensus.appendVote(env, false)
	require.NoError(t, err)

	require.NotNil(t, env.Governance)
	assert.False(t, env.Governance.L1.Validated)
	assert.Contains(t, env.Governance.L1.Violations, "MITRE_CHECK_FAILED")
}

func TestL2Consensus_appendVote_SafeNoViolations(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	consensus := NewL2Consensus("test-node", nil, priv)
	env := &governance.GovernanceEnvelope{Id: "test-id"}

	err = consensus.appendVote(env, true)
	require.NoError(t, err)

	require.NotNil(t, env.Governance)
	// Validated is not set by appendVote on safe path; callers set it before L5
	assert.Empty(t, env.Governance.L1.Violations)
	assert.NotEmpty(t, env.Governance.L2.ConsensusSignature)
}

func TestL2Consensus_appendVote_InitializesGovernance(t *testing.T) {
	t.Parallel()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	consensus := NewL2Consensus("test-node", nil, priv)
	env := &governance.GovernanceEnvelope{Id: "test-id"}
	require.Nil(t, env.Governance)

	err = consensus.appendVote(env, true)
	require.NoError(t, err)

	require.NotNil(t, env.Governance)
	require.NotNil(t, env.Governance.L1)
	require.NotNil(t, env.Governance.L2)
	require.NotNil(t, env.Governance.L3)
}

func TestL2Consensus_verifyPayloadHash_Valid(t *testing.T) {
	t.Parallel()
	consensus := NewL2Consensus("test-node", nil, nil)

	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		Payload:         []byte("test"),
	}

	id, err := governance.GenerateMessageID(env)
	require.NoError(t, err)
	env.Id = id

	err = consensus.verifyPayloadHash(env)
	require.NoError(t, err)
}

func TestL2Consensus_verifyPayloadHash_Mismatch(t *testing.T) {
	t.Parallel()
	consensus := NewL2Consensus("test-node", nil, nil)

	env := &governance.GovernanceEnvelope{
		ProtocolVersion: "1.0",
		OperatorId:      "agent-1",
		Timestamp:       timestamppb.Now(),
		Payload:         []byte("test"),
		Id:              "wrong-hash",
	}

	err := consensus.verifyPayloadHash(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "payload hash mismatch")
}
