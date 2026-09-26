// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"encoding/json"
	"fmt"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

type gatewayCampaignPublicationStateStore struct {
	client authcmd.APIClient
}

func newGatewayCampaignPublicationStateStore(client authcmd.APIClient) evaluation.CampaignPublicationStateStore {
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
	proofArtifacts := make(map[string]evaluation.CampaignPublishedProofArtifacts, len(remote.PublishedProofArtifacts))
	for assignmentID, artifacts := range remote.PublishedProofArtifacts {
		proofArtifacts[assignmentID] = evaluation.CampaignPublishedProofArtifacts{
			DatabaseSHA256: artifacts.DatabaseSHA256,
			VaultKeySHA256: artifacts.VaultKeySHA256,
		}
	}
	return &evaluation.CampaignPublicationState{
		SchemaVersion:           remote.SchemaVersion,
		RunID:                   remote.RunID,
		PublishedIdempotency:    remote.PublishedIdempotency,
		PublishedProofArtifacts: proofArtifacts,
		LastPublishedSequence:   remote.LastPublishedSequence,
	}, nil
}

func (s *gatewayCampaignPublicationStateStore) Delete(_ context.Context, runID string) error {
	if s == nil || s.client == nil || runID == "" {
		return fmt.Errorf("evaluation: delete publication state: %w", constants.ErrMissingRequiredField)
	}
	path := constants.APIPaths.EvalCampaignPublicationStateByRun + runID + "/publication-state"
	if _, err := s.client.Delete(path); err != nil {
		return fmt.Errorf("evaluation: delete publication state: %w", err)
	}
	return nil
}

func (s *gatewayCampaignPublicationStateStore) Save(_ context.Context, state *evaluation.CampaignPublicationState) error {
	if s == nil || s.client == nil || state == nil || state.RunID == "" {
		return fmt.Errorf("evaluation: save publication state: %w", constants.ErrMissingRequiredField)
	}
	path := constants.APIPaths.EvalCampaignPublicationStateByRun + state.RunID + "/publication-state"
	proofArtifacts := make(map[string]models.EvalCampaignPublishedProofArtifacts, len(state.PublishedProofArtifacts))
	for assignmentID, artifacts := range state.PublishedProofArtifacts {
		proofArtifacts[assignmentID] = models.EvalCampaignPublishedProofArtifacts{
			DatabaseSHA256: artifacts.DatabaseSHA256,
			VaultKeySHA256: artifacts.VaultKeySHA256,
		}
	}
	if _, err := s.client.Put(path, models.EvalCampaignPublicationState{
		SchemaVersion:           state.SchemaVersion,
		RunID:                   state.RunID,
		PublishedIdempotency:    state.PublishedIdempotency,
		PublishedProofArtifacts: proofArtifacts,
		LastPublishedSequence:   state.LastPublishedSequence,
	}); err != nil {
		return fmt.Errorf("evaluation: save publication state: %w", err)
	}
	return nil
}

func newGatewayCampaignPublicationStateStoreFromConfig(fileSvc fs.RuntimeFileService, cfg *config.Config) (evaluation.CampaignPublicationStateStore, error) {
	client, err := authcmd.DefaultAPIClientFactory(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: create gateway client: %w", err)
	}
	return newGatewayCampaignPublicationStateStore(client), nil
}
