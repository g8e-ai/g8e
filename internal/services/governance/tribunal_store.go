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

import "github.com/g8e-ai/g8e/internal/models"

// TribunalStore defines the interface for loading TribunalPolicy by ID.
// This is the Tribunal-specific store; the L4 Warden depends on the generic
// L2ConsensusPolicyStore interface instead.
type TribunalStore interface {
	GetTribunal(id string) (*models.TribunalPolicy, error)
}

// L2ConsensusPolicy is the generic consensus policy consumed by the L4 Warden.
// It is not tied to any specific consensus implementation (e.g., Tribunal).
type L2ConsensusPolicy struct {
	MemberKeyIDs    []string
	Quorum          int
	RequireDistinct bool
	Enabled         bool
}

// L2ConsensusPolicyStore defines the generic interface for loading an L2
// consensus policy by ID. The L4 Warden depends on this interface rather than
// TribunalStore, allowing alternative consensus implementations to be plugged
// in without modifying the warden.
type L2ConsensusPolicyStore interface {
	GetConsensusPolicy(id string) (*L2ConsensusPolicy, error)
}

// TribunalStoreAdapter wraps a TribunalStore and adapts it to satisfy
// L2ConsensusPolicyStore. This allows any TribunalStore implementation to
// be used where an L2ConsensusPolicyStore is required.
type TribunalStoreAdapter struct {
	Inner TribunalStore
}

func (a *TribunalStoreAdapter) GetConsensusPolicy(id string) (*L2ConsensusPolicy, error) {
	policy, err := a.Inner.GetTribunal(id)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, nil
	}
	return &L2ConsensusPolicy{
		MemberKeyIDs:    policy.MemberAppIDs,
		Quorum:          policy.Quorum,
		RequireDistinct: policy.RequireDistinct,
		Enabled:         policy.Enabled,
	}, nil
}
