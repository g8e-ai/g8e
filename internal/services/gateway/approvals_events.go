// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
)

// ApprovalsChangePublisher pushes g8e.v1.platform.approvals.changed to the
// dashboard sessions of the user whose pending approvals changed, so the
// console re-lists on the event instead of polling.
//
// The event is an invalidation, not a record: it carries which list changed
// and nothing about its content. A failed push is logged and never fails the
// mutation that caused it, and the event is ephemeral, so a console that missed
// one reconciles by re-listing when its stream reconnects.
type ApprovalsChangePublisher struct {
	docStore  *DocumentStoreService
	userSvc   *UserService
	publisher *SSEEventPublisher
	logger    *slog.Logger
}

// NewApprovalsChangePublisher creates a publisher that resolves the affected
// user's web sessions from docStore and the platform owner from userSvc.
func NewApprovalsChangePublisher(docStore *DocumentStoreService, userSvc *UserService, publisher *SSEEventPublisher, logger *slog.Logger) *ApprovalsChangePublisher {
	return &ApprovalsChangePublisher{docStore: docStore, userSvc: userSvc, publisher: publisher, logger: logger}
}

// TransactionsChanged announces that userID's pending suspended transactions
// changed.
func (p *ApprovalsChangePublisher) TransactionsChanged(ctx context.Context, userID string) {
	if err := p.publish(ctx, userID, models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedTransactions}); err != nil {
		p.logger.Warn("approvals changed event not delivered",
			"subject", models.ApprovalsChangedTransactions, "user_id", userID, "error", err)
	}
}

// EnrollmentsChanged announces that the pending platform enrollment requests
// changed. Only the platform owner reviews them, so the event goes to the
// owner's sessions.
func (p *ApprovalsChangePublisher) EnrollmentsChanged(ctx context.Context) {
	p.announceEnrollments(ctx, models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedEnrollments})
}

// EnrollmentRequested announces that a request became pending. When the
// worker that created it was launched by `operator deploy`, deploymentID keys
// the event to that launch so the deploying CLI learns requestID from the
// event instead of polling the worker's progress file.
func (p *ApprovalsChangePublisher) EnrollmentRequested(ctx context.Context, requestID, deploymentID string) {
	p.announceEnrollments(ctx, models.ApprovalsChangedPayload{
		Subject:      models.ApprovalsChangedEnrollments,
		DeploymentID: deploymentID,
		RequestID:    requestID,
	})
}

func (p *ApprovalsChangePublisher) announceEnrollments(ctx context.Context, payload models.ApprovalsChangedPayload) {
	ownerID, err := p.userSvc.FirstUserID(ctx)
	if err != nil || ownerID == "" {
		if err != nil {
			p.logger.Warn("approvals changed event not delivered: resolve owner",
				"subject", models.ApprovalsChangedEnrollments, "error", err)
		}
		return
	}
	if err := p.publish(ctx, ownerID, payload); err != nil {
		p.logger.Warn("approvals changed event not delivered",
			"subject", models.ApprovalsChangedEnrollments, "user_id", ownerID, "error", err)
	}
}

// EnrollmentsDecided wakes token-authenticated status requests after the
// governed decision commits, then updates the owner's existing SSE views.
// The internal invalidation uses the registered approvals event; no enrollment
// token or request details enter pub/sub.
func (p *ApprovalsChangePublisher) EnrollmentsDecided(ctx context.Context) {
	p.EnrollmentIssuanceSettled()
	p.EnrollmentsChanged(ctx)
}

// EnrollmentIssuanceSettled wakes token-authenticated completions held on
// another completion's issuance lease once that issuance committed, rolled
// back, or was recovered. It uses the same internal invalidation as
// EnrollmentsDecided and changes no owner view, so no SSE is sent.
func (p *ApprovalsChangePublisher) EnrollmentIssuanceSettled() {
	if p.publisher.pubsub != nil {
		p.publisher.pubsub.Publish(string(constants.EventPlatformApprovalsChanged), nil)
	}
}

// publish emits the event to every unexpired web session and active CLI session of userID.
// Sessions are delivered independently; the returned error joins every failure.
func (p *ApprovalsChangePublisher) publish(ctx context.Context, userID string, payload models.ApprovalsChangedPayload) error {
	if userID == "" {
		return nil
	}
	sessionIDs, err := ownerWebSessionIDs(ctx, p.docStore, userID)
	if err != nil {
		return fmt.Errorf("resolve web sessions of %s: %w", userID, err)
	}
	cliIDs, err := ownerConnectedCLISessionIDs(ctx, p.docStore, p.publisher, userID)
	if err != nil {
		p.logger.Warn("resolve cli sessions failed", "user_id", userID, "error", err)
	}
	payload.Timestamp = time.Now().UTC()
	var errs []error
	for _, sessionID := range sessionIDs {
		route := SSERoute{UserID: userID, WebSessionID: sessionID}
		if err := p.publisher.PublishEphemeral(route, string(constants.EventPlatformApprovalsChanged), payload); err != nil {
			errs = append(errs, fmt.Errorf("web session %s: %w", sessionID, err))
		}
	}
	for _, cliID := range cliIDs {
		route := SSERoute{UserID: userID, CLISessionID: cliID}
		if err := p.publisher.PublishEphemeral(route, string(constants.EventPlatformApprovalsChanged), payload); err != nil {
			errs = append(errs, fmt.Errorf("cli session %s: %w", cliID, err))
		}
	}
	return errors.Join(errs...)
}

// ownerConnectedCLISessionIDs returns the ids of userID's CLI sessions that
// have an SSE stream open, are active and have not expired. A session with no
// open stream cannot consume a live event, and enrollment creates one CLI
// session per worker, so addressing every session on record would make each
// status or approvals event cost the size of the fleet. Candidates are the
// connected streams, so the lookup is one point read per stream, and a stream
// owned by another user is skipped.
func ownerConnectedCLISessionIDs(ctx context.Context, docStore *DocumentStoreService, publisher *SSEEventPublisher, userID string) ([]string, error) {
	if docStore == nil || publisher == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	var ids []string
	for _, id := range publisher.ConnectedCLISessionIDs() {
		doc, err := docStore.DocGet(ctx, marshaler.CollectionName(constants.CollectionCLISessions), id)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue
		}
		wire, err := json.Marshal(doc.ForWire())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
		}
		var session models.CLISession
		if err := json.Unmarshal(wire, &session); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
		}
		if session.UserID != userID || !session.IsActive || (!session.ExpiresAt.IsZero() && now.After(session.ExpiresAt)) {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// notifyingSuspendedStore decorates the suspended-transaction store so every
// change to the pending set (suspend, approve, delete, expiry sweep) announces
// itself, whichever caller made it. The embedded store is the source of truth;
// notification happens only after the underlying operation succeeded.
type notifyingSuspendedStore struct {
	storage.SuspendedTransactionStore
	notify *ApprovalsChangePublisher
}

// newNotifyingSuspendedStore wraps store so changes are announced through notify.
func newNotifyingSuspendedStore(store storage.SuspendedTransactionStore, notify *ApprovalsChangePublisher) storage.SuspendedTransactionStore {
	return &notifyingSuspendedStore{SuspendedTransactionStore: store, notify: notify}
}

func (s *notifyingSuspendedStore) StoreSuspendedTransaction(ctx context.Context, tx *models.SuspendedTransaction) error {
	if err := s.SuspendedTransactionStore.StoreSuspendedTransaction(ctx, tx); err != nil {
		return err
	}
	s.notify.TransactionsChanged(ctx, tx.UserID)
	return nil
}

func (s *notifyingSuspendedStore) ApproveSuspendedTransaction(ctx context.Context, txHash string, proof models.ApprovalProof) error {
	userID := s.ownerOf(ctx, txHash)
	if err := s.SuspendedTransactionStore.ApproveSuspendedTransaction(ctx, txHash, proof); err != nil {
		return err
	}
	s.notify.TransactionsChanged(ctx, userID)
	return nil
}

func (s *notifyingSuspendedStore) DeleteSuspendedTransaction(ctx context.Context, txHash string) error {
	userID := s.ownerOf(ctx, txHash)
	if err := s.SuspendedTransactionStore.DeleteSuspendedTransaction(ctx, txHash); err != nil {
		return err
	}
	s.notify.TransactionsChanged(ctx, userID)
	return nil
}

func (s *notifyingSuspendedStore) CleanupExpiredSuspendedTransactions(ctx context.Context) (int64, error) {
	expired, listErr := s.GetExpiredSuspendedTransactions(ctx)
	deleted, err := s.SuspendedTransactionStore.CleanupExpiredSuspendedTransactions(ctx)
	if err != nil {
		return deleted, err
	}
	if listErr != nil {
		s.notify.logger.Warn("approvals changed: list expired transactions failed", "error", listErr)
	}
	// Expired rows were already hidden from the pending list; the sweep only
	// matters to a console that still shows them, so tell their owners once.
	notified := make(map[string]struct{}, len(expired))
	for _, tx := range expired {
		if _, done := notified[tx.UserID]; done {
			continue
		}
		notified[tx.UserID] = struct{}{}
		s.notify.TransactionsChanged(ctx, tx.UserID)
	}
	return deleted, nil
}

// ownerOf returns the user a suspended transaction belongs to, or "" when it
// cannot be resolved (already gone, expired, or the lookup failed); the
// mutation proceeds regardless and simply announces nothing.
func (s *notifyingSuspendedStore) ownerOf(ctx context.Context, txHash string) string {
	tx, ok, err := s.GetSuspendedTransaction(ctx, txHash)
	if err != nil || !ok || tx == nil {
		return ""
	}
	return tx.UserID
}
