// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// PlatformEnrollmentDocStore is the document-store subset required by the
// platform enrollment handlers. It is implemented natively by
// gateway.DocumentStoreService; the interface lives in the pubsub package
// so the handler layer does not import the gateway package (which would
// invert the dependency direction: gateway already imports pubsub).
type PlatformEnrollmentDocStore interface {
	DocSet(collection, id string, data json.RawMessage) error
	DocGet(collection, id string) (*models.Document, error)
	DocConditionalUpdate(collection, id string, setFields json.RawMessage, conditionField string, conditionValue interface{}) (bool, error)
	DocUpdate(collection, id string, data json.RawMessage) (*models.Document, error)
	DocDelete(collection, id string) error
}

// PlatformEnrollmentPKI is the PKI subset required by the platform
// enrollment handlers. SignPlatformAppCSR issues the dual-SAN app
// certificate for dashboard/ensemble; SignCSR issues operator and CLI
// leaf certificates; GatewayTrustBundle returns the pinned trust bundle.
// Implemented natively by gateway.PKIAuthority.
type PlatformEnrollmentPKI interface {
	SignPlatformAppCSR(csrPEM, appName, userID string) (certPEM, chainPEM string, err error)
	SignCSR(csrPEM string, leafType string, organizationID, operatorID, userID, sessionID, gatewayID string) (certPEM, chainPEM string, err error)
	GatewayTrustBundle() ([]byte, error)
	RevokeCertificate(serial string, reason string) error
}

// PlatformEnrollmentCLISessions is the CLI-session subset required by the
// platform enrollment handlers. Implemented natively by
// gateway.CLISessionService.
type PlatformEnrollmentCLISessions interface {
	PersistCLISession(cliSessionID, operatorSessionID, userID, systemFingerprint, certFingerprint, certSerial, loginMethod string) error
	DeactivateCLISession(sessionID string) error
}

// PlatformEnrollmentOperatorSessions is the operator-session subset
// required by the platform enrollment handlers. Implemented natively by
// gateway.OperatorSessionService.
type PlatformEnrollmentOperatorSessions interface {
	PersistOperatorSession(operatorSessionID, userID, orgID, operatorID, loginMethod string) error
	DeactivateOperatorSession(operatorSessionID string) error
}

type PlatformEnrollmentConnections interface {
	DisconnectIdentity(spiffeID string) int
}

// PlatformEnrollmentDeps bundles the gateway-side dependencies required
// by the five platform enrollment handlers registered in buildHandlers.
// All fields are required in gateway mode; the pubsub service fails
// closed at handler dispatch if any is nil. Outbound (operator) mode
// never constructs platform enrollment handlers, so nil is acceptable
// there. Owner authorization (IsFirstUser) is enforced by the
// enrollment service before submitting the DECIDE envelope, not by the
// handlers, so UserService is not part of this bundle.
type PlatformEnrollmentDeps struct {
	DocStore         PlatformEnrollmentDocStore
	PKI              PlatformEnrollmentPKI
	CLISessions      PlatformEnrollmentCLISessions
	OperatorSessions PlatformEnrollmentOperatorSessions
	Connections      PlatformEnrollmentConnections
	Posture          string
}

// platformEnrollmentCollection is the canonical collection name for
// persisted platform enrollment requests, resolved once through the
// marshaler to avoid repeated string conversions at handler dispatch.
func platformEnrollmentCollection() string {
	return marshaler.CollectionName(constants.CollectionPlatformEnrollments)
}

func loadPlatformEnrollmentOrganization(deps PlatformEnrollmentDeps, userID string) (*models.User, *models.Organization, error) {
	userDoc, err := deps.DocStore.DocGet(marshaler.CollectionName(constants.CollectionUsers), userID)
	if err != nil {
		return nil, nil, fmt.Errorf("platform enrollment: load user %s: %w", userID, err)
	}
	if userDoc == nil {
		return nil, nil, constants.ErrUserNotFound
	}
	userData, err := json.Marshal(userDoc.Data)
	if err != nil {
		return nil, nil, fmt.Errorf("platform enrollment: marshal user %s: %w", userID, err)
	}
	var user models.User
	if err := json.Unmarshal(userData, &user); err != nil {
		return nil, nil, fmt.Errorf("platform enrollment: decode user %s: %w", userID, err)
	}
	user.ID = userDoc.ID
	if user.OrganizationID == "" {
		return nil, nil, constants.ErrOrganizationIDRequired
	}
	organizationDoc, err := deps.DocStore.DocGet(marshaler.CollectionName(constants.CollectionOrganizations), user.OrganizationID)
	if err != nil {
		return nil, nil, fmt.Errorf("platform enrollment: load organization %s: %w", user.OrganizationID, err)
	}
	if organizationDoc == nil {
		return nil, nil, constants.ErrOrganizationNotFound
	}
	organizationData, err := json.Marshal(organizationDoc.Data)
	if err != nil {
		return nil, nil, fmt.Errorf("platform enrollment: marshal organization %s: %w", user.OrganizationID, err)
	}
	var organization models.Organization
	if err := json.Unmarshal(organizationData, &organization); err != nil {
		return nil, nil, fmt.Errorf("platform enrollment: decode organization %s: %w", user.OrganizationID, err)
	}
	organization.ID = organizationDoc.ID
	member := organization.OwnerUserID == user.ID
	for _, memberUserID := range organization.MemberUserIDs {
		member = member || memberUserID == user.ID
	}
	if organization.ID != user.OrganizationID || !member {
		return nil, nil, constants.ErrOrganizationMembershipInvalid
	}
	return &user, &organization, nil
}

// loadPlatformEnrollmentRequest reads a persisted enrollment request by
// ID and decodes it into the typed model. Returns (nil, nil) when the
// request does not exist so the caller can distinguish not-found from
// decode errors.
func loadPlatformEnrollmentRequest(ctx context.Context, deps PlatformEnrollmentDeps, requestID string) (*models.PlatformEnrollmentRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("platform enrollment: check request context: %w", err)
	}
	if requestID == "" {
		return nil, nil
	}
	doc, err := deps.DocStore.DocGet(platformEnrollmentCollection(), requestID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: load request %s: %w", requestID, err)
	}
	if doc == nil {
		return nil, nil
	}
	// Re-marshal the document's field map back to a JSON object so the
	// typed model can be decoded with a single json.Unmarshal call.
	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: marshal doc %s: %w", requestID, err)
	}
	var req models.PlatformEnrollmentRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("platform enrollment: decode request %s: %w", requestID, err)
	}
	return &req, nil
}

// platformEnrollmentLogger is a typed alias for the slog.Logger used by
// the handler, kept here so the handler struct in
// platform_enrollment_handlers.go does not re-import slog.
type platformEnrollmentLogger = *slog.Logger
