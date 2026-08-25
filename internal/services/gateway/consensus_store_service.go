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
	"time"
	"unicode"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// ConsensusStoreService provides ConsensusPolicy CRUD operations.
type ConsensusStoreService struct {
	db        *sqliteutil.DB
	logger    *slog.Logger
	docSvc    *DocumentStoreService
	signerSvc *SignerStoreService
}

// NewConsensusStoreService creates a new consensus store service.
// docSvc is the shared DocumentStoreService instance from CanonicalDBService.
func NewConsensusStoreService(db *sqliteutil.DB, logger *slog.Logger, docSvc *DocumentStoreService, signerSvc *SignerStoreService) *ConsensusStoreService {
	return &ConsensusStoreService{
		db:        db,
		logger:    logger,
		docSvc:    docSvc,
		signerSvc: signerSvc,
	}
}

// GetConsensus retrieves a ConsensusPolicy by ID from the database.
func (s *ConsensusStoreService) GetConsensus(id string) (*models.ConsensusPolicy, error) {
	doc, err := s.docSvc.DocGet(marshaler.CollectionName(constants.CollectionConsensus), id)
	if err != nil {
		return nil, fmt.Errorf("consensus store: get: %w", err)
	}
	if doc == nil {
		return nil, nil
	}

	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}

	var policy models.ConsensusPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalData, err)
	}
	policy.ID = doc.ID

	return &policy, nil
}

// GetConsensusPolicy implements governance.L2ConsensusPolicyStore by loading
// the ConsensusPolicy via GetConsensus and adapting it to the generic
// L2ConsensusPolicy struct.
func (s *ConsensusStoreService) GetConsensusPolicy(id string) (*governance.L2ConsensusPolicy, error) {
	policy, err := s.GetConsensus(id)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, nil
	}
	return &governance.L2ConsensusPolicy{
		MemberKeyIDs:    policy.MemberAppIDs,
		Quorum:          policy.Quorum,
		RequireDistinct: policy.RequireDistinct,
		Enabled:         policy.Enabled,
	}, nil
}

// AddConsensus adds or updates a ConsensusPolicy in the database.
// Validation at write time, fail-closed:
// - Consensus ID is non-empty and contains only alphanumeric, hyphens, underscores
// - Quorum >= 1
// - Quorum <= len(MemberAppIDs)
// - Every MemberAppID resolves to an enabled TrustedSigner
// - No duplicate member IDs
// - New consensus must be created with Enabled=true (updates may disable)
func (s *ConsensusStoreService) AddConsensus(policy models.ConsensusPolicy) error {
	if policy.ID == "" {
		return fmt.Errorf("%w: consensus ID", constants.ErrMissingRequiredField)
	}
	if !isValidConsensusID(policy.ID) {
		return fmt.Errorf("%w: %s", constants.ErrConsensusInvalidID, policy.ID)
	}
	if len(policy.MemberAppIDs) == 0 {
		return fmt.Errorf("%w: consensus member_app_ids", constants.ErrMissingRequiredField)
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

	// Existence check: prevent silent overwrite of existing consensus.
	// - New consensus must be created with Enabled=true.
	// - Existing consensus may only be updated via Enabled=false (disable path).
	// - Overwriting an existing consensus with Enabled=true is rejected.
	existing, err := s.GetConsensus(policy.ID)
	if err != nil {
		return fmt.Errorf("%w: failed to check existing consensus: %w", constants.ErrConstraintViolation, err)
	}
	if existing != nil {
		if policy.Enabled {
			return fmt.Errorf("%w: consensus %s", constants.ErrAlreadyExists, policy.ID)
		}
	} else {
		if !policy.Enabled {
			return constants.ErrConsensusMustBeEnabled
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

	return s.docSvc.DocSet(marshaler.CollectionName(constants.CollectionConsensus), policy.ID, data)
}

// ListConsensus returns all ConsensusPolicies in the database.
func (s *ConsensusStoreService) ListConsensus() ([]models.ConsensusPolicy, error) {
	docs, err := s.docSvc.DocQuery(marshaler.CollectionName(constants.CollectionConsensus), nil, "id", 0)
	if err != nil {
		return nil, fmt.Errorf("consensus store: list: %w", err)
	}

	results := make([]models.ConsensusPolicy, 0, len(docs))
	for _, doc := range docs {
		data, err := json.Marshal(doc.Data)
		if err != nil {
			s.logger.Warn("failed to marshal consensus document", "error", err)
			continue
		}
		var policy models.ConsensusPolicy
		if err := json.Unmarshal(data, &policy); err != nil {
			s.logger.Warn("failed to unmarshal consensus document", "error", err)
			continue
		}
		// id is not in the data map usually, so we set it from doc.ID
		policy.ID = doc.ID
		results = append(results, policy)
	}
	return results, nil
}

// DeleteConsensus removes a ConsensusPolicy from the database.
func (s *ConsensusStoreService) DeleteConsensus(id string) (bool, error) {
	return s.docSvc.DocDeleteWithResult(marshaler.CollectionName(constants.CollectionConsensus), id)
}

// isValidConsensusID validates that a consensus ID contains only allowed characters.
// Allowed: letters, digits, hyphens, and underscores. Must be non-empty.
func isValidConsensusID(id string) bool {
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
