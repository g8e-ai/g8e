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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteStateRootProvider_GetCurrentStateRoot_Success(t *testing.T) {
	t.Parallel()

	wantRoot := "abc123def456"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, constants.APIPaths.State, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.StateResponse{
			StateMerkleRoot: wantRoot,
		}))
	}))
	t.Cleanup(srv.Close)

	provider := NewRemoteStateRootProvider(srv.Client(), srv.URL, testutil.NewTestLogger())
	root, err := provider.GetCurrentStateRoot()
	require.NoError(t, err)
	assert.Equal(t, wantRoot, root)
}

func TestRemoteStateRootProvider_GetCurrentStateRoot_Non200Status(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	provider := NewRemoteStateRootProvider(srv.Client(), srv.URL, testutil.NewTestLogger())
	root, err := provider.GetCurrentStateRoot()

	require.Error(t, err)
	assert.Empty(t, root)
	assert.True(t, errors.Is(err, constants.ErrStateRootFetch),
		"expected ErrStateRootFetch, got: %v", err)
}

func TestRemoteStateRootProvider_GetCurrentStateRoot_EmptyRoot(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(models.StateResponse{
			StateMerkleRoot: "",
		}))
	}))
	t.Cleanup(srv.Close)

	provider := NewRemoteStateRootProvider(srv.Client(), srv.URL, testutil.NewTestLogger())
	root, err := provider.GetCurrentStateRoot()

	require.Error(t, err)
	assert.Empty(t, root)
	assert.True(t, errors.Is(err, constants.ErrStateRootFetch),
		"empty state root from gateway must fail closed with ErrStateRootFetch, got: %v", err)
}

func TestRemoteStateRootProvider_GetCurrentStateRoot_InvalidJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	t.Cleanup(srv.Close)

	provider := NewRemoteStateRootProvider(srv.Client(), srv.URL, testutil.NewTestLogger())
	root, err := provider.GetCurrentStateRoot()

	require.Error(t, err)
	assert.Empty(t, root)
	assert.True(t, errors.Is(err, constants.ErrStateRootFetch),
		"invalid JSON must fail closed with ErrStateRootFetch, got: %v", err)
}

func TestRemoteStateRootProvider_GetCurrentStateRoot_UnreachableServer(t *testing.T) {
	t.Parallel()

	// Create a server and immediately close it to simulate an unreachable gateway.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	provider := NewRemoteStateRootProvider(srv.Client(), srv.URL, testutil.NewTestLogger())
	root, err := provider.GetCurrentStateRoot()

	require.Error(t, err)
	assert.Empty(t, root)
	assert.True(t, errors.Is(err, constants.ErrStateRootFetch),
		"unreachable gateway must fail closed with ErrStateRootFetch, got: %v", err)
}
