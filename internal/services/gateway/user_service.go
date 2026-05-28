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

	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// UserService handles user management in the Operator Gateway.
// This replaces client's UserService as the authoritative user source.
type UserService struct {
	db     *GatewayDBService
	logger *slog.Logger
}

// NewUserService creates a new UserService.
func NewUserService(db *GatewayDBService, logger *slog.Logger) *UserService {
	return &UserService{
		db:     db,
		logger: logger,
	}
}

// CreateUser creates a new active user with a generated ID.
// Zero-PII: No email or name is stored - only the user ID and passkey credentials.
func (s *UserService) CreateUser() (*models.User, error) {
	return s.createUser(false)
}

// CreateBootstrapUser creates the ephemeral local-owner identity used by
// `./g8e platform start -a`. The resulting user carries IsBootstrap=true so
// the CSR-based registration path can identify and retire it the first time
// a real identity is provisioned.
func (s *UserService) CreateBootstrapUser() (*models.User, error) {
	return s.createUser(true)
}

func (s *UserService) createUser(isBootstrap bool) (*models.User, error) {
	s.logger.Info("[USER-SERVICE] Creating new user", "is_bootstrap", isBootstrap)

	if isBootstrap {
		existingBootstrap, err := s.FindBootstrapUser()
		if err != nil {
			return nil, fmt.Errorf("failed to check for existing bootstrap user: %w", err)
		}
		if existingBootstrap != nil {
			return nil, fmt.Errorf("bootstrap user already exists")
		}
	}

	userID := uuid.New().String()

	// Zero-PII: Only user ID and passkey credentials are stored
	user := &models.User{
		ID:                 userID,
		PasskeyCredentials: []models.PasskeyCredential{},
		Provider:           string(constants.AuthProviderPasskey),
		Status:             constants.UserStatusActive,
		IsBootstrap:        isBootstrap,
	}

	data, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, data); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	s.logger.Info("[USER-SERVICE] User created", "user_id", userID, "is_bootstrap", isBootstrap)
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
	if existing.Status == constants.UserStatusDisabled {
		// Already disabled - idempotent no-op, but still record an audit row
		// so the caller's intent is visible if they retried.
		return s.appendAdminAudit(models.AdminAuditEntry{
			Action:     models.AdminAuditActionRetireLocalOwner,
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
		"status": marshaler.Status(constants.UserStatusDisabled),
	}); err != nil {
		return fmt.Errorf("failed to disable user %s: %w", userID, err)
	}

	if err := s.appendAdminAudit(models.AdminAuditEntry{
		Action:     models.AdminAuditActionRetireLocalOwner,
		Actor:      actorUserID,
		Target:     userID,
		OperatorID: actorOperatorID,
		Details: map[string]interface{}{
			"reason": reason,
		},
	}); err != nil {
		// Audit write failed AFTER state change. Best we can do is log loudly
		// and propagate - the caller (registration) treats this as a hard
		// failure so we never reach a half-state where owner is disabled
		// but the audit trail does not record why.
		return fmt.Errorf("user %s disabled but audit append failed: %w", userID, err)
	}

	s.logger.Info("[USER-SERVICE] User disabled", "user_id", userID, "reason", reason, "actor", actorUserID)
	return nil
}

// FindBootstrapUser returns the single bootstrap user, if any. Multiple
// bootstrap users is a Gateway invariant violation; if more than one row
// is found the call fails closed so callers can refuse to proceed.
func (s *UserService) FindBootstrapUser() (*models.User, error) {
	filters := []models.DocFilter{
		{Field: "is_bootstrap", Op: "==", Value: json.RawMessage("true")},
	}
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionUsers), filters, "", 2)
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
	return s.db.DocSet(marshaler.CollectionName(constants.CollectionAuthAdminAudit), entry.ID, data)
}

// GetByID retrieves a user by ID.
func (s *UserService) GetByID(userID string) (*models.User, error) {
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionUsers), userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	return s.docToUser(doc)
}

// GetOrCreateBySub retrieves a user by subject (JWT sub claim) or creates one if it doesn't exist.
// This is used for JIT user provisioning from external IdP authentication.
func (s *UserService) GetOrCreateBySub(sub string) (*models.User, error) {
	if sub == "" {
		return nil, fmt.Errorf("sub is required")
	}

	// First, try to find an existing user with this sub as the ID
	user, err := s.GetByID(sub)
	if err != nil {
		return nil, fmt.Errorf("failed to query user by sub: %w", err)
	}

	if user != nil {
		s.logger.Debug("[USER-SERVICE] User found by sub", "sub", sub, "user_id", user.ID)
		return user, nil
	}

	// User doesn't exist, check for an active invitation
	invitation, err := s.FindActiveInvitationBySub(sub)
	if err != nil {
		return nil, fmt.Errorf("failed to query invitations: %w", err)
	}
	if invitation == nil {
		s.logger.Warn("[USER-SERVICE] JIT provisioning rejected: no active invitation found", "sub", sub)
		return nil, fmt.Errorf("no active invitation found for sub: %s", sub)
	}

	s.logger.Info("[USER-SERVICE] JIT provisioning new user from JWT via invitation", "sub", sub, "org", invitation.OrganizationID)

	user = &models.User{
		ID:                 sub,
		PasskeyCredentials: []models.PasskeyCredential{},
		Provider:           string(constants.AuthProviderJWT),
		Status:             constants.UserStatusActive,
		OrganizationID:     invitation.OrganizationID,
		Roles:              invitation.Roles,
		IsBootstrap:        false,
	}

	data, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal user: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionUsers), sub, data); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if err := s.ConsumeInvitation(invitation.ID); err != nil {
		s.logger.Error("[USER-SERVICE] Failed to consume invitation after provisioning", "invitation_id", invitation.ID, "error", err)
		// We don't fail the login since the user was created, but log it heavily
	}

	s.logger.Info("[USER-SERVICE] JIT user created", "user_id", sub, "provider", constants.AuthProviderJWT)
	return user, nil

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

	_, err = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionUsers), userID, updateBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return s.GetByID(userID)
}

// HasAnyUsers checks whether any users exist in the system.
func (s *UserService) HasAnyUsers() (bool, error) {
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionUsers), []models.DocFilter{}, "", 1)
	if err != nil {
		return false, err
	}
	return len(docs) > 0, nil
}

// DeleteUser removes a user by ID.
func (s *UserService) DeleteUser(userID string) error {
	deleted, err := s.db.DocDelete(marshaler.CollectionName(constants.CollectionUsers), userID)
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

// PersonaService handles persona management for role-based access control.
type PersonaService struct {
	db     *GatewayDBService
	logger *slog.Logger
}

// NewPersonaService creates a new PersonaService.
func NewPersonaService(db *GatewayDBService, logger *slog.Logger) *PersonaService {
	return &PersonaService{
		db:     db,
		logger: logger,
	}
}

// GetOrCreateDefaultPersonas ensures default personas exist in the database.
func (s *PersonaService) GetOrCreateDefaultPersonas() error {
	defaultPersonas := []models.Persona{
		{
			ID:          string(constants.UserRoleAdmin),
			Name:        string(constants.UserRoleAdmin),
			Description: "Administrator persona with full system access",
			Roles:       []string{string(constants.UserRoleAdmin), "administrator"},
		},
		{
			ID:          "security-analyst",
			Name:        "security-analyst",
			Description: "Security analyst persona for investigation and audit",
			Roles:       []string{"security-analyst", "analyst"},
		},
		{
			ID:          "developer",
			Name:        "developer",
			Description: "Developer persona for development and testing",
			Roles:       []string{"developer", "engineer"},
		},
		{
			ID:          "auditor",
			Name:        "auditor",
			Description: "Auditor persona for read-only audit access",
			Roles:       []string{"auditor"},
		},
		{
			ID:          "default",
			Name:        "default",
			Description: "Default persona for users without specific roles",
			Roles:       []string{},
		},
	}

	for _, persona := range defaultPersonas {
		existing, err := s.GetByID(persona.ID)
		if err != nil {
			return fmt.Errorf("failed to check existing persona %s: %w", persona.ID, err)
		}
		if existing != nil {
			continue
		}

		now := time.Now().UTC()
		persona.CreatedAt = now
		persona.UpdatedAt = now

		data, err := json.Marshal(persona)
		if err != nil {
			return fmt.Errorf("failed to marshal persona %s: %w", persona.ID, err)
		}

		if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionPersonas), persona.ID, data); err != nil {
			return fmt.Errorf("failed to create persona %s: %w", persona.ID, err)
		}

		s.logger.Info("[PERSONA-SERVICE] Default persona created", "persona_id", persona.ID, "name", persona.Name)
	}

	return nil
}

// GetByID retrieves a persona by ID.
func (s *PersonaService) GetByID(id string) (*models.Persona, error) {
	doc, err := s.db.DocGet(marshaler.CollectionName(constants.CollectionPersonas), id)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	return s.docToPersona(doc)
}

// GetAll retrieves all personas.
func (s *PersonaService) GetAll() ([]models.Persona, error) {
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionPersonas), []models.DocFilter{}, "", 100)
	if err != nil {
		return nil, err
	}

	personas := make([]models.Persona, 0, len(docs))
	for _, doc := range docs {
		persona, err := s.docToPersona(doc)
		if err != nil {
			s.logger.Warn("[PERSONA-SERVICE] Failed to convert persona doc", "doc_id", doc.ID, "error", err)
			continue
		}
		personas = append(personas, *persona)
	}

	return personas, nil
}

// MapRolesToPersona maps JWT roles to a binding persona.
// Returns the first matching persona, or "default" if no match is found.
func (s *PersonaService) MapRolesToPersona(roles []string) (string, error) {
	if len(roles) == 0 {
		return "default", nil
	}

	personas, err := s.GetAll()
	if err != nil {
		s.logger.Warn("[PERSONA-SERVICE] Failed to load personas, falling back to default", "error", err)
		return "default", nil
	}

	roleSet := make(map[string]struct{})
	for _, role := range roles {
		roleSet[role] = struct{}{}
	}

	for _, persona := range personas {
		for _, personaRole := range persona.Roles {
			if _, ok := roleSet[personaRole]; ok {
				s.logger.Debug("[PERSONA-SERVICE] Mapped role to persona", "role", personaRole, "persona", persona.ID)
				return persona.ID, nil
			}
		}
	}

	s.logger.Debug("[PERSONA-SERVICE] No persona matched roles, using default", "roles", roles)
	return "default", nil
}

// docToPersona converts a Document to a Persona model.
func (s *PersonaService) docToPersona(doc *models.Document) (*models.Persona, error) {
	data, err := json.Marshal(doc.ForWire())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal doc: %w", err)
	}

	var persona models.Persona
	if err := json.Unmarshal(data, &persona); err != nil {
		return nil, fmt.Errorf("failed to unmarshal persona: %w", err)
	}
	persona.ID = doc.ID
	return &persona, nil
}

// FindActiveInvitationBySub finds an active, unconsumed invitation for the given subject.
func (s *UserService) FindActiveInvitationBySub(sub string) (*models.Invitation, error) {
	if sub == "" {
		return nil, fmt.Errorf("sub is required")
	}

	filters := []models.DocFilter{
		{Field: "sub", Op: "==", Value: []byte(fmt.Sprintf("%q", sub))},
		{Field: "is_consumed", Op: "==", Value: []byte("false")},
	}

	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionInvitations), filters, "", 1)
	if err != nil {
		return nil, fmt.Errorf("failed to query invitations: %w", err)
	}

	if len(docs) == 0 {
		return nil, nil // No active invitation
	}

	var invitation models.Invitation
	docData, err := json.Marshal(docs[0].ForWire())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal doc data: %w", err)
	}

	if err := json.Unmarshal(docData, &invitation); err != nil {
		return nil, fmt.Errorf("failed to unmarshal invitation: %w", err)
	}
	invitation.ID = docs[0].ID

	if !invitation.IsValid() {
		return nil, nil // Expired
	}

	return &invitation, nil
}

// ConsumeInvitation marks an invitation as consumed.
func (s *UserService) ConsumeInvitation(id string) error {
	updates := map[string]interface{}{
		"is_consumed": true,
		"consumed_at": time.Now().UTC().UnixMilli(),
	}

	updateBytes, err := json.Marshal(updates)
	if err != nil {
		return fmt.Errorf("failed to marshal updates: %w", err)
	}

	_, err = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionInvitations), id, updateBytes)
	if err != nil {
		return fmt.Errorf("failed to update invitation: %w", err)
	}

	s.logger.Info("[USER-SERVICE] Invitation consumed", "invitation_id", id)
	return nil
}

// CreateInvitation creates a new invitation for a user to join an organization.
func (s *UserService) CreateInvitation(organizationID, sub, createdBy string, roles []string, ttl time.Duration) (*models.Invitation, error) {
	if organizationID == "" || sub == "" || createdBy == "" {
		return nil, fmt.Errorf("organization_id, sub, and created_by are required")
	}
	if len(roles) == 0 {
		roles = []string{"user"}
	}

	invitation := &models.Invitation{
		ID:             uuid.New().String(),
		OrganizationID: organizationID,
		Sub:            sub,
		Roles:          roles,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now().UTC(),
		ExpiresAt:      time.Now().UTC().Add(ttl),
		IsConsumed:     false,
	}

	data, err := json.Marshal(invitation)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal invitation: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionInvitations), invitation.ID, data); err != nil {
		return nil, fmt.Errorf("failed to save invitation: %w", err)
	}

	s.logger.Info("[USER-SERVICE] Invitation created", "invitation_id", invitation.ID, "sub", sub, "org", organizationID)
	return invitation, nil
}
