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
func (p *ApprovalsChangePublisher) TransactionsChanged(userID string) {
	if err := p.publish(userID, models.ApprovalsChangedTransactions); err != nil {
		p.logger.Warn("approvals changed event not delivered",
			"subject", models.ApprovalsChangedTransactions, "user_id", userID, "error", err)
	}
}

// EnrollmentsChanged announces that the pending platform enrollment requests
// changed. Only the platform owner reviews them, so the event goes to the
// owner's sessions.
func (p *ApprovalsChangePublisher) EnrollmentsChanged() {
	ownerID, err := p.userSvc.FirstUserID()
	if err != nil || ownerID == "" {
		if err != nil {
			p.logger.Warn("approvals changed event not delivered: resolve owner",
				"subject", models.ApprovalsChangedEnrollments, "error", err)
		}
		return
	}
	if err := p.publish(ownerID, models.ApprovalsChangedEnrollments); err != nil {
		p.logger.Warn("approvals changed event not delivered",
			"subject", models.ApprovalsChangedEnrollments, "user_id", ownerID, "error", err)
	}
}

// EnrollmentsDecided wakes token-authenticated status requests after the
// governed decision commits, then updates the owner's existing SSE views.
// The internal invalidation uses the registered approvals event; no enrollment
// token or request details enter pub/sub.
func (p *ApprovalsChangePublisher) EnrollmentsDecided() {
	if p.publisher.pubsub != nil {
		p.publisher.pubsub.Publish(string(constants.EventPlatformApprovalsChanged), nil)
	}
	p.EnrollmentsChanged()
}

// publish emits the event to every unexpired web session and active CLI session of userID.
// Sessions are delivered independently; the returned error joins every failure.
func (p *ApprovalsChangePublisher) publish(userID string, subject models.ApprovalsChangedSubject) error {
	if userID == "" {
		return nil
	}
	sessionIDs, err := ownerWebSessionIDs(p.docStore, userID)
	if err != nil {
		return fmt.Errorf("resolve web sessions of %s: %w", userID, err)
	}
	cliIDs, err := ownerCLISessionIDs(p.docStore, userID)
	if err != nil {
		p.logger.Warn("resolve cli sessions failed", "user_id", userID, "error", err)
	}
	payload := models.ApprovalsChangedPayload{Subject: subject, Timestamp: time.Now().UTC()}
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

// ownerCLISessionIDs returns the ids of userID's CLI sessions that have not
// expired and are not terminated.
func ownerCLISessionIDs(docStore *DocumentStoreService, userID string) ([]string, error) {
	if docStore == nil {
		return nil, nil
	}
	docs, err := docStore.DocQuery(marshaler.CollectionName(constants.CollectionCLISessions), []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
	}, "", 0)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		wire, err := json.Marshal(doc.ForWire())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
		}
		var session models.CLISession
		if err := json.Unmarshal(wire, &session); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
		}
		if !session.IsActive || (!session.ExpiresAt.IsZero() && now.After(session.ExpiresAt)) {
			continue
		}
		ids = append(ids, doc.ID)
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
	s.notify.TransactionsChanged(tx.UserID)
	return nil
}

func (s *notifyingSuspendedStore) ApproveSuspendedTransaction(ctx context.Context, txHash string, proof models.ApprovalProof) error {
	userID := s.ownerOf(ctx, txHash)
	if err := s.SuspendedTransactionStore.ApproveSuspendedTransaction(ctx, txHash, proof); err != nil {
		return err
	}
	s.notify.TransactionsChanged(userID)
	return nil
}

func (s *notifyingSuspendedStore) DeleteSuspendedTransaction(ctx context.Context, txHash string) error {
	userID := s.ownerOf(ctx, txHash)
	if err := s.SuspendedTransactionStore.DeleteSuspendedTransaction(ctx, txHash); err != nil {
		return err
	}
	s.notify.TransactionsChanged(userID)
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
		s.notify.TransactionsChanged(tx.UserID)
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
