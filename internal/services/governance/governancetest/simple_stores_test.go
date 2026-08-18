// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governancetest

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleAppPolicyStore_NilMap(t *testing.T) {
	t.Parallel()
	s := &SimpleAppPolicyStore{}
	policy, err := s.GetAppPolicy("app1")
	require.NoError(t, err)
	assert.Nil(t, policy)
}

func TestSimpleAppPolicyStore_NotFound(t *testing.T) {
	t.Parallel()
	s := &SimpleAppPolicyStore{Policies: map[string]*models.AppPolicy{}}
	policy, err := s.GetAppPolicy("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, policy)
}

func TestSimpleAppPolicyStore_Found(t *testing.T) {
	t.Parallel()
	expected := &models.AppPolicy{AppID: "app1"}
	s := &SimpleAppPolicyStore{Policies: map[string]*models.AppPolicy{"app1": expected}}
	policy, err := s.GetAppPolicy("app1")
	require.NoError(t, err)
	assert.Equal(t, expected, policy)
}

func TestSimpleStateRootProvider_EmptyRoot(t *testing.T) {
	t.Parallel()
	s := &SimpleStateRootProvider{Root: ""}
	_, err := s.GetCurrentStateRoot()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrTxProviderMisconfigured)
}

func TestSimpleStateRootProvider_ValidRoot(t *testing.T) {
	t.Parallel()
	s := &SimpleStateRootProvider{Root: "abc123"}
	root, err := s.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, "abc123", root)
}

func TestSimpleConsensusStore_NilMap(t *testing.T) {
	t.Parallel()
	s := &SimpleConsensusStore{}
	consensus, err := s.GetConsensus("consensus1")
	require.NoError(t, err)
	assert.Nil(t, consensus)
}

func TestSimpleConsensusStore_NotFound(t *testing.T) {
	t.Parallel()
	s := &SimpleConsensusStore{Consensus: map[string]*models.ConsensusPolicy{}}
	consensus, err := s.GetConsensus("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, consensus)
}

func TestSimpleConsensusStore_Found(t *testing.T) {
	t.Parallel()
	expected := &models.ConsensusPolicy{
		ID:              "consensus1",
		MemberAppIDs:    []string{"member-a", "member-b"},
		Quorum:          2,
		RequireDistinct: true,
		Enabled:         true,
	}
	s := &SimpleConsensusStore{Consensus: map[string]*models.ConsensusPolicy{"consensus1": expected}}
	consensus, err := s.GetConsensus("consensus1")
	require.NoError(t, err)
	assert.Equal(t, expected, consensus)
}
