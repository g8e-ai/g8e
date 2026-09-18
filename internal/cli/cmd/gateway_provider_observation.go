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

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type remoteProviderObservationClient struct {
	client apiClient
	mu     sync.Mutex
	cache  map[string]*remoteProviderObservationBundle
}

type remoteProviderObservationBundle struct {
	window  *evalv1.ProviderBoundaryObservationWindow
	attempt *operatorv1.InferenceProviderAttemptRecord
}

func (c *remoteProviderObservationClient) Load(ctx context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, *operatorv1.InferenceProviderAttemptRecord, error) {
	if c == nil || c.client == nil || providerAttemptID == "" {
		return nil, nil, constants.ErrNotFound
	}

	c.mu.Lock()
	if bundle := c.cache[providerAttemptID]; bundle != nil {
		c.mu.Unlock()
		return bundle.window, bundle.attempt, nil
	}
	c.mu.Unlock()

	body, err := c.client.Get(constants.APIPaths.InferenceProviderObservations + providerAttemptID)
	if err != nil {
		return nil, nil, fmt.Errorf("provider observation: gateway read: %w", err)
	}
	var resp models.ProviderObservationResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	window := &evalv1.ProviderBoundaryObservationWindow{}
	if err := evalv1.UnmarshalCanonical(resp.Window, window); err != nil {
		return nil, nil, fmt.Errorf("provider observation: decode window: %w", err)
	}
	attempt := &operatorv1.InferenceProviderAttemptRecord{}
	if err := protojson.Unmarshal(resp.ProviderAttempt, attempt); err != nil {
		return nil, nil, fmt.Errorf("provider observation: decode attempt: %w", err)
	}

	bundle := &remoteProviderObservationBundle{window: window, attempt: attempt}
	c.mu.Lock()
	if c.cache == nil {
		c.cache = make(map[string]*remoteProviderObservationBundle)
	}
	c.cache[providerAttemptID] = bundle
	c.mu.Unlock()
	return bundle.window, bundle.attempt, nil
}

func newProviderObservationRemote(fileSvc fs.RuntimeFileService, cfg *config.Config) (evaluation.ProviderObservationRemote, error) {
	if !isGatewayHealthy() {
		return nil, nil
	}
	client, err := defaultAPIClientFactory(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("provider observation: create gateway client: %w", err)
	}
	return &remoteProviderObservationClient{client: client}, nil
}

func newCampaignProviderObservationReader(fileSvc fs.RuntimeFileService, cfg *config.Config) (*evaluation.CampaignProviderObservationReader, error) {
	remote, err := newProviderObservationRemote(fileSvc, cfg)
	if err != nil {
		return nil, err
	}
	return evaluation.NewCampaignProviderObservationReaderWithRemote(fileSvc, remote)
}

func preflightProviderObservationDelivery(fileSvc fs.RuntimeFileService, cfg *config.Config) error {
	if !isGatewayHealthy() {
		return constants.ErrEvaluationObservationUnavailable
	}
	client, err := defaultAPIClientFactory(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("provider observation preflight: create gateway client: %w", err)
	}
	body, err := client.Get(constants.APIPaths.InferenceProviderObservations + "_preflight")
	if err != nil {
		return fmt.Errorf("provider observation preflight: %w", err)
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	if resp.Status != "ready" {
		return fmt.Errorf("provider observation preflight: unexpected status %q", resp.Status)
	}
	return nil
}
