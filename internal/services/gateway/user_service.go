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
	"os/user"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
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
func NewUserService(docStore *DocumentStoreService, logger *slog.Logger) *UserService {
	return &UserService{
		db:     docStore,
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
// The LocalOSUser field is left nil; callers that need to attach OS user info
// (the bootstrap path) use CreateUserWithOSUser.
func (s *UserService) CreateUser() (*models.User, error) {
	return s.createUser(nil, nil)
}

// CreateUserWithOSUser creates a new active user with a generated ID, the
// provided local OS user info attached, and the owner role. Used by the CLI
// bootstrap path (handleLocalBootstrapWithURL) so the first human enrollee
// carries the zero-PII OS-user metadata from the enrollment request and is
// marked as the gateway owner.
func (s *UserService) CreateUserWithOSUser(localOSUser *models.LocalOSUser) (*models.User, error) {
	return s.createUser(localOSUser, []string{string(constants.UserRoleOwner)})
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

func (s *UserService) createUser(localOSUser *models.LocalOSUser, roles []string) (*models.User, error) {
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
		LocalOSUser:        localOSUser,
		WebAuthnUserID:     webAuthnUserID,
		Roles:              roles,
	}

	data, err := json.Marshal(user)
	if err != nil {
		return nil, fmt.Errorf("user service: failed to marshal user: %w", err)
	}

	if err := s.db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, data); err != nil {
		return nil, fmt.Errorf("user service: failed to store user: %w", err)
	}

	s.logger.Info("[USER-SERVICE] User created", "user_id", userID)
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

// IsFirstUser reports whether the given userID is the first user ever
// created in the system (the first human enrollee). The first user created
// via `auth enroll user` is the gateway owner and admin; admin endpoints
// gate on this check instead of the removed IsBootstrap flag. The first
// user remains admin permanently regardless of how many users are added
// later. Returns false when the user does not exist or is not the first
// user by creation order.
func (s *UserService) IsFirstUser(userID string) (bool, error) {
	if userID == "" {
		return false, constants.ErrUserIDRequired
	}
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionUsers), []models.DocFilter{}, "", 0)
	if err != nil {
		return false, fmt.Errorf("user service: failed to query users for first-user check: %w", err)
	}
	if len(docs) == 0 {
		return false, nil
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].CreatedAt.Before(docs[j].CreatedAt) })
	return docs[0].ID == userID, nil
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

// List retrieves all users in the system.
func (s *UserService) List() ([]models.User, error) {
	docs, err := s.db.DocQuery(marshaler.CollectionName(constants.CollectionUsers), []models.DocFilter{}, "", 0)
	if err != nil {
		return nil, fmt.Errorf("user service: list users: %w", err)
	}

	users := make([]models.User, 0, len(docs))
	for _, doc := range docs {
		user, err := s.docToUser(doc)
		if err != nil {
			s.logger.Warn("user service: list users: decode failed", "doc_id", doc.ID, "error", err)
			continue
		}
		users = append(users, *user)
	}
	return users, nil
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
	deleted, err := s.db.DocDeleteWithResult(marshaler.CollectionName(constants.CollectionUsers), userID)
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
func NewPersonaService(docStore *DocumentStoreService, logger *slog.Logger) *PersonaService {
	return &PersonaService{
		db:     docStore,
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
