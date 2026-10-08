// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// operatorStalenessWatcher holds one deadline timer per active remote Operator.
// Each heartbeat moves the Operator's timer to its next stale deadline, so a
// timer fires only when an Operator has actually gone silent; nothing scans the
// registry. The watcher is inert until WatchOperatorStaleness starts it, as in
// unit tests that build a bare document store.
type operatorStalenessWatcher struct {
	mu       sync.Mutex
	watching bool
	// ctx is the WatchOperatorStaleness lifetime. Timer-driven reads use it,
	// since a deadline timer has no request to inherit a context from.
	ctx    context.Context
	timers map[string]*operatorStaleTimer
}

// operatorStaleTimer is one Operator's deadline timer and the silence window,
// read from its document when the timer was armed, that a heartbeat restarts.
type operatorStaleTimer struct {
	timer      *time.Timer
	staleAfter time.Duration
}

// WatchOperatorStaleness starts the deadline timers: it arms one per active
// remote Operator already in the registry (an Operator already past its
// deadline fires immediately) and stops every timer when ctx is cancelled.
// From here on, the Operator status transitions reported through
// NotifyOperatorStatusChanged and each RearmOperatorStaleness call keep the
// timers current.
func (s *DocumentStoreService) WatchOperatorStaleness(ctx context.Context) error {
	w := &s.staleness
	w.mu.Lock()
	w.watching = true
	w.ctx = ctx
	w.timers = make(map[string]*operatorStaleTimer)
	w.mu.Unlock()

	go func() {
		<-ctx.Done()
		w.mu.Lock()
		defer w.mu.Unlock()
		w.watching = false
		for id, t := range w.timers {
			t.timer.Stop()
			delete(w.timers, id)
		}
	}()

	docs, err := s.docQuery(ctx, marshaler.CollectionName(constants.CollectionOperators), activeRemoteOperatorFilters(), "", 0)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrOperatorStalenessReconcile, err)
	}
	for _, doc := range docs {
		if err := s.armOperatorStaleness(doc); err != nil {
			return fmt.Errorf("%w: operator %s: %w", constants.ErrOperatorStalenessReconcile, doc.ID, err)
		}
	}
	return nil
}

// ExtendOperatorStaleness moves operatorID's armed deadline timer to the stale
// deadline that follows a heartbeat recorded at heartbeatAt. The heartbeat path
// calls it after recording a heartbeat. An armed timer already holds the
// Operator's silence window, so this reads nothing; an Operator without one is
// armed from its document. A timer that fires re-reads the persisted deadline
// before acting, so a window that changed since arming is corrected then.
func (s *DocumentStoreService) ExtendOperatorStaleness(ctx context.Context, operatorID string, heartbeatAt time.Time) {
	w := &s.staleness
	w.mu.Lock()
	t, ok := w.timers[operatorID]
	if ok {
		t.timer.Reset(max(time.Until(heartbeatAt.Add(t.staleAfter)), 0))
	}
	w.mu.Unlock()
	if !ok {
		s.RearmOperatorStaleness(ctx, operatorID)
	}
}

// RearmOperatorStaleness sets operatorID's deadline timer from its persisted
// document. ctx bounds that read. A failure only costs the timely push: the
// next read still reconciles the Operator.
func (s *DocumentStoreService) RearmOperatorStaleness(ctx context.Context, operatorID string) {
	s.staleness.mu.Lock()
	watching := s.staleness.watching
	s.staleness.mu.Unlock()
	if !watching {
		return
	}
	doc, err := s.docGet(ctx, marshaler.CollectionName(constants.CollectionOperators), operatorID)
	if err == nil && doc != nil {
		err = s.armOperatorStaleness(doc)
	}
	if err != nil {
		s.logger.Warn("Operator staleness timer not rearmed", "operator_id", operatorID, "error", err)
	}
}

// trackOperatorStaleness keeps the deadline timer in step with a reported
// status transition: an Operator that became active gets a timer, any other
// status (stale, stopped, terminated) cancels it.
func (s *DocumentStoreService) trackOperatorStaleness(ctx context.Context, t OperatorStatusTransition) {
	if t.Status != constants.OperatorStatusActive {
		s.disarmOperatorStaleness(t.OperatorID)
		return
	}
	s.RearmOperatorStaleness(ctx, t.OperatorID)
}

// armOperatorStaleness sets doc's timer to its stale deadline, or cancels it
// when doc is not an active remote Operator.
func (s *DocumentStoreService) armOperatorStaleness(doc *models.Document) error {
	deadline, op, err := operatorStaleDeadline(doc)
	if err != nil {
		return err
	}
	if deadline.IsZero() {
		s.disarmOperatorStaleness(doc.ID)
		return nil
	}

	w := &s.staleness
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.watching {
		return nil
	}
	delay := max(time.Until(deadline), 0)
	if t, ok := w.timers[doc.ID]; ok {
		t.staleAfter = operatorStaleAfter(op)
		t.timer.Reset(delay)
		return nil
	}
	operatorID := doc.ID
	ctx := w.ctx
	w.timers[operatorID] = &operatorStaleTimer{
		timer:      time.AfterFunc(delay, func() { s.operatorWentSilent(ctx, operatorID) }),
		staleAfter: operatorStaleAfter(op),
	}
	return nil
}

// disarmOperatorStaleness cancels operatorID's deadline timer.
func (s *DocumentStoreService) disarmOperatorStaleness(operatorID string) {
	w := &s.staleness
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.timers[operatorID]; ok {
		t.timer.Stop()
		delete(w.timers, operatorID)
	}
}

// operatorWentSilent runs when operatorID's deadline timer fires. The
// reconciling read re-checks the persisted deadline, so a heartbeat that landed
// in the meantime keeps the Operator active; a real transition is persisted and
// reported to the status observer, which also cancels the timer. An Operator
// that is still active afterwards (the timer fired a hair before the persisted
// deadline) has its timer re-armed from that same read, so it can never be left
// without one. A deleted Operator's timer is dropped.
func (s *DocumentStoreService) operatorWentSilent(ctx context.Context, operatorID string) {
	doc, err := s.DocGet(ctx, marshaler.CollectionName(constants.CollectionOperators), operatorID)
	if err != nil {
		s.logger.Warn("Operator staleness reconcile failed", "operator_id", operatorID, "error", err)
		return
	}
	if doc == nil {
		s.disarmOperatorStaleness(operatorID)
		return
	}
	if err := s.armOperatorStaleness(doc); err != nil {
		s.logger.Warn("Operator staleness timer not rearmed", "operator_id", operatorID, "error", err)
	}
}
