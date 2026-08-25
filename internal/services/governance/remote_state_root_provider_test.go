// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
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
