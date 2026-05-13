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
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/components/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/components/g8eo/internal/models"
)

// UserService handles user management in the Operator substrate.
// This replaces g8ed's UserService as the authoritative user source.
type UserService struct {
	db     *ListenDBService
	logger *slog.Logger
}

// NewUserService creates a new UserService.
func NewUserService(db *ListenDBService, logger *slog.Logger) *UserService {
	return &UserService{
		db:     db,
		logger: logger,
	}
}

// CreateUser creates a new user with email uniqueness enforcement.
// The first user automatically receives SUPERADMIN role.
func (s *UserService) CreateUser(email, name string, roles []string) (*models.User, error) {
	sanitizedEmail := strings.TrimSpace(strings.ToLower(email))
	if sanitizedEmail == "" {
		return nil, fmt.Errorf("email is required")
	}

	s.logger.Info("[USER-SERVICE] Creating new user", "email", sanitizedEmail)

	// Enforce email uniqueness
	existing, err := s.FindByEmail(sanitizedEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to check email uniqueness: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("user with email %s already exists", sanitizedEmail)
	}

	// Determine roles: first user gets SUPERADMIN
	if roles == nil {
		hasAny, err := s.HasAnyUsers()
		if err != nil {
			return nil, fmt.Errorf("failed to check existing users: %w", err)
		}
		if !hasAny {
			roles = []string{"SUPERADMIN"}
		} else {
			roles = []string{"USER"}
		}
	}

	userID := uuid.New().String()

	userName := name
	if userName == "" {
		userName = strings.Split(sanitizedEmail, "@")[0]
	}

	user := &models.User{
		ID:                 userID,
		Email:              sanitizedEmail,
		Name:               userName,
		PasskeyCredentials: []models.PasskeyCredential{},
		Provider:           "passkey",
	}

	data, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	if err := s.db.DocSet(string(constants.CollectionUsers), userID, data); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	s.logger.Info("[USER-SERVICE] User created", "user_id", userID, "email", sanitizedEmail)
	return user, nil
}

// FindByEmail finds a user by email address.
func (s *UserService) FindByEmail(email string) (*models.User, error) {
	sanitizedEmail := strings.TrimSpace(strings.ToLower(email))
	if sanitizedEmail == "" {
		return nil, nil
	}

	filters := []models.DocFilter{
		{Field: "email", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", sanitizedEmail))},
	}

	docs, err := s.db.DocQuery(string(constants.CollectionUsers), filters, "", 1)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}

	return s.docToUser(docs[0])
}

// GetByID retrieves a user by ID.
func (s *UserService) GetByID(userID string) (*models.User, error) {
	doc, err := s.db.DocGet(string(constants.CollectionUsers), userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	return s.docToUser(doc)
}

// UpdateUser updates a user with the provided field changes.
func (s *UserService) UpdateUser(userID string, updates map[string]interface{}) (*models.User, error) {
	existing, err := s.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("user not found: %s", userID)
	}

	// Add updated_at timestamp
	updates["updated_at"] = time.Now().UTC().UnixMilli()

	updateBytes, err := json.Marshal(updates)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updates: %w", err)
	}

	_, err = s.db.DocUpdate(string(constants.CollectionUsers), userID, updateBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return s.GetByID(userID)
}

// HasAnyUsers checks whether any users exist in the system.
func (s *UserService) HasAnyUsers() (bool, error) {
	docs, err := s.db.DocQuery(string(constants.CollectionUsers), []models.DocFilter{}, "", 1)
	if err != nil {
		return false, err
	}
	return len(docs) > 0, nil
}

// DeleteUser removes a user by ID.
func (s *UserService) DeleteUser(userID string) error {
	deleted, err := s.db.DocDelete(string(constants.CollectionUsers), userID)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("user not found: %s", userID)
	}

	s.logger.Info("[USER-SERVICE] User deleted", "user_id", userID)
	return nil
}

// docToUser converts a Document to a User model.
func (s *UserService) docToUser(doc *models.Document) (*models.User, error) {
	data, err := json.Marshal(doc.ForWire())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal doc: %w", err)
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}
	user.ID = doc.ID
	return &user, nil
}
