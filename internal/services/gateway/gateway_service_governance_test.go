// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
		err = ls.GetDocStore().DocSet("trusted_signers", "test-signer-1", signerBytes)
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
	assert.NotNil(t, deps.GovernedDocStore, "GovernanceDeps.GovernedDocStore should be populated by GetGovernanceDeps")
	assert.NotNil(t, deps.L3Notary)
	assert.NotNil(t, deps.SignerStore)
	assert.NotNil(t, deps.FieldReader)
	assert.Equal(t, ls.GetDocStore(), deps.FieldReader)
	assert.NotNil(t, deps.Doctrine, "GovernanceDeps.Doctrine should be populated by GetGovernanceDeps")
}

func TestGatewayModeService_GetGovernanceDeps_AlwaysUsesRealNotary(t *testing.T) {
	t.Setenv("G8E_L3_MOCK", "true")

	ls := newTestGatewayService(t, testGatewayOpts{})

	deps := ls.GetGovernanceDeps()
	require.NotNil(t, deps.L3Notary)

	proof := &commonv1.L3Proof{}
	ok, err := deps.L3Notary.VerifyL3Proof(context.Background(), "test-user", "test-hash", "", proof)
	assert.False(t, ok)
	assert.ErrorIs(t, err, constants.ErrPasskeyProofRequired)
}
