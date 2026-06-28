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
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/uuid"
)

// authCacheInvalidator defines the auth service operations needed by UserService
type authCacheInvalidator interface {
	InvalidateUserCache(userID string)
}

// UserService handles user management in the Operator Gateway.
// This replaces client's UserService as the authoritative user source.
type UserService struct {
	db      *DocumentStoreService
	logger  *slog.Logger
	authSvc authCacheInvalidator // Optional: for cache invalidation on user changes
}

// NewUserService creates a new UserService.
func NewUserService(db *CanonicalDBService, logger *slog.Logger) *UserService {
	return &UserService{
		db:     db.DocStore,
		logger: logger,
	}
}

// SetAuthService sets the auth service for cache invalidation.
// This is optional; if nil, cache invalidation is skipped.
func (s *UserService) SetAuthService(authSvc authCacheInvalidator) {
	s.authSvc = authSvc
}

// CreateUser creates a new active user with a generated ID.
// Zero-PII: No email or name is stored - only the user ID and passkey credentials.
func (s *UserService) CreateUser() (*models.User, error) {
	return s.createUser(false, nil)
}

// CreateBootstrapUserWithOSUser creates the ephemeral local-owner identity used by
// `./g8e gw start -a`. The resulting user carries IsBootstrap=true so
// the CSR-based registration path can identify and retire it the first time
// a real identity is provisioned.
// If localOSUser is nil, it falls back to the gateway's local OS user.
func (s *UserService) CreateBootstrapUserWithOSUser(localOSUser *models.LocalOSUser) (*models.User, error) {
	return s.createUser(true, localOSUser)
}

// CreateUserWithSub creates a user with the provided subject (JWT sub) as their ID.
// Used for JIT provisioning when a valid JWT is presented for an unknown identity.
func (s *UserService) CreateUserWithSub(sub string) (*models.User, error) {
	if sub == "" {
		return nil, constants.ErrJWTSessionSubjectMissing
	}

	existing, err := s.GetByID(sub)
	if err != nil {
		return nil, fmt.Errorf("user service: JIT lookup failed: %w", err)
	}
	if existing != nil {
		return existing, nil
	}

	user := &models.User{
		ID:                 sub,
		PasskeyCredentials: []models.PasskeyCredential{},
		Provider:           string(constants.AuthProviderJWT),
		Status:             constants.UserStatusActive,
		IsBootstrap:        false,
		WebAuthnUserID:     uuid.NewString(),
	}

	data, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("user service: failed to marshal JIT user: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionUsers), sub, data); err != nil {
		return nil, fmt.Errorf("user service: failed to store JIT user: %w", err)
	}

	s.logger.Info("[USER-SERVICE] JIT user provisioned", "user_id", sub)
	return user, nil
}

func getLocalOSUser() *models.LocalOSUser {
	currentUser, err := user.Current()
	if err != nil {
		return nil
	}

	var domain, username string
	parts := strings.SplitN(currentUser.Username, "\\", 2)
	if len(parts) == 2 {
		domain = parts[0]
		username = parts[1]
	} else {
		username = currentUser.Username
	}

	var sid string
	if runtime.GOOS == "windows" {
		sid = currentUser.Uid
	}

	return &models.LocalOSUser{
		Domain:   domain,
		Username: username,
		UID:      currentUser.Uid,
		GID:      currentUser.Gid,
		SID:      sid,
	}
}

func (s *UserService) createUser(isBootstrap bool, localOSUser *models.LocalOSUser) (*models.User, error) {
	s.logger.Info("[USER-SERVICE] Creating new user", "is_bootstrap", isBootstrap)

	if isBootstrap {
		existingBootstrap, err := s.FindBootstrapUser()
		if err != nil {
			return nil, fmt.Errorf("user service: failed to find bootstrap user: %w", err)
		}
		if existingBootstrap != nil {
			return nil, constants.ErrAlreadyExists
		}
	}

	userID := uuid.NewString()

	// Use provided OS user info, or fall back to gateway's local OS user
	if localOSUser == nil {
		localOSUser = getLocalOSUser()
	}

	// Generate WebAuthnUserID for v4 compliance (Windows Hello requires a GUID, not SID)
	// This is a stable 16-byte GUID used for WebAuthn operations
	webAuthnUserID := uuid.NewString()

	// Zero-PII: Only user ID and passkey credentials are stored
	user := &models.User{
		ID:                 userID,
		PasskeyCredentials: []models.PasskeyCredential{},
		Provider:           string(constants.AuthProviderPasskey),
		Status:             constants.UserStatusActive,
		IsBootstrap:        isBootstrap,
		LocalOSUser:        localOSUser,
		WebAuthnUserID:     webAuthnUserID,
	}

	data, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("user service: failed to marshal user: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, data); err != nil {
		return nil, fmt.Errorf("user service: failed to store user: %w", err)
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
		return constants.ErrUserIDRequired
	}
	existing, err := s.GetByID(userID)
	if err != nil {
		return fmt.Errorf("user service: failed to get user by ID: %w", err)
	}
	if existing == nil {
		return constants.ErrUserNotFound
	}
	if existing.Status == constants.UserStatusDisabled {
		// Already disabled - idempotent no-op, but still record an audit row
		// so the caller's intent is visible if they retried.
		return s.appendAdminAudit(models.AdminAuditEntry{
			Action:     models.AdminAuditActionRetireLocalOwner,
			Actor:      actorUserID,
			Target:     userID,
			OperatorID: actorOperatorID,
			Details: &models.AdminAuditDetails{
				Reason:  reason,
				Noop:    true,
				Comment: "user was already disabled",
			},
		})
	}

	if err := s.updateUserStatus(userID, constants.UserStatusDisabled); err != nil {
		return fmt.Errorf("user service: failed to update user status: %w", err)
	}

	// Invalidate auth cache for this user
	if s.authSvc != nil {
		s.authSvc.InvalidateUserCache(userID)
	}

	if err := s.appendAdminAudit(models.AdminAuditEntry{
		Action:     models.AdminAuditActionRetireLocalOwner,
		Actor:      actorUserID,
		Target:     userID,
		OperatorID: actorOperatorID,
		Details: &models.AdminAuditDetails{
			Reason: reason,
		},
	}); err != nil {
		// Audit write failed AFTER state change. Best we can do is log loudly
		// and propagate - the caller (registration) treats this as a hard
		// failure so we never reach a half-state where owner is disabled
		// but the audit trail does not record why.
		return fmt.Errorf("user service: failed to append admin audit: %w", err)
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
		return nil, fmt.Errorf("user service: failed to query bootstrap users: %w", err)
	}
	if len(docs) == 0 {
		return nil, nil
	}
	if len(docs) > 1 {
		return nil, fmt.Errorf("%w: %d bootstrap users found, expected at most 1", constants.ErrConstraintViolation, len(docs))
	}
	return s.docToUser(docs[0])
}

func (s *UserService) appendAdminAudit(entry models.AdminAuditEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.At.IsZero() {
		entry.At = time.Now().UTC()
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("user service: failed to marshal admin audit entry: %w", err)
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

// GetBySub retrieves a user by subject (JWT sub claim).
func (s *UserService) GetBySub(sub string) (*models.User, error) {
	if sub == "" {
		return nil, constants.ErrMissingRequiredField
	}
	return s.GetByID(sub)
}

// updateUserStatus updates a user's status field.
func (s *UserService) updateUserStatus(userID string, status constants.UserStatus) error {
	updates := map[string]interface{}{
		"status":     marshaler.Status(status),
		"updated_at": time.Now().UTC().UnixMilli(),
	}

	updateBytes, err := json.Marshal(updates)
	if err != nil {
		return constants.ErrDocumentStoreMarshalDocument
	}

	_, err = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionUsers), userID, updateBytes)
	if err != nil {
		return err
	}

	return nil
}

// UpdatePasskeyCredentials updates a user's passkey credentials.
func (s *UserService) UpdatePasskeyCredentials(userID string, credentials []models.PasskeyCredential) error {
	updates := map[string]interface{}{
		"passkey_credentials": credentials,
		"updated_at":          time.Now().UTC().UnixMilli(),
	}

	updateBytes, err := json.Marshal(updates)
	if err != nil {
		return constants.ErrDocumentStoreMarshalDocument
	}

	_, err = s.db.DocUpdate(marshaler.CollectionName(constants.CollectionUsers), userID, updateBytes)
	if err != nil {
		return err
	}

	return nil
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
		return constants.ErrUserNotFound
	}

	// Invalidate auth cache for this user
	if s.authSvc != nil {
		s.authSvc.InvalidateUserCache(userID)
	}

	s.logger.Info("[USER-SERVICE] User deleted", "user_id", userID)
	return nil
}

// docToUser converts a Document to a User model.
func (s *UserService) docToUser(doc *models.Document) (*models.User, error) {
	data, err := json.Marshal(doc.ForWire())
	if err != nil {
		return nil, fmt.Errorf("user service: failed to marshal user document: %w", err)
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("user service: failed to unmarshal user document: %w", err)
	}
	user.ID = doc.ID
	return &user, nil
}

// PersonaService handles persona management for role-based access control.
type PersonaService struct {
	db     *DocumentStoreService
	logger *slog.Logger
}

// NewPersonaService creates a new PersonaService.
func NewPersonaService(db *CanonicalDBService, logger *slog.Logger) *PersonaService {
	return &PersonaService{
		db:     db.DocStore,
		logger: logger,
	}
}

// DefaultPersonaDefinitions returns the list of default personas.
func DefaultPersonaDefinitions() []models.Persona {
	return []models.Persona{
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
}

// CreatePersona creates a new persona.
func (s *PersonaService) CreatePersona(persona *models.Persona) error {
	now := time.Now().UTC()
	persona.CreatedAt = now
	persona.UpdatedAt = now

	data, err := json.Marshal(persona)
	if err != nil {
		return fmt.Errorf("persona service: failed to marshal persona: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionPersonas), persona.ID, data); err != nil {
		return fmt.Errorf("persona service: failed to store persona: %w", err)
	}

	s.logger.Info("[PERSONA-SERVICE] Persona created", "persona_id", persona.ID, "name", persona.Name)
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
		return nil, fmt.Errorf("persona service: failed to marshal persona document: %w", err)
	}

	var persona models.Persona
	if err := json.Unmarshal(data, &persona); err != nil {
		return nil, fmt.Errorf("persona service: failed to unmarshal persona document: %w", err)
	}
	persona.ID = doc.ID
	return &persona, nil
}
