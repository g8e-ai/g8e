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
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

func TestGatewayModeService_IsGovernanceReady(t *testing.T) {
	t.Run("Doctrine posture returns true without signers", func(t *testing.T) {
		ls := newTestGatewayService(t, testGatewayOpts{posture: config.PostureDoctrine})
		assert.True(t, ls.IsGovernanceReady())
	})

	t.Run("Empty posture returns true without signers", func(t *testing.T) {
		ls := newTestGatewayService(t, testGatewayOpts{})
		assert.True(t, ls.IsGovernanceReady())
	})

	t.Run("Notary posture returns false without signers", func(t *testing.T) {
		ls := newTestGatewayService(t, testGatewayOpts{posture: config.PostureNotary})
		assert.False(t, ls.IsGovernanceReady())
	})

	t.Run("Notary posture returns true with signers", func(t *testing.T) {
		ls := newTestGatewayService(t, testGatewayOpts{posture: config.PostureNotary})

		signer := map[string]interface{}{
			"id":         "test-signer-1",
			"public_key": "abc123",
			"added_at":   "2026-01-01T00:00:00Z",
			"enabled":    true,
		}
		signerBytes, err := json.Marshal(signer)
		require.NoError(t, err)
		err = ls.stores.DocStore.DocSet("trusted_signers", "test-signer-1", signerBytes)
		require.NoError(t, err)

		assert.True(t, ls.IsGovernanceReady())
	})
}

func TestGatewayModeService_GetGovernanceDeps(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})

	deps := ls.GetGovernanceDeps()
	assert.NotNil(t, deps)
	assert.NotNil(t, deps.ReplayStore)
	assert.NotNil(t, deps.StateRootProvider)
	assert.NotNil(t, deps.TransactionAudit)
	assert.NotNil(t, deps.L3Notary)
	assert.NotNil(t, deps.SignerStore)
	assert.NotNil(t, deps.AppPolicyStore)
	assert.NotNil(t, deps.FieldReader)
	assert.Equal(t, ls.stores.DocStore, deps.FieldReader)
	assert.NotNil(t, deps.Doctrine, "GovernanceDeps.Doctrine should be populated by GetGovernanceDeps")
}

func TestGatewayModeService_GetGovernanceDeps_L3MockWiring(t *testing.T) {
	t.Run("G8E_L3_MOCK=true returns demo notary that auto-approves", func(t *testing.T) {
		t.Setenv("G8E_L3_MOCK", "true")

		ls := newTestGatewayService(t, testGatewayOpts{})

		deps := ls.GetGovernanceDeps()
		require.NotNil(t, deps.L3Notary)

		proof := &commonv1.L3Proof{}
		ok, err := deps.L3Notary.VerifyL3Proof(context.Background(), "test-user", "test-hash", "", proof)
		assert.True(t, ok)
		assert.NoError(t, err)
	})

	t.Run("G8E_L3_MOCK unset returns gateway notary that requires passkey", func(t *testing.T) {
		t.Setenv("G8E_L3_MOCK", "")

		ls := newTestGatewayService(t, testGatewayOpts{})

		deps := ls.GetGovernanceDeps()
		require.NotNil(t, deps.L3Notary)

		proof := &commonv1.L3Proof{}
		ok, err := deps.L3Notary.VerifyL3Proof(context.Background(), "test-user", "test-hash", "", proof)
		assert.False(t, ok)
		assert.ErrorIs(t, err, constants.ErrPasskeyProofRequired)
	})
}
