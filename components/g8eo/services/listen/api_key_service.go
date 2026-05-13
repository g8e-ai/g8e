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

package listen

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

const (
	apiKeyPrefix = "g8e-"
	apiKeyLength = 32
)

// ApiKeyService handles the issuance and validation of download API keys.
type ApiKeyService struct {
	db     *ListenDBService
	logger *slog.Logger
}

// NewApiKeyService creates a new ApiKeyService.
func NewApiKeyService(db *ListenDBService, logger *slog.Logger) *ApiKeyService {
	return &ApiKeyService{
		db:     db,
		logger: logger,
	}
}

// IssueDownloadKey generates and stores a new download API key for a user.
func (s *ApiKeyService) IssueDownloadKey(userID, orgID string) (string, error) {
	rawKey, err := s.generateRawKey()
	if err != nil {
		return "", err
	}

	docID := s.makeDocID(rawKey)
	now := time.Now().UnixMilli()

	keyDoc := map[string]interface{}{
		"id":              docID,
		"user_id":         userID,
		"organization_id": orgID,
		"status":          string(constants.Status.OperatorStatus.Active),
		"created_at":      now,
		"last_used_at":    0,
	}

	data, err := json.Marshal(keyDoc)
	if err != nil {
		return "", err
	}

	if err := s.db.DocSet(string(constants.CollectionAPIKeys), docID, data); err != nil {
		return "", fmt.Errorf("failed to store API key: %w", err)
	}

	// Also update the user's g8e_key field for fast lookup
	userUpdates := map[string]interface{}{
		"g8e_key": rawKey,
	}
	userUpdateBytes, _ := json.Marshal(userUpdates)
	_, _ = s.db.DocUpdate(string(constants.CollectionUsers), userID, userUpdateBytes)

	s.logger.Info("[API-KEY-SERVICE] Issued download key", "user_id", userID, "key_prefix", rawKey[:10])
	return rawKey, nil
}

// ValidateKey checks if a raw API key is valid.
func (s *ApiKeyService) ValidateKey(rawKey string) (*models.Document, error) {
	if !strings.HasPrefix(rawKey, apiKeyPrefix) {
		return nil, fmt.Errorf("invalid key format")
	}

	docID := s.makeDocID(rawKey)
	doc, err := s.db.DocGet(string(constants.CollectionAPIKeys), docID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("key not found")
	}

	// Check status
	var status string
	if statusVal, ok := doc.Data["status"]; ok {
		json.Unmarshal(statusVal, &status)
	}

	if status != constants.Status.OperatorStatus.Active {
		return nil, fmt.Errorf("key is %s", status)
	}

	// Update last used
	updates := map[string]interface{}{
		"last_used_at": time.Now().UnixMilli(),
	}
	updateBytes, _ := json.Marshal(updates)
	_, _ = s.db.DocUpdate(string(constants.CollectionAPIKeys), docID, updateBytes)

	return doc, nil
}

// RevokeKey revokes an API key.
func (s *ApiKeyService) RevokeKey(rawKey string) error {
	docID := s.makeDocID(rawKey)
	_, err := s.db.DocDelete(string(constants.CollectionAPIKeys), docID)
	return err
}

func (s *ApiKeyService) generateRawKey() (string, error) {
	b := make([]byte, apiKeyLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return apiKeyPrefix + hex.EncodeToString(b), nil
}

func (s *ApiKeyService) makeDocID(rawKey string) string {
	// Use the first 16 chars of the hex part as doc ID to avoid storing full key in index
	return rawKey[:20]
}
