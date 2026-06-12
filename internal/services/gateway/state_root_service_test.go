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
