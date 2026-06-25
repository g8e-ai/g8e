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
	"time"
	"unicode"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// TribunalStoreService provides TribunalPolicy CRUD operations.
type TribunalStoreService struct {
	db        *sqliteutil.DB
	logger    *slog.Logger
	docSvc    *DocumentStoreService
	signerSvc *SignerStoreService
}

// NewTribunalStoreService creates a new tribunal store service.
func NewTribunalStoreService(db *sqliteutil.DB, logger *slog.Logger, signerSvc *SignerStoreService) *TribunalStoreService {
	return &TribunalStoreService{
		db:        db,
		logger:    logger,
		docSvc:    NewDocumentStoreService(db, logger),
		signerSvc: signerSvc,
	}
}

// GetTribunal retrieves a TribunalPolicy by ID from the database.
func (s *TribunalStoreService) GetTribunal(id string) (*models.TribunalPolicy, error) {
	doc, err := s.docSvc.DocGet(marshaler.CollectionName(constants.CollectionTribunals), id)
	if err != nil {
		return nil, fmt.Errorf("tribunal store: get: %w", err)
	}
	if doc == nil {
		return nil, nil
	}

	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}

	var policy models.TribunalPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalData, err)
	}
	policy.ID = doc.ID

	return &policy, nil
}

// AddTribunal adds or updates a TribunalPolicy in the database.
// Validation at write time, fail-closed:
// - Tribunal ID is non-empty and contains only alphanumeric, hyphens, underscores
// - Quorum >= 1
// - Quorum <= len(MemberAppIDs)
// - Every MemberAppID resolves to an enabled TrustedSigner
// - No duplicate member IDs
// - New tribunals must be created with Enabled=true (updates may disable)
func (s *TribunalStoreService) AddTribunal(policy models.TribunalPolicy) error {
	if policy.ID == "" {
		return fmt.Errorf("%w: tribunal ID", constants.ErrMissingRequiredField)
	}
	if !isValidTribunalID(policy.ID) {
		return fmt.Errorf("%w: %s", constants.ErrTribunalInvalidID, policy.ID)
	}
	if len(policy.MemberAppIDs) == 0 {
		return fmt.Errorf("%w: tribunal member_app_ids", constants.ErrMissingRequiredField)
	}
	if policy.Quorum < 1 {
		return fmt.Errorf("%w: quorum must be >= 1", constants.ErrConstraintViolation)
	}
	if policy.Quorum > len(policy.MemberAppIDs) {
		return fmt.Errorf("%w: quorum cannot exceed member count", constants.ErrConstraintViolation)
	}

	// Check for duplicate member IDs
	memberSet := make(map[string]bool)
	for _, appID := range policy.MemberAppIDs {
		if appID == "" {
			return fmt.Errorf("%w: member_app_ids cannot contain empty strings", constants.ErrConstraintViolation)
		}
		if memberSet[appID] {
			return fmt.Errorf("%w: duplicate member_app_id: %s", constants.ErrConstraintViolation, appID)
		}
		memberSet[appID] = true
	}

	// Verify every member ID resolves to an enabled TrustedSigner
	for _, appID := range policy.MemberAppIDs {
		pubKey, err := s.signerSvc.GetTrustedSigner(appID)
		if err != nil {
			return fmt.Errorf("%w: failed to verify signer %s: %v", constants.ErrConstraintViolation, appID, err)
		}
		if pubKey == nil {
			return fmt.Errorf("%w: member_app_id %s is not an enabled trusted signer", constants.ErrConstraintViolation, appID)
		}
	}

	// Existence check: prevent silent overwrite of existing tribunals.
	// - New tribunals must be created with Enabled=true.
	// - Existing tribunals may only be updated via Enabled=false (disable path).
	// - Overwriting an existing tribunal with Enabled=true is rejected.
	existing, err := s.GetTribunal(policy.ID)
	if err != nil {
		return fmt.Errorf("%w: failed to check existing tribunal: %v", constants.ErrConstraintViolation, err)
	}
	if existing != nil {
		if policy.Enabled {
			return fmt.Errorf("%w: tribunal %s", constants.ErrAlreadyExists, policy.ID)
		}
	} else {
		if !policy.Enabled {
			return fmt.Errorf("%w", constants.ErrTribunalMustBeEnabled)
		}
	}

	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = time.Now().UTC()
	}
	policy.UpdatedAt = time.Now().UTC()

	data, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}

	return s.docSvc.DocSet(marshaler.CollectionName(constants.CollectionTribunals), policy.ID, data)
}

// ListTribunals returns all TribunalPolicies in the database.
func (s *TribunalStoreService) ListTribunals() ([]models.TribunalPolicy, error) {
	docs, err := s.docSvc.DocQuery(marshaler.CollectionName(constants.CollectionTribunals), nil, "id", 0)
	if err != nil {
		return nil, fmt.Errorf("tribunal store: list: %w", err)
	}

	results := make([]models.TribunalPolicy, 0, len(docs))
	for _, doc := range docs {
		data, err := json.Marshal(doc.Data)
		if err != nil {
			s.logger.Warn("failed to marshal tribunal document", "error", err)
			continue
		}
		var policy models.TribunalPolicy
		if err := json.Unmarshal(data, &policy); err != nil {
			s.logger.Warn("failed to unmarshal tribunal document", "error", err)
			continue
		}
		// id is not in the data map usually, so we set it from doc.ID
		policy.ID = doc.ID
		results = append(results, policy)
	}
	return results, nil
}

// DeleteTribunal removes a TribunalPolicy from the database.
func (s *TribunalStoreService) DeleteTribunal(id string) (bool, error) {
	return s.docSvc.DocDelete(marshaler.CollectionName(constants.CollectionTribunals), id)
}

// isValidTribunalID validates that a tribunal ID contains only allowed characters.
// Allowed: letters, digits, hyphens, and underscores. Must be non-empty.
func isValidTribunalID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
