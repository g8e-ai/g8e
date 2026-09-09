// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObserveAPIPaths verifies the observe API path constants match their
// expected wire values. These routes form the passkey-scoped, read-only
// observability surface added in v2.1.8.
func TestObserveAPIPaths(t *testing.T) {
	cases := []struct {
		name     string
		got      string
		expected string
	}{
		{"ObservePrefix", APIPaths.ObservePrefix, "/api/v1/observe/"},
		{"ObserveBootstrap", APIPaths.ObserveBootstrap, "/api/v1/observe/bootstrap"},
		{"ObserveRuns", APIPaths.ObserveRuns, "/api/v1/observe/runs"},
		{"ObserveRunsByID", APIPaths.ObserveRunsByID, "/api/v1/observe/runs/"},
		{"ObserveEvals", APIPaths.ObserveEvals, "/api/v1/observe/evals"},
		{"ObserveEvalsByID", APIPaths.ObserveEvalsByID, "/api/v1/observe/evals/"},
		{"ObserveDownloads", APIPaths.ObserveDownloads, "/api/v1/observe/downloads"},
		{"ObserveDownloadsByID", APIPaths.ObserveDownloadsByID, "/api/v1/observe/downloads/"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, tc.got, "%s mismatch", tc.name)
	}
}

// TestObserveAPIPathsJSONSync verifies that the Go observe path constants
// match the canonical JSON source of truth in protocol/constants/api_paths.json.
func TestObserveAPIPathsJSONSync(t *testing.T) {
	data, err := os.ReadFile("../../protocol/constants/api_paths.json")
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	cases := []struct {
		jsonKey   string
		goValue   string
	}{
		{"observe_prefix", APIPaths.ObservePrefix},
		{"observe_bootstrap", APIPaths.ObserveBootstrap},
		{"observe_runs", APIPaths.ObserveRuns},
		{"observe_runs_by_id", APIPaths.ObserveRunsByID},
		{"observe_evals", APIPaths.ObserveEvals},
		{"observe_evals_by_id", APIPaths.ObserveEvalsByID},
		{"observe_downloads", APIPaths.ObserveDownloads},
		{"observe_downloads_by_id", APIPaths.ObserveDownloadsByID},
	}
	for _, tc := range cases {
		rawVal, ok := raw[tc.jsonKey]
		require.True(t, ok, "api_paths.json missing key %s", tc.jsonKey)
		jsonVal, ok := rawVal.(string)
		require.True(t, ok, "api_paths.json key %s is not a string", tc.jsonKey)
		assert.Equal(t, tc.goValue, jsonVal, "Go constant for %s does not match JSON", tc.jsonKey)
	}
}

// TestObserveAPIPathsAreUnderObservePrefix verifies every observe route
// (except the prefix itself) lives under the observe prefix. This guards
// against accidental route classification that would expose these routes
// under a different prefix.
func TestObserveAPIPathsAreUnderObservePrefix(t *testing.T) {
	prefix := APIPaths.ObservePrefix
	routes := []string{
		APIPaths.ObserveBootstrap,
		APIPaths.ObserveRuns,
		APIPaths.ObserveRunsByID,
		APIPaths.ObserveEvals,
		APIPaths.ObserveEvalsByID,
		APIPaths.ObserveDownloads,
		APIPaths.ObserveDownloadsByID,
	}
	for _, route := range routes {
		assert.True(t, len(route) > len(prefix) && route[:len(prefix)] == prefix,
			"observe route %s must live under %s", route, prefix)
	}
}
