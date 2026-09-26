// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package embedded is the gateway's in-process Operator substrate.
//
// It owns the embedded operator document: pending registration at gateway
// start, the single-user claim that mints an operator session, and
// persistence of that session for browser bootstrap. Document IDs, claim
// rules, and error sentinels match the previous gateway helpers.
//
// HTTP routing stays in the parent gateway package (operator_controller.go
// is a shell over registration and dispatch). Web-session binding stays on
// RegistrationService. The outbound Operator runtime is services.G8eoService,
// not this package.
package embedded

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

// Store is the document persistence the substrate needs.
// *gateway.DocumentStoreService satisfies it.
type Store interface {
	DocGet(collection, id string) (*models.Document, error)
	DocSet(collection, id string, data json.RawMessage) error
	DocUpdate(collection, id string, fields json.RawMessage) (*models.Document, error)
}

// SessionPersister records the operator session a claim mints.
// *gateway.OperatorSessionService satisfies it.
type SessionPersister interface {
	PersistOperatorSession(operatorSessionID, userID, orgID, operatorID, loginMethod string) error
}

// Service is the gateway's in-process embedded Operator substrate.
//
// The embedded operator carries no certificate: its operators document is a
// binding record whose operator_session_id authorizes the sessions the first
// user's bootstrap binds to it. The document lifecycle is:
//
//  1. Gateway start registers a pending document (RegisterPending) with a
//     deterministic doc ID, claimed=false, empty user_id, and empty
//     operator_session_id. Empty user_id keeps it unclaimable by
//     RegisterDeviceCSR (which matches on user_id); empty
//     operator_session_id keeps it unauthenticated by
//     ValidateOperatorSession (which matches on operator_session_id).
//  2. First-user bootstrap claims it (Claim): the explicit human enrollment
//     act binds the operator to the first user and mints its
//     operator_session_id.
//  3. Web-session creation binds the user's claimed embedded operator to
//     the web session via RegistrationService.BindOperators.
type Service struct {
	docs     Store
	sessions SessionPersister
}

// New constructs the substrate over the gateway document store and the
// operator-session persister. Both are required for ClaimEmbeddedOperator;
// Claim itself only writes the operator document.
func New(docs Store, sessions SessionPersister) *Service {
	return &Service{docs: docs, sessions: sessions}
}

func pendingDocument(now time.Time) *models.OperatorDocumentGo {
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

// RegisterPending ensures the pending embedded-operator document exists.
// It is idempotent: an existing document (pending or claimed) is left
// untouched. Called once at gateway start; a failure aborts startup so the
// gateway never runs without its operator substrate record.
func (s *Service) RegisterPending() error {
	doc, err := s.docs.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	if err != nil {
		return fmt.Errorf("gateway: embedded operator: load pending document: %w", err)
	}
	if doc != nil {
		return nil
	}
	return persistDocument(s.docs, pendingDocument(time.Now().UTC()))
}

func persistDocument(docs Store, op *models.OperatorDocumentGo) error {
	b, err := json.Marshal(op)
	if err != nil {
		return fmt.Errorf("gateway: embedded operator: marshal document: %w", err)
	}
	if err := docs.DocSet(marshaler.CollectionName(constants.CollectionOperators), op.ID, b); err != nil {
		return fmt.Errorf("gateway: embedded operator: persist document: %w", err)
	}
	return nil
}

// ClaimEmbeddedOperator claims the embedded operator for userID with an
// empty system fingerprint (browser bootstrap supplies none) and persists
// the operator session. Same-user re-claim returns the document's existing
// operator session ID and re-persists the identical session document, so a
// retried claim stays idempotent.
func (s *Service) ClaimEmbeddedOperator(userID string) (operatorID, operatorSessionID string, err error) {
	operatorID, operatorSessionID, err = s.Claim(userID, "", time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	if err := s.sessions.PersistOperatorSession(
		operatorSessionID, userID, userID, operatorID, string(constants.HeartbeatTypeBootstrap)); err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: persist operator session: %w", err)
	}
	return operatorID, operatorSessionID, nil
}

// Claim binds the embedded operator to userID and mints its operator
// session ID. It is the explicit human enrollment act for the gateway's
// embedded operator substrate. The caller persists the operator session
// when it owns that write (CLI bootstrap). ClaimEmbeddedOperator persists
// it for browser bootstrap.
//
// Semantics:
//   - Document absent: a pending document is created first, then claimed
//     (self-healing for a gateway that lost the pending record), so a
//     bootstrap never fails on a missing pending record.
//   - Already claimed by the same user: no-op returning the document's
//     existing operator_session_id. No rotation, no re-persist of the
//     operator document — true idempotence, so a retried claim cannot
//     strand a previously minted binding.
//   - Already claimed by a different user: ErrEmbeddedOperatorClaimed. The
//     embedded operator belongs to exactly one user for its lifetime.
//
// Returns the operator document ID and the (possibly newly minted)
// operator session ID the caller must bind sessions to.
func (s *Service) Claim(userID, systemFingerprint string, now time.Time) (operatorID, operatorSessionID string, err error) {
	doc, err := s.docs.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	if err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: load document: %w", err)
	}
	if doc == nil {
		if err := persistDocument(s.docs, pendingDocument(now)); err != nil {
			return "", "", err
		}
		doc, err = s.docs.DocGet(marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
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
	type claimUpdate struct {
		UserID            string    `json:"user_id"`
		OrganizationID    string    `json:"organization_id"`
		Status            string    `json:"status"`
		Claimed           bool      `json:"claimed"`
		ClaimedAt         time.Time `json:"claimed_at"`
		SystemFingerprint string    `json:"system_fingerprint"`
		OperatorSessionID string    `json:"operator_session_id"`
		UpdatedAt         time.Time `json:"updated_at"`
	}
	updateBytes, err := json.Marshal(claimUpdate{
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
	if _, err := s.docs.DocUpdate(marshaler.CollectionName(constants.CollectionOperators), op.ID, updateBytes); err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: persist claim: %w", err)
	}
	return op.ID, operatorSessionID, nil
}
