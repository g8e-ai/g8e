// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package governancetest provides test-only implementations of governance
// store interfaces. This keeps test fixtures out of production code.
package governancetest

import (
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// SimpleConsensusStore provides in-memory ConsensusPolicy lookup for tests.
// It is adapted to governance.L2ConsensusPolicyStore via consensusStoreTestAdapter
// in governance test code (the adapter lives in the governance package to avoid
// an import cycle).
type SimpleConsensusStore struct {
	Consensus map[string]*models.ConsensusPolicy
}

func (s *SimpleConsensusStore) GetConsensus(id string) (*models.ConsensusPolicy, error) {
	if s.Consensus == nil {
		return nil, nil
	}
	consensus, ok := s.Consensus[id]
	if !ok {
		return nil, nil
	}
	return consensus, nil
}

// SimpleAppPolicyStore implements governance.AppPolicyStore using a static map.
type SimpleAppPolicyStore struct {
	Policies map[string]*models.AppPolicy
}

func (s *SimpleAppPolicyStore) GetAppPolicy(appID string) (*models.AppPolicy, error) {
	if s.Policies == nil {
		return nil, nil
	}
	policy, ok := s.Policies[appID]
	if !ok {
		return nil, nil
	}
	return policy, nil
}

// SimpleStateRootProvider returns a fixed root set at construction time.
// Root must be non-empty; a missing root is a misconfiguration that returns an
// error so callers fail closed rather than silently accepting any state root.
type SimpleStateRootProvider struct {
	Root string
}

func (s *SimpleStateRootProvider) GetCurrentStateRoot() (string, error) {
	if s.Root == "" {
		return "", constants.ErrTxProviderMisconfigured
	}
	return s.Root, nil
}
