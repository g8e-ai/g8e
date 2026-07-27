package governance

import "github.com/g8e-ai/g8e/internal/services/governance/governancetest"

// consensusStoreTestAdapter wraps a governancetest.SimpleConsensusStore and adapts
// it to satisfy L2ConsensusPolicyStore for test code within the governance package.
// This is the test-only replacement for the removed production ConsensusStoreAdapter.
type consensusStoreTestAdapter struct {
	Inner *governancetest.SimpleConsensusStore
}

func (a *consensusStoreTestAdapter) GetConsensusPolicy(id string) (*L2ConsensusPolicy, error) {
	policy, err := a.Inner.GetConsensus(id)
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
