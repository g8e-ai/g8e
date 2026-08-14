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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// RemoteStateRootProvider implements StateRootProvider by fetching the
// gateway's state Merkle root from the gateway's /api/v1/state endpoint over
// mTLS. The gateway owns the state Merkle root; operators are leaves in the
// gateway's Merkle tree. The operator re-validates the state root locally as
// part of L4, but against the gateway's root — the operator does not have an
// independent state root.
//
// For the first cut, each call to GetCurrentStateRoot fetches the gateway's
// state root with no caching. This eliminates the staleness window entirely at
// the cost of one HTTP round-trip per command. A background refresh goroutine
// with a short TTL is a production optimization.
type RemoteStateRootProvider struct {
	client   *http.Client
	stateURL string
	logger   *slog.Logger
}

// NewRemoteStateRootProvider creates a RemoteStateRootProvider that fetches
// the gateway's state root from the given base URL (e.g. "https://localhost:8443")
// using the provided mTLS HTTP client.
func NewRemoteStateRootProvider(client *http.Client, baseURL string, logger *slog.Logger) *RemoteStateRootProvider {
	return &RemoteStateRootProvider{
		client:   client,
		stateURL: baseURL + constants.APIPaths.State,
		logger:   logger,
	}
}

// GetCurrentStateRoot fetches the gateway's current state Merkle root.
// Fail-closed: any HTTP, network, or parse error is returned; the provider
// never returns a stale or empty root.
func (p *RemoteStateRootProvider) GetCurrentStateRoot() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.stateURL, nil)
	if err != nil {
		return "", fmt.Errorf("remote_state_root: %w: %w", constants.ErrStateRootFetch, err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("remote_state_root: %w: %w", constants.ErrStateRootFetch, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("remote_state_root: %w: gateway returned status %d: %s",
			constants.ErrStateRootFetch, resp.StatusCode, string(body))
	}

	var stateResp models.StateResponse
	if err := json.NewDecoder(resp.Body).Decode(&stateResp); err != nil {
		return "", fmt.Errorf("remote_state_root: %w: failed to decode response: %w",
			constants.ErrStateRootFetch, err)
	}

	if stateResp.StateMerkleRoot == "" {
		return "", fmt.Errorf("remote_state_root: %w: gateway returned empty state merkle root",
			constants.ErrStateRootFetch)
	}

	return stateResp.StateMerkleRoot, nil
}
