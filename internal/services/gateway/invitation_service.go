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

// InvitationService handles invitations in the Operator Gateway.
type InvitationService struct {
	db     *CanonicalDBService
	logger *slog.Logger
}

// NewInvitationService creates a new InvitationService.
func NewInvitationService(db *CanonicalDBService, logger *slog.Logger) *InvitationService {
	return &InvitationService{
		db:     db,
		logger: logger,
	}
}

// CreateInvitation creates a new invitation for a user to join an organization.
func (s *InvitationService) CreateInvitation(organizationID, sub, createdBy string, roles []string, ttl time.Duration) (*models.Invitation, error) {
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

	s.logger.Info("[INVITATION-SERVICE] Invitation created", "invitation_id", invitation.ID, "sub", sub, "org", organizationID)
	return invitation, nil
}

// FindActiveInvitationBySub finds an active, unconsumed invitation for the given subject.
func (s *InvitationService) FindActiveInvitationBySub(sub string) (*models.Invitation, error) {
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
func (s *InvitationService) ConsumeInvitation(id string) error {
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

	s.logger.Info("[INVITATION-SERVICE] Invitation consumed", "invitation_id", id)
	return nil
}
