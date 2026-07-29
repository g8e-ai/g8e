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

package consensus

import (
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	govsvc "github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConsensusService_Deliberate_ConcurrentIdempotency verifies that
// concurrent calls to Deliberate on the same envelope produce identical
// results — same vote count, same decisions, same signatures. This guards
// against race conditions in the deliberation path.
func TestConsensusService_Deliberate_ConcurrentIdempotency(t *testing.T) {
	t.Parallel()
	doctrine := govsvc.NewL1Doctrine()
	members := makeMembers(t, 3)
	svc := NewConsensusService("test-consensus", members, doctrine, slog.New(slog.NewTextHandler(io.Discard, nil)), newTestResponder())

	env := makeEnvelope(t, string(constants.ActionTypeFetchLogs), []byte("fetch logs"))

	const numConcurrent = 10
	var wg sync.WaitGroup
	results := make([]*DeliberateResult, numConcurrent)
	errs := make([]error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine gets its own copy of the envelope to avoid
			// concurrent mutation of the Governance field.
			envCopy := *env
			envCopy.Governance = nil
			result, err := svc.Deliberate(&envCopy)
			results[idx] = result
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i := 0; i < numConcurrent; i++ {
		require.NoError(t, errs[i], "goroutine %d failed", i)
		require.NotNil(t, results[i], "goroutine %d returned nil result", i)
		assert.Len(t, results[i].Envelope.Governance.L2.Votes, 3, "goroutine %d has wrong vote count", i)
		for j, vote := range results[i].Envelope.Governance.L2.Votes {
			assert.True(t, vote.Decision, "goroutine %d vote %d should be true", i, j)
			assert.NotEmpty(t, vote.ConsensusSignature, "goroutine %d vote %d has empty signature", i, j)
		}
	}

	// All signatures must be identical (deterministic signing from same key + hash).
	refSigs := make([]string, len(results[0].Envelope.Governance.L2.Votes))
	for j, vote := range results[0].Envelope.Governance.L2.Votes {
		refSigs[j] = vote.ConsensusSignature
	}
	for i := 1; i < numConcurrent; i++ {
		for j, vote := range results[i].Envelope.Governance.L2.Votes {
			assert.Equal(t, refSigs[j], vote.ConsensusSignature,
				"goroutine %d vote %d signature differs from goroutine 0", i, j)
		}
	}
}
