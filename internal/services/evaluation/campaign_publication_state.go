// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

const campaignPublicationStateSchemaVersion = CampaignSchemaVersion

// CampaignPublicationState tracks exported public projection idempotency keys
// for one campaign run. The gateway document store is authoritative.
type CampaignPublicationState struct {
	SchemaVersion         string
	RunID                 string
	PublishedIdempotency  []string
	LastPublishedSequence int64
}

// CampaignPublicationStateStore persists publication idempotency in the
// gateway-owned eval state plane.
type CampaignPublicationStateStore interface {
	Load(ctx context.Context, runID string) (*CampaignPublicationState, error)
	Save(ctx context.Context, state *CampaignPublicationState) error
}

type memoryCampaignPublicationStateStore struct {
	byRun map[string]*CampaignPublicationState
}

func NewMemoryCampaignPublicationStateStore() *memoryCampaignPublicationStateStore {
	return &memoryCampaignPublicationStateStore{byRun: map[string]*CampaignPublicationState{}}
}

func (s *memoryCampaignPublicationStateStore) Load(_ context.Context, runID string) (*CampaignPublicationState, error) {
	if s == nil || runID == "" {
		return nil, fmt.Errorf("evaluation: load publication state: %w", constants.ErrMissingRequiredField)
	}
	state, ok := s.byRun[runID]
	if !ok {
		return &CampaignPublicationState{
			SchemaVersion:        campaignPublicationStateSchemaVersion,
			RunID:                runID,
			PublishedIdempotency: []string{},
		}, nil
	}
	return cloneCampaignPublicationState(state), nil
}

func (s *memoryCampaignPublicationStateStore) Save(_ context.Context, state *CampaignPublicationState) error {
	if s == nil || state == nil || state.RunID == "" {
		return fmt.Errorf("evaluation: save publication state: %w", constants.ErrMissingRequiredField)
	}
	s.byRun[state.RunID] = cloneCampaignPublicationState(state)
	return nil
}

func cloneCampaignPublicationState(state *CampaignPublicationState) *CampaignPublicationState {
	if state == nil {
		return nil
	}
	keys := append([]string(nil), state.PublishedIdempotency...)
	return &CampaignPublicationState{
		SchemaVersion:         state.SchemaVersion,
		RunID:                 state.RunID,
		PublishedIdempotency:  keys,
		LastPublishedSequence: state.LastPublishedSequence,
	}
}
