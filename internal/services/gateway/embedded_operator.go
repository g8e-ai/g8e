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
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

// The embedded operator is the gateway's in-process operator substrate
// (OperatorPubSubService). It carries no certificate: its operators document
// is a binding record whose operator_session_id authorizes the sessions the
// first user's bootstrap binds to it. The document lifecycle is:
//
//  1. gw start registers a pending document (registerPendingEmbeddedOperator)
//     with a deterministic doc ID, claimed=false, empty user_id, and empty
//     operator_session_id. Empty user_id keeps it unclaimable by
//     RegisterDeviceCSR (which matches on user_id); empty
//     operator_session_id keeps it unauthenticated by
//     ValidateOperatorSession (which matches on operator_session_id).
//  2. First-user bootstrap claims it (claimEmbeddedOperator): the explicit
//     human enrollment act binds the operator to the first user and mints
//     its operator_session_id.
//  3. Web-session creation binds the user's claimed embedded operator to
//     the web session via RegistrationService.BindOperators.

// pendingEmbeddedOperatorDocument returns the unclaimed embedded-operator
// document registered at gateway start.
func pendingEmbeddedOperatorDocument(now time.Time) *models.OperatorDocumentGo {
	return &models.OperatorDocumentGo{
		ID:           string(constants.DocIDEmbeddedOperator),
		Component:    constants.ComponentNameG8EO,
		Name:         string(constants.DocIDEmbeddedOperator),
		Status:       constants.OperatorStatusAvailable,
		IsSlot:       false,
		Claimed:      false,
		OperatorType: constants.OperatorTypeEmbedded,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// registerPendingEmbeddedOperator ensures the pending embedded-operator
// document exists. It is idempotent: an existing document (pending or
// claimed) is left untouched. Called once at gateway start; a failure
// aborts startup so the gateway never runs without its operator substrate
// record.
func registerPendingEmbeddedOperator(docStore *DocumentStoreService, logger *slog.Logger) error {
	doc, err := docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	if err != nil {
		return fmt.Errorf("gateway: embedded operator: load pending document: %w", err)
	}
	if doc != nil {
		return nil
	}
	return persistEmbeddedOperatorDocument(docStore, pendingEmbeddedOperatorDocument(time.Now().UTC()))
}

// persistEmbeddedOperatorDocument marshals and stores an embedded-operator
// document at the deterministic doc ID.
func persistEmbeddedOperatorDocument(docStore *DocumentStoreService, op *models.OperatorDocumentGo) error {
	b, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("gateway: embedded operator: marshal document: %w", err)
	}
	if err := docStore.DocSet(marshaler.CollectionName(constants.CollectionOperators), op.ID, b); err != nil {
		return fmt.Errorf("gateway: embedded operator: persist document: %w", err)
	}
	return nil
}

// embeddedOperatorClaimer claims the gateway's embedded operator for a user
// and persists the operator session the claim mints. Satisfied by
// *embeddedOperatorService.
type embeddedOperatorClaimer interface {
	ClaimEmbeddedOperator(userID string) (operatorID, operatorSessionID string, err error)
}

// embeddedOperatorService implements embeddedOperatorClaimer over the
// document store and operator session service so claim sites outside the
// bootstrap controller (browser bootstrap) claim and persist through one
// narrow dependency.
type embeddedOperatorService struct {
	docStore           *DocumentStoreService
	operatorSessionSvc *OperatorSessionService
}

func newEmbeddedOperatorService(docStore *DocumentStoreService, operatorSessionSvc *OperatorSessionService) *embeddedOperatorService {
	return &embeddedOperatorService{docStore: docStore, operatorSessionSvc: operatorSessionSvc}
}

// ClaimEmbeddedOperator claims the embedded operator for userID with an
// empty system fingerprint (browser bootstrap supplies none) and persists
// the operator session. Same-user re-claim returns the document's existing
// operator session ID and re-persists the identical session document, so a
// retried claim stays idempotent.
func (s *embeddedOperatorService) ClaimEmbeddedOperator(userID string) (operatorID, operatorSessionID string, err error) {
	operatorID, operatorSessionID, err = claimEmbeddedOperator(s.docStore, userID, "", time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	if err := s.operatorSessionSvc.PersistOperatorSession(
		operatorSessionID, userID, userID, operatorID, string(constants.HeartbeatTypeBootstrap)); err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: persist operator session: %w", err)
	}
	return operatorID, operatorSessionID, nil
}

// claimEmbeddedOperator binds the embedded operator to userID and mints its
// operator session ID. It is the explicit human enrollment act for the
// gateway's embedded operator substrate.
//
// Semantics:
//   - Document absent: a pending document is created first, then claimed
//     (self-healing for a gateway that lost the pending record), so a
//     bootstrap never fails on a missing pending record.
//   - Already claimed by the same user: no-op returning the document's
//     existing operator_session_id. No rotation, no re-persist — true
//     idempotence, so a retried claim cannot strand a previously minted
//     binding.
//   - Already claimed by a different user: ErrEmbeddedOperatorClaimed. The
//     embedded operator belongs to exactly one user for its lifetime.
//
// Returns the operator document ID and the (possibly newly minted)
// operator session ID the caller must bind sessions to.
func claimEmbeddedOperator(docStore *DocumentStoreService, userID, systemFingerprint string, now time.Time) (operatorID, operatorSessionID string, err error) {
	doc, err := docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	if err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: load document: %w", err)
	}
	if doc == nil {
		if err := persistEmbeddedOperatorDocument(docStore, pendingEmbeddedOperatorDocument(now)); err != nil {
			return "", "", err
		}
		doc, err = docStore.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
		if err != nil {
			return "", "", fmt.Errorf("gateway: embedded operator: reload document: %w", err)
		}
	}

	var op models.OperatorDocumentGo
	b, err := json.Marshal(doc.Data)
	if err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: marshal document data: %w", err)
	}
	if err := json.Unmarshal(b, &op); err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: unmarshal document: %w", err)
	}
	op.ID = doc.ID

	if op.Claimed {
		if op.UserID == userID {
			return op.ID, op.OperatorSessionID, nil
		}
		return "", "", fmt.Errorf("gateway: embedded operator: claimed by %s, not %s: %w", op.UserID, userID, constants.ErrEmbeddedOperatorClaimed)
	}

	operatorSessionID = uuid.NewString()
	type embeddedOperatorClaimUpdate struct {
		UserID            string    `json:"user_id"`
		OrganizationID    string    `json:"organization_id"`
		Status            string    `json:"status"`
		Claimed           bool      `json:"claimed"`
		ClaimedAt         time.Time `json:"claimed_at"`
		SystemFingerprint string    `json:"system_fingerprint"`
		OperatorSessionID string    `json:"operator_session_id"`
		UpdatedAt         time.Time `json:"updated_at"`
	}
	updateBytes, err := json.Marshal(embeddedOperatorClaimUpdate{
		UserID:            userID,
		OrganizationID:    userID,
		Status:            string(constants.OperatorStatusActive),
		Claimed:           true,
		ClaimedAt:         now,
		SystemFingerprint: systemFingerprint,
		OperatorSessionID: operatorSessionID,
		UpdatedAt:         now,
	})
	if err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: marshal claim: %w", err)
	}
	if _, err := docStore.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), op.ID, updateBytes); err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: persist claim: %w", err)
	}
	return op.ID, operatorSessionID, nil
}
