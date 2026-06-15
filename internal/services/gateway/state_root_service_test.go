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

package gateway

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStateRootService(t *testing.T) *StateRootService {
	t.Helper()
	db := newTestDB(t)
	return NewStateRootService(db.GetDB(), testutil.NewTestLogger())
}

func TestStateRootService_GetCurrentStateRoot(t *testing.T) {
	t.Parallel()
	svc := newStateRootService(t)

	// Get initial state root
	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root1)

	// Get state root again - should return cached value
	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2)
}

func TestStateRootService_InvalidateCache(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewStateRootService(db.GetDB(), testutil.NewTestLogger())

	// Get initial state root
	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)

	// Invalidate cache
	err = svc.InvalidateCache()
	require.NoError(t, err)

	// Add a document to change state
	docData := mustDocJSON(t, map[string]interface{}{"key": "value"})
	err = db.DocStore.DocSet("test", "doc1", docData)
	require.NoError(t, err)

	// Get state root after invalidation - should recalculate
	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root1, root2, "state root should change after cache invalidation and document change")
}

func TestStateRootService_StateChangeDetection(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewStateRootService(db.GetDB(), testutil.NewTestLogger())

	// Get initial state root
	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)

	// Add a document
	docData := mustDocJSON(t, map[string]interface{}{"key": "value"})
	err = db.DocStore.DocSet("test", "doc1", docData)
	require.NoError(t, err)

	// Get state root after change - should be different
	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root1, root2, "state root should change after document insertion")

	// Get state root again without changes - should be cached
	root3, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root2, root3, "state root should be cached when state hasn't changed")
}

func TestStateRootService_CachingBehavior(t *testing.T) {
	t.Parallel()
	svc := newStateRootService(t)

	// First call - calculates and caches
	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)

	// Second call - should use cache
	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2)

	// Invalidate cache
	err = svc.InvalidateCache()
	require.NoError(t, err)

	// Third call - should recalculate
	root3, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root3, "state root should be same after recalculation without state changes")
}

func TestStateRootService_CalculateStateRootUncached(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewStateRootService(db.GetDB(), testutil.NewTestLogger())

	// Delete the state_version table to force uncached path
	_, err := db.GetDB().Exec("DROP TABLE IF EXISTS state_version")
	require.NoError(t, err)

	// This should fall back to calculateStateRootUncached
	root, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root)

	// Verify the root was persisted to state_root table
	var persistedRoot string
	err = db.GetDB().QueryRow("SELECT root FROM state_root WHERE id = 1").Scan(&persistedRoot)
	require.NoError(t, err)
	assert.Equal(t, root, persistedRoot)
}

// TestStateRootService_NoCacheLeakOnDocumentWrite verifies that inserting a document
// and then calling GetCurrentStateRoot does NOT change the returned root between the
// insert and the root call (cache invalidation must not leak into the authoritative root).
// This is a regression test for the bug where cache-key churn in kv_store caused
// state root mismatches.
func TestStateRootService_NoCacheLeakOnDocumentWrite(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	svc := NewStateRootService(db.GetDB(), testutil.NewTestLogger())

	// Get initial state root
	root1, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEmpty(t, root1)

	// Add a cache entry (simulating cache invalidation)
	err = db.KVStore.KVSet("g8e:cache:doc:test:doc1", "cached_value", 3600)
	require.NoError(t, err)

	// Get state root - should NOT change because cache keys are excluded
	root2, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root1, root2, "state root should not change when cache keys are added")

	// Add another cache entry with different prefix
	err = db.KVStore.KVSet("g8e:cache:query:SELECT * FROM test", "query_result", 3600)
	require.NoError(t, err)

	// Get state root - should still not change
	root3, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, root2, root3, "state root should not change when more cache keys are added")

	// Add a non-cache KV entry (authoritative state)
	err = db.KVStore.KVSet("authoritative:key", "value", 0)
	require.NoError(t, err)

	// Get state root - should now change
	root4, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root3, root4, "state root should change when authoritative KV entries are added")

	// Add a document (which triggers cache invalidation internally)
	docData := mustDocJSON(t, map[string]interface{}{"key": "value"})
	err = db.DocStore.DocSet("test", "doc1", docData)
	require.NoError(t, err)

	// Get state root - should change due to document, not due to cache churn
	root5, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root4, root5, "state root should change after document insertion")

	// Add another document to verify consistent behavior
	docData2 := mustDocJSON(t, map[string]interface{}{"key2": "value2"})
	err = db.DocStore.DocSet("test", "doc2", docData2)
	require.NoError(t, err)

	root6, err := svc.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.NotEqual(t, root5, root6, "state root should change after second document insertion")
}
