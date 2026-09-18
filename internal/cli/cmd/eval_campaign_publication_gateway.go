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

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

type gatewayCampaignPublicationStateStore struct {
	client apiClient
}

func newGatewayCampaignPublicationStateStore(client apiClient) evaluation.CampaignPublicationStateStore {
	return &gatewayCampaignPublicationStateStore{client: client}
}

func (s *gatewayCampaignPublicationStateStore) Load(_ context.Context, runID string) (*evaluation.CampaignPublicationState, error) {
	if s == nil || s.client == nil || runID == "" {
		return nil, fmt.Errorf("evaluation: load publication state: %w", constants.ErrMissingRequiredField)
	}
	path := constants.APIPaths.EvalCampaignPublicationStateByRun + runID + "/publication-state"
	body, err := s.client.Get(path)
	if err != nil {
		return nil, fmt.Errorf("evaluation: load publication state: %w", err)
	}
	var remote models.EvalCampaignPublicationState
	if err := json.Unmarshal(body, &remote); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return &evaluation.CampaignPublicationState{
		SchemaVersion:         remote.SchemaVersion,
		RunID:                 remote.RunID,
		PublishedIdempotency:  remote.PublishedIdempotency,
		LastPublishedSequence: remote.LastPublishedSequence,
	}, nil
}

func (s *gatewayCampaignPublicationStateStore) Save(_ context.Context, state *evaluation.CampaignPublicationState) error {
	if s == nil || s.client == nil || state == nil || state.RunID == "" {
		return fmt.Errorf("evaluation: save publication state: %w", constants.ErrMissingRequiredField)
	}
	path := constants.APIPaths.EvalCampaignPublicationStateByRun + state.RunID + "/publication-state"
	payload, err := json.Marshal(models.EvalCampaignPublicationState{
		SchemaVersion:         state.SchemaVersion,
		RunID:                 state.RunID,
		PublishedIdempotency:  state.PublishedIdempotency,
		LastPublishedSequence: state.LastPublishedSequence,
	})
	if err != nil {
		return fmt.Errorf("evaluation: save publication state: %w", err)
	}
	if _, err := s.client.Put(path, payload); err != nil {
		return fmt.Errorf("evaluation: save publication state: %w", err)
	}
	return nil
}

func newGatewayCampaignPublicationStateStoreFromConfig(fileSvc fs.RuntimeFileService, cfg *config.Config) (evaluation.CampaignPublicationStateStore, error) {
	client, err := defaultAPIClientFactory(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: create gateway client: %w", err)
	}
	return newGatewayCampaignPublicationStateStore(client), nil
}
