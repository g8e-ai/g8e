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
	"log/slog"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// AppPolicyStoreService provides app policy retrieval.
type AppPolicyStoreService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
	docSvc *DocumentStoreService
}

// NewAppPolicyStoreService creates a new app policy store service.
// docSvc is the shared DocumentStoreService instance from CanonicalDBService.
func NewAppPolicyStoreService(db *sqliteutil.DB, logger *slog.Logger, docSvc *DocumentStoreService) *AppPolicyStoreService {
	return &AppPolicyStoreService{
		db:     db,
		logger: logger,
		docSvc: docSvc,
	}
}

// GetAppPolicy retrieves an AppPolicy by app_id from the database.
// Implements governance.AppPolicyStore.
func (s *AppPolicyStoreService) GetAppPolicy(appID string) (*models.AppPolicy, error) {
	doc, err := s.docSvc.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", constants.ErrAppPolicyStoreGetFailed, appID)
	}
	if doc == nil {
		return nil, nil
	}

	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAppPolicyStoreMarshalFailed, err)
	}

	var policy models.AppPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrAppPolicyStoreUnmarshalFailed, err)
	}

	return &policy, nil
}
