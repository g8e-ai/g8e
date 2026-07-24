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
	"testing"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTribunalStoreAdapter_GetConsensusPolicy_Found(t *testing.T) {
	t.Parallel()
	inner := &mockTribunalStore{
		policies: map[string]*models.TribunalPolicy{
			"trib-1": {
				ID:              "trib-1",
				MemberAppIDs:    []string{"member-a", "member-b"},
				Quorum:          2,
				RequireDistinct: true,
				Enabled:         true,
			},
		},
	}
	adapter := &TribunalStoreAdapter{Inner: inner}

	policy, err := adapter.GetConsensusPolicy("trib-1")
	require.NoError(t, err)
	require.NotNil(t, policy)
	assert.Equal(t, []string{"member-a", "member-b"}, policy.MemberKeyIDs)
	assert.Equal(t, 2, policy.Quorum)
	assert.True(t, policy.RequireDistinct)
	assert.True(t, policy.Enabled)
}

func TestTribunalStoreAdapter_GetConsensusPolicy_NotFound(t *testing.T) {
	t.Parallel()
	inner := &mockTribunalStore{
		policies: map[string]*models.TribunalPolicy{},
	}
	adapter := &TribunalStoreAdapter{Inner: inner}

	policy, err := adapter.GetConsensusPolicy("nonexistent")
	require.NoError(t, err)
	assert.Nil(t, policy)
}

func TestTribunalStoreAdapter_GetConsensusPolicy_Error(t *testing.T) {
	t.Parallel()
	inner := &mockTribunalStore{err: assert.AnError}
	adapter := &TribunalStoreAdapter{Inner: inner}

	policy, err := adapter.GetConsensusPolicy("trib-1")
	require.Error(t, err)
	assert.Nil(t, policy)
}

type mockTribunalStore struct {
	policies map[string]*models.TribunalPolicy
	err      error
}

func (m *mockTribunalStore) GetTribunal(id string) (*models.TribunalPolicy, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.policies == nil {
		return nil, nil
	}
	p, ok := m.policies[id]
	if !ok {
		return nil, nil
	}
	return p, nil
}
