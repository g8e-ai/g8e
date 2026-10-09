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
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStateRootService_ConcurrentDirtyReadersCoalesceFlushes verifies that N concurrent
// dirty readers trigger significantly fewer than N write flush transactions, while
// still computing and returning identical, correct state roots matching the oracle.
func TestStateRootService_ConcurrentDirtyReadersCoalesceFlushes(t *testing.T) {
	db := newTestDB(t)
	s := db.GetStateRootSvc()

	// Insert several dirty rows so state requires a tree root flush.
	const docCount = 10
	for i := 0; i < docCount; i++ {
		doc := json.RawMessage(fmt.Sprintf(`{"index":%d,"data":"coalescing-test"}`, i))
		require.NoError(t, db.GetDocStore().DocSet(t.Context(), "coalesce_cohort", fmt.Sprintf("doc-%d", i), doc))
	}

	startsBefore := s.flushStarts.Load()

	const numReaders = 25
	roots := make([]string, numReaders)
	errs := make([]error, numReaders)

	var wg sync.WaitGroup
	wg.Add(numReaders)

	startGate := make(chan struct{})

	for i := 0; i < numReaders; i++ {
		idx := i
		go func() {
			defer wg.Done()
			<-startGate
			roots[idx], errs[idx] = s.GetCurrentStateRoot(context.Background())
		}()
	}

	// Release all readers simultaneously.
	close(startGate)
	wg.Wait()

	// All readers must have succeeded without error.
	for i := 0; i < numReaders; i++ {
		require.NoError(t, errs[i], "reader %d returned error: %v", i, errs[i])
	}

	// All readers must return the exact same computed state root.
	firstRoot := roots[0]
	require.NotEmpty(t, firstRoot)
	for i := 1; i < numReaders; i++ {
		assert.Equal(t, firstRoot, roots[i], "reader %d root must match first root", i)
	}

	// Root coverage integrity: the coalesced root must match the oracle recalculation.
	oracleBound, _ := oracleStateRoots(t, db)
	assert.Equal(t, oracleBound, firstRoot, "coalesced root must match oracle root")

	// Flush coalescing assertion: the number of actual flush transactions executed
	// must be strictly fewer than the number of concurrent readers (numReaders=25).
	flushesExecuted := s.flushStarts.Load() - startsBefore
	t.Logf("N=%d concurrent dirty readers triggered %d write flushes", numReaders, flushesExecuted)
	assert.Less(t, flushesExecuted, uint64(numReaders), "coalescing must trigger fewer than N write flushes")
	assert.GreaterOrEqual(t, flushesExecuted, uint64(1), "at least one flush must occur for dirty state")
}

// TestStateRootService_FailedFlushDoesNotSatisfyWaitingReaders verifies that if a write
// flush transaction fails, flushCovered does not advance, ensuring subsequent readers
// or retries do not mistakenly treat dirty state as flushed.
func TestStateRootService_FailedFlushDoesNotSatisfyWaitingReaders(t *testing.T) {
	db := newTestDB(t)
	s := db.GetStateRootSvc()

	// Initial clean root.
	initialRoot, err := s.GetCurrentStateRoot(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, initialRoot)

	// Make state dirty by inserting a document.
	require.NoError(t, db.GetDocStore().DocSet(t.Context(), "fail_cohort", "doc1", json.RawMessage(`{"state":"dirty"}`)))

	coveredBefore := s.flushCovered

	// Temporarily rename state_nodes to induce a flush failure.
	_, err = db.db.Exec("ALTER TABLE state_nodes RENAME TO state_nodes_tmp")
	require.NoError(t, err)

	// Attempting to calculate root on dirty state with broken schema must fail.
	_, err = s.GetCurrentStateRoot(t.Context())
	require.Error(t, err, "GetCurrentStateRoot must fail when flush transaction errors")

	// Critical contract: flushCovered MUST NOT advance when a flush fails.
	assert.Equal(t, coveredBefore, s.flushCovered, "failed flush must not advance flushCovered")

	// Restore table schema.
	_, err = db.db.Exec("ALTER TABLE state_nodes_tmp RENAME TO state_nodes")
	require.NoError(t, err)

	// Subsequent call must execute the flush and succeed now that schema is intact.
	recoveredRoot, err := s.GetCurrentStateRoot(t.Context())
	require.NoError(t, err, "GetCurrentStateRoot must succeed after schema restored")
	assert.NotEqual(t, initialRoot, recoveredRoot, "flushed root must reflect newly inserted document")
	assert.Greater(t, s.flushCovered, coveredBefore, "successful flush must advance flushCovered")

	// Verify root matches oracle recalculation.
	requireRootsMatchOracle(t, db)
}
