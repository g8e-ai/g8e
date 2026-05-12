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
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

// RegistrationService handles substrate-native device enrollment.
// Ported from g8ed/DeviceLinkService and g8ee/OperatorAuthService.
type RegistrationService struct {
	db     *ListenDBService
	certs  *CertStore
	logger *slog.Logger
}

// NewRegistrationService creates a new RegistrationService.
func NewRegistrationService(db *ListenDBService, certs *CertStore, logger *slog.Logger) *RegistrationService {
	return &RegistrationService{
		db:     db,
		certs:  certs,
		logger: logger,
	}
}

// RegisterDevice handles the registration request from an enrolling operator binary.
func (s *RegistrationService) RegisterDevice(token string, req models.OperatorRegistrationRequest) (*models.OperatorRegistrationResponse, error) {
	s.logger.Info("[REGISTRATION] Registering device", "token", token, "hostname", req.Hostname)

	// 1. Fetch and validate device link
	linkKey := "g8e:device-link:" + token
	storedLink, found := s.db.KVGet(linkKey)
	if !found {
		return nil, fmt.Errorf("device link not found or expired")
	}

	var linkData models.DeviceLinkData
	if err := json.Unmarshal([]byte(storedLink), &linkData); err != nil {
		return nil, fmt.Errorf("failed to parse device link: %w", err)
	}

	if linkData.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("device link expired")
	}

	// 2. Resolve operator slot
	var operator *models.OperatorDocumentGo

	// Try single-operator link first
	if linkData.OperatorID != "" {
		doc, err := s.db.DocGet("operators", linkData.OperatorID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch operator: %w", err)
		}
		if doc == nil {
			return nil, fmt.Errorf("operator slot %s not found", linkData.OperatorID)
		}
		// Convert Document to OperatorDocumentGo
		operator, err = s.toOperatorDoc(doc)
		if err != nil {
			return nil, err
		}
	} else {
		// Multi-use link: find or create slot
		// 1. Try fingerprint match
		filters := []models.DocFilter{
			{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", linkData.UserID))},
			{Field: "system_fingerprint", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", req.SystemFingerprint))},
		}
		docs, err := s.db.DocQuery("operators", filters, "", 1)
		if err == nil && len(docs) > 0 {
			operator, _ = s.toOperatorDoc(docs[0])
		}

		// 2. Try offline slot
		if operator == nil {
			filters = []models.DocFilter{
				{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", linkData.UserID))},
				{Field: "status", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.Status.OperatorStatus.Offline))},
			}
			docs, err = s.db.DocQuery("operators", filters, "", 1)
			if err == nil && len(docs) > 0 {
				operator, _ = s.toOperatorDoc(docs[0])
			}
		}

		// 3. Create new slot
		if operator == nil {
			operator, err = s.createSlot(linkData.UserID, linkData.OrganizationID)
			if err != nil {
				return nil, err
			}
		}
	}

	if operator == nil {
		return nil, fmt.Errorf("failed to resolve operator slot")
	}

	// 3. Update link usage
	// For multi-use links, we would track usage here.
	// For Phase 4 early start, we'll keep it simple.

	// 4. Create session
	sessionID := uuid.NewString()
	session := &models.SessionSummary{
		ID:        sessionID,
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour), // Default 24h
	}

	// 5. Update operator document
	update := map[string]interface{}{
		"status":              constants.Status.OperatorStatus.Active,
		"operator_session_id": sessionID,
		"system_fingerprint":  req.SystemFingerprint,
		"claimed":             true,
		"claimed_at":          time.Now().UTC(),
	}
	updateBytes, _ := json.Marshal(update)
	_, err := s.db.DocUpdate("operators", operator.ID, updateBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to update operator status: %w", err)
	}

	// 6. Generate credentials
	// In the substrate, we'll return the same api_key the operator already has
	// or generate a new one if it's missing.
	apiKey := operator.OperatorAPIKey
	if apiKey == "" {
		apiKey = fmt.Sprintf("g8e_%s_%s", operator.ID[:8], uuid.NewString())
		// Update it in the doc
		s.db.DocUpdate("operators", operator.ID, json.RawMessage(fmt.Sprintf(`{"operator_api_key": %q}`, apiKey)))
	}

	// 7. Return response
	return &models.OperatorRegistrationResponse{
		Success:           true,
		OperatorID:        operator.ID,
		OperatorSessionID: sessionID,
		APIKey:            apiKey,
		Session:           session,
	}, nil
}

func (s *RegistrationService) toOperatorDoc(doc *models.Document) (*models.OperatorDocumentGo, error) {
	b, err := json.Marshal(doc.ForWire())
	if err != nil {
		return nil, err
	}
	var op models.OperatorDocumentGo
	if err := json.Unmarshal(b, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

func (s *RegistrationService) createSlot(userID, orgID string) (*models.OperatorDocumentGo, error) {
	id := uuid.NewString()
	if orgID == "" {
		orgID = userID
	}

	// Simple slot counter logic
	slotNumber := 1
	filters := []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
	}
	docs, err := s.db.DocQuery("operators", filters, "", 0)
	if err == nil {
		slotNumber = len(docs) + 1
	}

	op := &models.OperatorDocumentGo{
		ID:             id,
		UserID:         userID,
		OrganizationID: orgID,
		Component:      "g8eo",
		Name:           fmt.Sprintf("operator-%d", slotNumber),
		Status:         constants.Status.OperatorStatus.Offline,
		SlotNumber:     slotNumber,
		IsSlot:         true,
		OperatorType:   constants.Status.OperatorType.System,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	b, _ := json.Marshal(op)
	if err := s.db.DocSet("operators", id, b); err != nil {
		return nil, err
	}

	return op, nil
}
