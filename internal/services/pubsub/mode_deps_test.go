// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/consensus"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func validTestGovernanceCoreDeps() GovernanceCoreDeps {
	return GovernanceCoreDeps{
		ReplayStore:       &testutil.MockReplayStore{},
		StateRootProvider: testutil.NewMockStateRootProvider("test-root"),
		TransactionAudit:  &testutil.MockTransactionAudit{},
		L3Notary:          &testutil.MockL3Notary{},
		SignerStore:       &governance.FailClosedSignerStore{Signers: make(map[string]ed25519.PublicKey)},
		Doctrine:          governance.NewL1Doctrine(),
	}
}

func validTestGatewayModeDeps(posture config.GatewayPosture) GatewayModeDeps {
	deps := GatewayModeDeps{
		GovernanceCoreDeps:     validTestGovernanceCoreDeps(),
		GovernedDocStore:       &testutil.ConfigurableMockGovernedDocStore{},
		ConsensusPolicyStore:   testConsensusStore(),
		FieldReader:            mcp.NoopFieldReader{},
		PlatformEnrollmentDeps: &PlatformEnrollmentDeps{},
		Posture:                posture,
	}
	if posture != config.PostureDoctrine {
		deps.Consensus = &consensus.ConsensusService{}
	}
	return deps
}

func TestNewOutboundModeDeps_Success(t *testing.T) {
	t.Parallel()

	input := OutboundModeDeps{
		GovernanceCoreDeps: validTestGovernanceCoreDeps(),
	}

	deps, err := NewOutboundModeDeps(input)
	require.NoError(t, err)
	require.NotNil(t, deps)
	assert.Equal(t, input.ReplayStore, deps.ReplayStore)
	assert.Equal(t, input.StateRootProvider, deps.StateRootProvider)
	assert.Equal(t, input.TransactionAudit, deps.TransactionAudit)
	assert.Equal(t, input.L3Notary, deps.L3Notary)
	assert.Equal(t, input.SignerStore, deps.SignerStore)
	assert.Equal(t, input.Doctrine, deps.Doctrine)
}

func TestNewOutboundModeDeps_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modify      func(*GovernanceCoreDeps)
		expectedErr error
	}{
		{
			name: "nil ReplayStore returns ErrReplayStoreRequired",
			modify: func(d *GovernanceCoreDeps) {
				d.ReplayStore = nil
			},
			expectedErr: constants.ErrReplayStoreRequired,
		},
		{
			name: "nil StateRootProvider returns ErrStateRootProviderRequired",
			modify: func(d *GovernanceCoreDeps) {
				d.StateRootProvider = nil
			},
			expectedErr: constants.ErrStateRootProviderRequired,
		},
		{
			name: "nil TransactionAudit returns ErrTransactionAuditRequired",
			modify: func(d *GovernanceCoreDeps) {
				d.TransactionAudit = nil
			},
			expectedErr: constants.ErrTransactionAuditRequired,
		},
		{
			name: "nil L3Notary returns ErrL3NotaryRequired",
			modify: func(d *GovernanceCoreDeps) {
				d.L3Notary = nil
			},
			expectedErr: constants.ErrL3NotaryRequired,
		},
		{
			name: "nil SignerStore returns ErrSignerStoreRequired",
			modify: func(d *GovernanceCoreDeps) {
				d.SignerStore = nil
			},
			expectedErr: constants.ErrSignerStoreRequired,
		},
		{
			name: "nil Doctrine returns ErrDoctrineRequired",
			modify: func(d *GovernanceCoreDeps) {
				d.Doctrine = nil
			},
			expectedErr: constants.ErrDoctrineRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			core := validTestGovernanceCoreDeps()
			tc.modify(&core)
			deps, err := NewOutboundModeDeps(OutboundModeDeps{GovernanceCoreDeps: core})
			require.Error(t, err)
			require.Nil(t, deps)
			assert.True(t, errors.Is(err, tc.expectedErr), "expected error %v, got %v", tc.expectedErr, err)
		})
	}
}

func TestNewGatewayModeDeps_Success(t *testing.T) {
	t.Parallel()

	postures := []struct {
		name              string
		posture           config.GatewayPosture
		requiresConsensus bool
	}{
		{name: "doctrine posture (consensus nil)", posture: config.PostureDoctrine},
		{name: "consensus posture (consensus non-nil)", posture: config.PostureConsensus, requiresConsensus: true},
		{name: "ratify posture (consensus nil)", posture: config.PostureRatify},
		{name: "notary posture (consensus non-nil)", posture: config.PostureNotary, requiresConsensus: true},
	}

	for _, p := range postures {
		t.Run(p.name, func(t *testing.T) {
			input := validTestGatewayModeDeps(p.posture)
			if !p.requiresConsensus {
				input.Consensus = nil
			}
			deps, err := NewGatewayModeDeps(input)
			require.NoError(t, err)
			require.NotNil(t, deps)
			assert.Equal(t, p.posture, deps.Posture)
			assert.Equal(t, input.GovernedDocStore, deps.GovernedDocStore)
			assert.Equal(t, input.ConsensusPolicyStore, deps.ConsensusPolicyStore)
			assert.Equal(t, input.FieldReader, deps.FieldReader)
			assert.Equal(t, input.PlatformEnrollmentDeps, deps.PlatformEnrollmentDeps)
			if p.requiresConsensus {
				assert.NotNil(t, deps.Consensus)
			} else {
				assert.Nil(t, deps.Consensus)
			}
		})
	}
}

func TestNewGatewayModeDeps_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modify      func(*GatewayModeDeps)
		expectedErr error
	}{
		{
			name: "nil ReplayStore returns ErrReplayStoreRequired",
			modify: func(d *GatewayModeDeps) {
				d.ReplayStore = nil
			},
			expectedErr: constants.ErrReplayStoreRequired,
		},
		{
			name: "nil StateRootProvider returns ErrStateRootProviderRequired",
			modify: func(d *GatewayModeDeps) {
				d.StateRootProvider = nil
			},
			expectedErr: constants.ErrStateRootProviderRequired,
		},
		{
			name: "nil TransactionAudit returns ErrTransactionAuditRequired",
			modify: func(d *GatewayModeDeps) {
				d.TransactionAudit = nil
			},
			expectedErr: constants.ErrTransactionAuditRequired,
		},
		{
			name: "nil L3Notary returns ErrL3NotaryRequired",
			modify: func(d *GatewayModeDeps) {
				d.L3Notary = nil
			},
			expectedErr: constants.ErrL3NotaryRequired,
		},
		{
			name: "nil SignerStore returns ErrSignerStoreRequired",
			modify: func(d *GatewayModeDeps) {
				d.SignerStore = nil
			},
			expectedErr: constants.ErrSignerStoreRequired,
		},
		{
			name: "nil Doctrine returns ErrDoctrineRequired",
			modify: func(d *GatewayModeDeps) {
				d.Doctrine = nil
			},
			expectedErr: constants.ErrDoctrineRequired,
		},
		{
			name: "nil GovernedDocStore returns ErrGovernedDocStoreRequired",
			modify: func(d *GatewayModeDeps) {
				d.GovernedDocStore = nil
			},
			expectedErr: constants.ErrGovernedDocStoreRequired,
		},
		{
			name: "nil ConsensusPolicyStore returns ErrConsensusPolicyStoreRequired",
			modify: func(d *GatewayModeDeps) {
				d.ConsensusPolicyStore = nil
			},
			expectedErr: constants.ErrConsensusPolicyStoreRequired,
		},
		{
			name: "nil FieldReader returns ErrFieldReaderRequired",
			modify: func(d *GatewayModeDeps) {
				d.FieldReader = nil
			},
			expectedErr: constants.ErrFieldReaderRequired,
		},
		{
			name: "nil PlatformEnrollmentDeps returns ErrPlatformEnrollmentDepsRequired",
			modify: func(d *GatewayModeDeps) {
				d.PlatformEnrollmentDeps = nil
			},
			expectedErr: constants.ErrPlatformEnrollmentDepsRequired,
		},
		{
			name: "nil Consensus under Consensus posture returns ErrConsensusServiceRequired",
			modify: func(d *GatewayModeDeps) {
				d.Posture = config.PostureConsensus
				d.Consensus = nil
			},
			expectedErr: constants.ErrConsensusServiceRequired,
		},
		{
			name: "nil Consensus under Notary posture returns ErrConsensusServiceRequired",
			modify: func(d *GatewayModeDeps) {
				d.Posture = config.PostureNotary
				d.Consensus = nil
			},
			expectedErr: constants.ErrConsensusServiceRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validTestGatewayModeDeps(config.PostureDoctrine)
			tc.modify(&input)
			deps, err := NewGatewayModeDeps(input)
			require.Error(t, err)
			require.Nil(t, deps)
			assert.True(t, errors.Is(err, tc.expectedErr), "expected error %v, got %v", tc.expectedErr, err)
		})
	}
}
