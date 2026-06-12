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
func NewAppPolicyStoreService(db *sqliteutil.DB, logger *slog.Logger) *AppPolicyStoreService {
	return &AppPolicyStoreService{
		db:     db,
		logger: logger,
		docSvc: NewDocumentStoreService(db, logger),
	}
}

// GetAppPolicy retrieves an AppPolicy by app_id from the database.
// Implements governance.AppPolicyStore.
func (s *AppPolicyStoreService) GetAppPolicy(appID string) (*models.AppPolicy, error) {
	doc, err := s.docSvc.DocGet(marshaler.CollectionName(constants.CollectionAppPolicies), appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app policy %s: %w", appID, err)
	}
	if doc == nil {
		return nil, nil
	}

	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal app policy data: %w", err)
	}

	var policy models.AppPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("failed to unmarshal app policy: %w", err)
	}

	return &policy, nil
}
