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
	"context"
	"encoding/json"
	"fmt"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

// Store is the document persistence the substrate needs.
// *gateway.DocumentStoreService satisfies it.
type Store interface {
	DocGet(ctx context.Context, collection, id string) (*models.Document, error)
	DocSet(ctx context.Context, collection, id string, data json.RawMessage) error
	DocUpdate(ctx context.Context, collection, id string, fields json.RawMessage) (*models.Document, error)
}

// SessionPersister records the operator session a claim mints.
// *gateway.OperatorSessionService satisfies it.
type SessionPersister interface {
	PersistOperatorSession(ctx context.Context, operatorSessionID, userID, orgID, operatorID, loginMethod string) error
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

// ExecutesFor admits only the Operator record owned by this substrate.
// The pending record also executes bootstrap enrollment before a user claims it.
func (s *Service) ExecutesFor(operatorID string) bool {
	return s != nil && operatorID == string(constants.DocIDEmbeddedOperator)
}

func pendingDocument(now time.Time) *operatorv1.OperatorDocument {
	return &operatorv1.OperatorDocument{
		Id:           string(constants.DocIDEmbeddedOperator),
		Component:    string(constants.ComponentNameG8EO),
		Name:         string(constants.DocIDEmbeddedOperator),
		Status:       string(constants.OperatorStatusAvailable),
		IsSlot:       false,
		Claimed:      false,
		OperatorType: string(constants.OperatorTypeEmbedded),
		CreatedAt:    timestamppb.New(now),
		UpdatedAt:    timestamppb.New(now),
	}
}

// RegisterPending ensures the pending embedded-operator document exists.
// It is idempotent: an existing document (pending or claimed) is left
// untouched. Called once at gateway start; a failure aborts startup so the
// gateway never runs without its operator substrate record.
func (s *Service) RegisterPending(ctx context.Context) error {
	doc, err := s.docs.DocGet(ctx, marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	if err != nil {
		return fmt.Errorf("gateway: embedded operator: load pending document: %w", err)
	}
	if doc != nil {
		return nil
	}
	return persistDocument(ctx, s.docs, pendingDocument(time.Now().UTC()))
}

func persistDocument(ctx context.Context, docs Store, op *operatorv1.OperatorDocument) error {
	b, err := models.MarshalOperatorDocument(op)
	if err != nil {
		return fmt.Errorf("gateway: embedded operator: marshal document: %w", err)
	}
	if err := docs.DocSet(ctx, marshaler.CollectionName(constants.CollectionOperators), op.Id, b); err != nil {
		return fmt.Errorf("gateway: embedded operator: persist document: %w", err)
	}
	return nil
}

// ClaimEmbeddedOperator claims the embedded operator for userID with an
// empty system fingerprint (browser bootstrap supplies none) and persists
// the operator session. Same-user re-claim returns the document's existing
// operator session ID and re-persists the identical session document, so a
// retried claim stays idempotent.
func (s *Service) ClaimEmbeddedOperator(ctx context.Context, userID string) (operatorID, operatorSessionID string, err error) {
	operatorID, operatorSessionID, err = s.Claim(ctx, userID, "", time.Now().UTC())
	if err != nil {
		return "", "", err
	}
	if err := s.sessions.PersistOperatorSession(
		ctx, operatorSessionID, userID, userID, operatorID, string(constants.HeartbeatTypeBootstrap)); err != nil {
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
func (s *Service) Claim(ctx context.Context, userID, systemFingerprint string, now time.Time) (operatorID, operatorSessionID string, err error) {
	doc, err := s.docs.DocGet(ctx, marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
	if err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: load document: %w", err)
	}
	if doc == nil {
		if err := persistDocument(ctx, s.docs, pendingDocument(now)); err != nil {
			return "", "", err
		}
		doc, err = s.docs.DocGet(ctx, marshaler.CollectionName(constants.CollectionOperators), string(constants.DocIDEmbeddedOperator))
		if err != nil {
			return "", "", fmt.Errorf("gateway: embedded operator: reload document: %w", err)
		}
	}

	op, err := models.OperatorDocumentFromStore(doc)
	if err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: load document: %w", err)
	}

	if op.Claimed {
		if op.UserId == userID {
			return op.Id, op.OperatorSessionId, nil
		}
		return "", "", fmt.Errorf("gateway: embedded operator: claimed by %s, not %s: %w", op.UserId, userID, constants.ErrEmbeddedOperatorClaimed)
	}

	operatorSessionID, err = uuid.NewString()
	if err != nil {
		return "", "", err
	}
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
	if _, err := s.docs.DocUpdate(ctx, marshaler.CollectionName(constants.CollectionOperators), op.Id, updateBytes); err != nil {
		return "", "", fmt.Errorf("gateway: embedded operator: persist claim: %w", err)
	}
	return op.Id, operatorSessionID, nil
}
