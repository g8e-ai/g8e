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
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// SignerStoreService provides trusted signer CRUD operations.
type SignerStoreService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
	docSvc *DocumentStoreService
}

// NewSignerStoreService creates a new signer store service.
func NewSignerStoreService(db *sqliteutil.DB, logger *slog.Logger) *SignerStoreService {
	return &SignerStoreService{
		db:     db,
		logger: logger,
		docSvc: NewDocumentStoreService(db, logger),
	}
}

// GetTrustedSigner retrieves an L2 signer public key from the database.
// Implements governance.SignerStore.
func (s *SignerStoreService) GetTrustedSigner(keyID string) (ed25519.PublicKey, error) {
	doc, err := s.docSvc.DocGet(marshaler.CollectionName(constants.CollectionTrustedSigners), keyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", constants.ErrDocumentStoreUnmarshalData, keyID)
	}
	if doc == nil {
		return nil, nil
	}

	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrDocumentStoreMarshalDocument, err)
	}

	var signer models.TrustedSigner
	if err := json.Unmarshal(data, &signer); err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrDocumentStoreUnmarshalData, err)
	}

	if !signer.Enabled {
		return nil, nil
	}

	pubBytes, err := hex.DecodeString(signer.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrValidationFailed, err)
	}

	if len(pubBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: invalid public key size: %d", constants.ErrValidationFailed, len(pubBytes))
	}

	return ed25519.PublicKey(pubBytes), nil
}

// AddTrustedSigner adds or updates a trusted L2 signer in the database.
func (s *SignerStoreService) AddTrustedSigner(signer models.TrustedSigner) error {
	if signer.ID == "" {
		return fmt.Errorf("%w: signer ID", constants.ErrMissingRequiredField)
	}
	if signer.PublicKey == "" {
		return fmt.Errorf("%w: signer public key", constants.ErrMissingRequiredField)
	}

	if signer.AddedAt.IsZero() {
		signer.AddedAt = time.Now().UTC()
	}

	data, err := json.Marshal(signer)
	if err != nil {
		return fmt.Errorf("%w: %v", constants.ErrDocumentStoreMarshalDocument, err)
	}

	return s.docSvc.DocSet(marshaler.CollectionName(constants.CollectionTrustedSigners), signer.ID, data)
}

// ListTrustedSigners returns all trusted L2 signers in the database.
func (s *SignerStoreService) ListTrustedSigners() ([]models.TrustedSigner, error) {
	docs, err := s.docSvc.DocQuery(marshaler.CollectionName(constants.CollectionTrustedSigners), nil, "id", 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", constants.ErrDocumentStoreUnmarshalData, err)
	}

	results := make([]models.TrustedSigner, 0, len(docs))
	for _, doc := range docs {
		data, err := json.Marshal(doc.Data)
		if err != nil {
			s.logger.Warn("failed to marshal signer document", "error", err)
			continue
		}
		var signer models.TrustedSigner
		if err := json.Unmarshal(data, &signer); err != nil {
			s.logger.Warn("failed to unmarshal signer document", "error", err)
			continue
		}
		// id is not in the data map usually, so we set it from doc.ID
		signer.ID = doc.ID
		results = append(results, signer)
	}
	return results, nil
}

// DeleteTrustedSigner removes a trusted L2 signer from the database.
func (s *SignerStoreService) DeleteTrustedSigner(keyID string) (bool, error) {
	return s.docSvc.DocDelete(marshaler.CollectionName(constants.CollectionTrustedSigners), keyID)
}

// HasTrustedSigners returns true if at least one trusted L2 signer is provisioned in the database.
func (s *SignerStoreService) HasTrustedSigners() (bool, error) {
	filters := []models.DocFilter{
		{Field: "enabled", Op: "==", Value: json.RawMessage("true")},
	}
	docs, err := s.docSvc.DocQuery(marshaler.CollectionName(constants.CollectionTrustedSigners), filters, "", 1)
	if err != nil {
		return false, fmt.Errorf("%w: %v", constants.ErrDocumentStoreUnmarshalData, err)
	}
	return len(docs) > 0, nil
}
