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

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
)

// UserService handles user management in the Operator substrate.
// This replaces client's UserService as the authoritative user source.
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

// CreateUser creates a new active user with email uniqueness enforcement.
func (s *UserService) CreateUser(email, name string) (*models.User, error) {
	return s.createUser(email, name, false)
}

// CreateBootstrapUser creates the ephemeral local-superadmin identity used by
// `./g8e platform start -a`. The resulting user carries IsBootstrap=true so
// the device-link registration path can identify and retire it the first time
// a real identity is provisioned.
func (s *UserService) CreateBootstrapUser(email, name string) (*models.User, error) {
	return s.createUser(email, name, true)
}

func (s *UserService) createUser(email, name string, isBootstrap bool) (*models.User, error) {
	sanitizedEmail := strings.TrimSpace(strings.ToLower(email))
	if sanitizedEmail == "" {
		return nil, fmt.Errorf("email is required")
	}

	s.logger.Info("[USER-SERVICE] Creating new user", "email", sanitizedEmail, "is_bootstrap", isBootstrap)

	if isBootstrap {
		existingBootstrap, err := s.FindBootstrapUser()
		if err != nil {
			return nil, fmt.Errorf("failed to check for existing bootstrap user: %w", err)
		}
		if existingBootstrap != nil {
			return nil, fmt.Errorf("bootstrap user already exists")
		}
	}

	// Enforce email uniqueness
	existing, err := s.FindByEmail(sanitizedEmail)
	if err != nil {
		return nil, fmt.Errorf("failed to check email uniqueness: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("user with email %s already exists", sanitizedEmail)
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
		Status:             models.UserStatusActive,
		IsBootstrap:        isBootstrap,
	}

	data, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	if err := s.db.DocSet(string(constants.CollectionUsers), userID, data); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	s.logger.Info("[USER-SERVICE] User created", "user_id", userID, "email", sanitizedEmail, "is_bootstrap", isBootstrap)
	return user, nil
}

// Disable transitions a user to UserStatusDisabled and appends an audit row.
// Subsequent reads via GetByID / FindByEmail still return the user (so audit
// trails remain joinable), but every authentication chokepoint MUST reject
// requests bearing a disabled user identity. See `User.IsActive`.
func (s *UserService) Disable(userID, reason, actorUserID, actorOperatorID string) error {
	if userID == "" {
		return fmt.Errorf("user_id is required")
	}
	existing, err := s.GetByID(userID)
	if err != nil {
		return fmt.Errorf("failed to load user %s: %w", userID, err)
	}
	if existing == nil {
		return fmt.Errorf("user not found: %s", userID)
	}
	if existing.Status == models.UserStatusDisabled {
		// Already disabled — idempotent no-op, but still record an audit row
		// so the caller's intent is visible if they retried.
		return s.appendAdminAudit(models.AdminAuditEntry{
			Action:     models.AdminAuditActionRetireLocalSuperadmin,
			Actor:      actorUserID,
			Target:     userID,
			OperatorID: actorOperatorID,
			Details: map[string]interface{}{
				"reason":  reason,
				"noop":    true,
				"comment": "user was already disabled",
			},
		})
	}

	if _, err := s.UpdateUser(userID, map[string]interface{}{
		"status": string(models.UserStatusDisabled),
	}); err != nil {
		return fmt.Errorf("failed to disable user %s: %w", userID, err)
	}

	if err := s.appendAdminAudit(models.AdminAuditEntry{
		Action:     models.AdminAuditActionRetireLocalSuperadmin,
		Actor:      actorUserID,
		Target:     userID,
		OperatorID: actorOperatorID,
		Details: map[string]interface{}{
			"reason": reason,
		},
	}); err != nil {
		// Audit write failed AFTER state change. Best we can do is log loudly
		// and propagate — the caller (registration) treats this as a hard
		// failure so we never reach a half-state where superadmin is disabled
		// but the audit trail does not record why.
		return fmt.Errorf("user %s disabled but audit append failed: %w", userID, err)
	}

	s.logger.Info("[USER-SERVICE] User disabled", "user_id", userID, "reason", reason, "actor", actorUserID)
	return nil
}

// FindBootstrapUser returns the single bootstrap user, if any. Multiple
// bootstrap users is a substrate invariant violation; if more than one row
// is found the call fails closed so callers can refuse to proceed.
func (s *UserService) FindBootstrapUser() (*models.User, error) {
	filters := []models.DocFilter{
		{Field: "is_bootstrap", Op: "==", Value: json.RawMessage("true")},
	}
	docs, err := s.db.DocQuery(string(constants.CollectionUsers), filters, "", 2)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}
	if len(docs) > 1 {
		return nil, fmt.Errorf("invariant violation: %d bootstrap users found, expected at most 1", len(docs))
	}
	return s.docToUser(docs[0])
}

func (s *UserService) appendAdminAudit(entry models.AdminAuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.At.IsZero() {
		entry.At = time.Now().UTC()
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal admin audit entry: %w", err)
	}
	return s.db.DocSet(string(constants.CollectionAuthAdminAudit), entry.ID, data)
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
