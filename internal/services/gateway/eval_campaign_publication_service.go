// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

const evalCampaignPublicationStateSchemaVersion = "1.0.0"

// EvalCampaignPublicationService persists native eval campaign publication
// idempotency in the gateway document store.
type EvalCampaignPublicationService struct {
	docStore *DocumentStoreService
}

func NewEvalCampaignPublicationService(docStore *DocumentStoreService) *EvalCampaignPublicationService {
	return &EvalCampaignPublicationService{docStore: docStore}
}

func (s *EvalCampaignPublicationService) Get(runID string) (*models.EvalCampaignPublicationState, error) {
	if s == nil || s.docStore == nil || runID == "" {
		return nil, fmt.Errorf("eval campaign publication: %w", constants.ErrMissingRequiredField)
	}
	collection := marshaler.CollectionName(constants.CollectionEvalCampaignPublicationState)
	doc, err := s.docStore.DocGet(collection, runID)
	if err != nil {
		return nil, fmt.Errorf("eval campaign publication: get: %w", err)
	}
	if doc == nil {
		return &models.EvalCampaignPublicationState{
			SchemaVersion:        evalCampaignPublicationStateSchemaVersion,
			RunID:                runID,
			PublishedIdempotency: []string{},
		}, nil
	}
	state := &models.EvalCampaignPublicationState{}
	if err := unmarshalDocData(doc, state); err != nil {
		return nil, fmt.Errorf("eval campaign publication: decode: %w", err)
	}
	if state.SchemaVersion == "" {
		state.SchemaVersion = evalCampaignPublicationStateSchemaVersion
	}
	if state.RunID == "" {
		state.RunID = runID
	}
	if state.PublishedIdempotency == nil {
		state.PublishedIdempotency = []string{}
	}
	return state, nil
}

func (s *EvalCampaignPublicationService) Put(state models.EvalCampaignPublicationState) error {
	if s == nil || s.docStore == nil || state.RunID == "" {
		return fmt.Errorf("eval campaign publication: %w", constants.ErrMissingRequiredField)
	}
	if state.SchemaVersion == "" {
		state.SchemaVersion = evalCampaignPublicationStateSchemaVersion
	}
	if state.PublishedIdempotency == nil {
		state.PublishedIdempotency = []string{}
	}
	sort.Strings(state.PublishedIdempotency)
	body, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("eval campaign publication: marshal: %w", err)
	}
	collection := marshaler.CollectionName(constants.CollectionEvalCampaignPublicationState)
	if err := s.docStore.DocSet(collection, state.RunID, body); err != nil {
		return fmt.Errorf("eval campaign publication: put: %w", err)
	}
	return nil
}
