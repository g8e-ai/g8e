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

// Package governancetest provides test-only implementations of governance
// store interfaces. This keeps test fixtures out of production code.
package governancetest

import (
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// SimpleConsensusStore implements governance.ConsensusStore using a static map.
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
