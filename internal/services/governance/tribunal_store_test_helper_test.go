package governance

import "github.com/g8e-ai/g8e/internal/services/governance/governancetest"

// tribunalStoreTestAdapter wraps a governancetest.SimpleTribunalStore and adapts
// it to satisfy L2ConsensusPolicyStore for test code within the governance package.
// This is the test-only replacement for the removed production TribunalStoreAdapter.
type tribunalStoreTestAdapter struct {
	Inner *governancetest.SimpleTribunalStore
}

func (a *tribunalStoreTestAdapter) GetConsensusPolicy(id string) (*L2ConsensusPolicy, error) {
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
