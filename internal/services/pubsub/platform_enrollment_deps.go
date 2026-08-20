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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
)

// PlatformEnrollmentDocStore is the document-store subset required by the
// platform enrollment handlers. It is implemented natively by
// gateway.DocumentStoreService; the interface lives in the pubsub package
// so the handler layer does not import the gateway package (which would
// invert the dependency direction: gateway already imports pubsub).
type PlatformEnrollmentDocStore interface {
	DocSet(collection, id string, data []byte) error
	DocGet(collection, id string) ([]byte, error)
	DocConditionalUpdate(collection, id string, setFields map[string]interface{}, conditionField string, conditionValue interface{}) (bool, error)
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
}

// PlatformEnrollmentCLISessions is the CLI-session subset required by the
// platform enrollment handlers. Implemented natively by
// gateway.CLISessionService.
type PlatformEnrollmentCLISessions interface {
	PersistCLISession(cliSessionID, operatorSessionID, userID, systemFingerprint, certFingerprint, certSerial, loginMethod string) error
}

// PlatformEnrollmentOperatorSessions is the operator-session subset
// required by the platform enrollment handlers. Implemented natively by
// gateway.OperatorSessionService.
type PlatformEnrollmentOperatorSessions interface {
	PersistOperatorSession(operatorSessionID, userID, orgID, operatorID, loginMethod string) error
}

// PlatformEnrollmentUsers is the user-service subset required by the
// platform enrollment handlers. Implemented natively by
// gateway.UserService.
type PlatformEnrollmentUsers interface {
	IsFirstUser(userID string) (bool, error)
	HasAnyUsers() (bool, error)
}

// PlatformEnrollmentDeps bundles the gateway-side dependencies required
// by the five platform enrollment handlers registered in buildHandlers.
// All fields are required in gateway mode; the pubsub service fails
// closed at handler dispatch if any is nil. Outbound (operator) mode
// never constructs platform enrollment handlers, so nil is acceptable
// there.
type PlatformEnrollmentDeps struct {
	DocStore         PlatformEnrollmentDocStore
	PKI              PlatformEnrollmentPKI
	CLISessions      PlatformEnrollmentCLISessions
	OperatorSessions PlatformEnrollmentOperatorSessions
	Users            PlatformEnrollmentUsers
}

// platformEnrollmentCollection is the canonical collection name for
// persisted platform enrollment requests, resolved once through the
// marshaler to avoid repeated string conversions at handler dispatch.
func platformEnrollmentCollection() string {
	return marshaler.CollectionName(constants.CollectionPlatformEnrollments)
}

// loadPlatformEnrollmentRequest reads a persisted enrollment request by
// ID and decodes it into the typed model. Returns (nil, nil) when the
// request does not exist so the caller can distinguish not-found from
// decode errors.
func loadPlatformEnrollmentRequest(ctx context.Context, deps PlatformEnrollmentDeps, requestID string) (*models.PlatformEnrollmentRequest, error) {
	_ = ctx
	if requestID == "" {
		return nil, nil
	}
	data, err := deps.DocStore.DocGet(platformEnrollmentCollection(), requestID)
	if err != nil {
		return nil, fmt.Errorf("platform enrollment: load request %s: %w", requestID, err)
	}
	if data == nil {
		return nil, nil
	}
	var req models.PlatformEnrollmentRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("platform enrollment: decode request %s: %w", requestID, err)
	}
	return &req, nil
}

// persistPlatformEnrollmentRequest writes the typed request back to the
// document store as canonical JSON.
func persistPlatformEnrollmentRequest(deps PlatformEnrollmentDeps, req *models.PlatformEnrollmentRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("platform enrollment: marshal request %s: %w", req.ID, err)
	}
	if err := deps.DocStore.DocSet(platformEnrollmentCollection(), req.ID, data); err != nil {
		return fmt.Errorf("platform enrollment: persist request %s: %w", req.ID, err)
	}
	return nil
}

// platformEnrollmentLogger is a typed alias for the slog.Logger used by
// the handler, kept here so the handler struct in
// platform_enrollment_handlers.go does not re-import slog.
type platformEnrollmentLogger = *slog.Logger
