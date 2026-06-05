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

//go:build integration

package tests

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/require"
)

// TestNativeRealOperator_Smoke is a live-operator smoke test that validates
// the bootstrap flow against a running ./g8e platform.
//
// This test is intentionally narrow: it does NOT exercise the full native
// protocol with envelope submission and WebSocket Pub/Sub - those flows
// are covered hermetically by TestBYOClient_EndToEnd in byo_client_test.go.
//
// This test requires the platform to be running via `./g8e platform start`
// and authenticated via `./g8e auth login`.
func TestNativeRealOperator_Smoke(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	client, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)

	// Test basic connectivity to Operator via HTTPS
	healthURL := fmt.Sprintf("https://localhost:%d/api/v1/health", constants.Ports.OperatorHttps)

	resp, err := client.Get(healthURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode, "health check failed")
}
