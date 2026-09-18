// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type remoteModelProvenanceClient struct {
	client apiClient
	mu     sync.Mutex
	cache  map[string]*evalv1.ModelProvenanceAttestationWindow
}

func (c *remoteModelProvenanceClient) Load(ctx context.Context, providerAttemptID string) (*evalv1.ModelProvenanceAttestationWindow, error) {
	if c == nil || c.client == nil || providerAttemptID == "" {
		return nil, constants.ErrNotFound
	}

	c.mu.Lock()
	if window := c.cache[providerAttemptID]; window != nil {
		c.mu.Unlock()
		return window, nil
	}
	c.mu.Unlock()

	body, err := c.client.Get(constants.APIPaths.InferenceModelProvenanceAttestations + providerAttemptID)
	if err != nil {
		return nil, fmt.Errorf("model provenance: gateway read: %w", err)
	}
	var resp models.ModelProvenanceResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	window := &evalv1.ModelProvenanceAttestationWindow{}
	if err := evalv1.UnmarshalCanonical(resp.Window, window); err != nil {
		return nil, fmt.Errorf("model provenance: decode window: %w", err)
	}

	c.mu.Lock()
	if c.cache == nil {
		c.cache = make(map[string]*evalv1.ModelProvenanceAttestationWindow)
	}
	c.cache[providerAttemptID] = window
	c.mu.Unlock()
	return window, nil
}

func newModelProvenanceRemote(fileSvc fs.RuntimeFileService, cfg *config.Config) (evaluation.ModelProvenanceRemote, error) {
	if !isGatewayHealthy() {
		return nil, nil
	}
	client, err := defaultAPIClientFactory(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("model provenance: create gateway client: %w", err)
	}
	return &remoteModelProvenanceClient{client: client}, nil
}

func newCampaignModelProvenanceReader(fileSvc fs.RuntimeFileService, cfg *config.Config) (*evaluation.CampaignModelProvenanceReader, error) {
	remote, err := newModelProvenanceRemote(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	return evaluation.NewCampaignModelProvenanceReaderWithRemote(fileSvc, remote)
}
