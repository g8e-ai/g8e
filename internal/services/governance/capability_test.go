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
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	govtypes "github.com/g8e-ai/g8e/internal/governance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMintCapability_BindsEnvelopeFields(t *testing.T) {
	t.Parallel()

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	expiresAt := time.Now().Add(5 * time.Minute)
	vt := &VerifiedTransaction{
		Envelope: &govtypes.GovernanceEnvelope{
			TransactionHash:   "abc123hash",
			OperatorId:        "op-1",
			OperatorSessionId: "sess-1",
			TargetResource:    "localhost",
		},
		ActionType: constants.ActionTypeExecuteBash,
		ExpiresAt:  expiresAt,
	}

	cap, err := MintCapability(vt, privKey, "l5-key-1")
	require.NoError(t, err)
	require.NotNil(t, cap)

	assert.Equal(t, "abc123hash", cap.TransactionHash)
	assert.Equal(t, constants.ActionTypeExecuteBash, cap.ActionType)
	assert.Equal(t, "localhost", cap.TargetResource)
	assert.Equal(t, "op-1", cap.OperatorID)
	assert.Equal(t, "sess-1", cap.OperatorSession)
	assert.Equal(t, "l5-key-1", cap.KeyID)
	assert.Equal(t, expiresAt.Unix(), cap.ExpiresAt.Unix())
	assert.NotEmpty(t, cap.Token)
	assert.False(t, cap.IsDissolved(), "freshly minted capability should not be dissolved")
}

func TestMintCapability_NilVT(t *testing.T) {
	t.Parallel()

	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	_, err = MintCapability(nil, privKey, "l5-key-1")
	require.Error(t, err)
}

func TestCapability_Dissolve(t *testing.T) {
	t.Parallel()

	cap := &Capability{
		TransactionHash: "hash-1",
		ActionType:      constants.ActionTypeExecuteBash,
		ExpiresAt:       time.Now().Add(5 * time.Minute),
	}

	require.False(t, cap.IsDissolved())
	require.True(t, cap.IsValid(time.Now()))

	cap.Dissolve()

	require.True(t, cap.IsDissolved())
	require.False(t, cap.IsValid(time.Now()))
}

func TestCapability_Expired(t *testing.T) {
	t.Parallel()

	cap := &Capability{
		TransactionHash: "hash-1",
		ActionType:      constants.ActionTypeExecuteBash,
		ExpiresAt:       time.Now().Add(-1 * time.Minute), // already expired
	}

	require.True(t, cap.IsExpired(time.Now()))
	require.False(t, cap.IsValid(time.Now()))
}

func TestCapability_Verify_Success(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cap := &Capability{
		TransactionHash: "hash-1",
		ActionType:      constants.ActionTypeExecuteBash,
		ExpiresAt:       now.Add(5 * time.Minute),
	}

	err := cap.Verify(constants.ActionTypeExecuteBash, "hash-1", now)
	require.NoError(t, err)
}

func TestCapability_Verify_ActionMismatch(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cap := &Capability{
		TransactionHash: "hash-1",
		ActionType:      constants.ActionTypeExecuteBash,
		ExpiresAt:       now.Add(5 * time.Minute),
	}

	err := cap.Verify(constants.ActionTypeFileEdit, "hash-1", now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "action type mismatch")
}

func TestCapability_Verify_HashMismatch(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cap := &Capability{
		TransactionHash: "hash-1",
		ActionType:      constants.ActionTypeExecuteBash,
		ExpiresAt:       now.Add(5 * time.Minute),
	}

	err := cap.Verify(constants.ActionTypeExecuteBash, "wrong-hash", now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction hash mismatch")
}

func TestCapability_Verify_Dissolved(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cap := &Capability{
		TransactionHash: "hash-1",
		ActionType:      constants.ActionTypeExecuteBash,
		ExpiresAt:       now.Add(5 * time.Minute),
	}
	cap.Dissolve()

	err := cap.Verify(constants.ActionTypeExecuteBash, "hash-1", now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dissolved")
}

func TestCapability_Verify_Expired(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cap := &Capability{
		TransactionHash: "hash-1",
		ActionType:      constants.ActionTypeExecuteBash,
		ExpiresAt:       now.Add(-1 * time.Minute),
	}

	err := cap.Verify(constants.ActionTypeExecuteBash, "hash-1", now)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestCapabilityFromContext(t *testing.T) {
	t.Parallel()

	t.Run("present", func(t *testing.T) {
		t.Parallel()
		cap := &Capability{TransactionHash: "h1"}
		ctx := ContextWithCapability(context.Background(), cap)

		extracted := CapabilityFromContext(ctx)
		require.NotNil(t, extracted)
		assert.Equal(t, "h1", extracted.TransactionHash)
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()
		extracted := CapabilityFromContext(context.Background())
		require.Nil(t, extracted)
	})
}

func TestL5Actuator_MintsAndDissolvesCapability(t *testing.T) {
	t.Parallel()

	actuator, _ := newTestActuator(t)

	var capturedCap *Capability
	handler := actuator.ExecutionHandler.(*mockExecutionHandler)
	handler.ExecuteVerifiedTransactionFunc = func(ctx context.Context, _ constants.EventType, _ CommandMessage) (string, error) {
		capturedCap = CapabilityFromContext(ctx)
		return "ok", nil
	}

	envelope := &govtypes.GovernanceEnvelope{
		TransactionHash:   "test-hash-cap-123",
		OperatorId:        "op-1",
		OperatorSessionId: "sess-1",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
		ExpiresAt:  time.Now().Add(5 * time.Minute),
	}

	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// During execution, the handler should have received a valid capability
	require.NotNil(t, capturedCap, "handler should have received a capability via context")
	assert.Equal(t, "test-hash-cap-123", capturedCap.TransactionHash)
	assert.Equal(t, constants.ActionTypeExecuteBash, capturedCap.ActionType)

	// After execution, the capability should be dissolved
	assert.True(t, capturedCap.IsDissolved(), "capability must be dissolved after execution")
}

func TestL5Actuator_CapabilityDissolvedOnHandlerError(t *testing.T) {
	t.Parallel()

	actuator, _ := newTestActuator(t)

	var capturedCap *Capability
	handler := actuator.ExecutionHandler.(*mockExecutionHandler)
	handler.err = nil // set below via func
	handler.ExecuteVerifiedTransactionFunc = func(ctx context.Context, _ constants.EventType, _ CommandMessage) (string, error) {
		capturedCap = CapabilityFromContext(ctx)
		return "", errors.New("simulated execution failure")
	}

	envelope := &govtypes.GovernanceEnvelope{
		TransactionHash:   "test-hash-cap-err",
		OperatorId:        "op-1",
		OperatorSessionId: "sess-1",
		ActionType:        string(constants.ActionTypeExecuteBash),
		TargetResource:    "localhost",
	}

	vt := &VerifiedTransaction{
		Envelope:   envelope,
		ActionType: constants.ActionTypeExecuteBash,
		ExpiresAt:  time.Now().Add(5 * time.Minute),
	}

	receipt, err := actuator.Execute(context.Background(), vt, nil)
	require.Error(t, err)
	require.NotNil(t, receipt)

	// Capability should still be dissolved even on handler error
	require.NotNil(t, capturedCap)
	assert.True(t, capturedCap.IsDissolved(), "capability must be dissolved even when handler fails")
}
