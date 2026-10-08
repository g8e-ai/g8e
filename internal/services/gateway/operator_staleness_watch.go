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
	timers   map[string]*time.Timer
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
	w.timers = make(map[string]*time.Timer)
	w.mu.Unlock()

	go func() {
		<-ctx.Done()
		w.mu.Lock()
		defer w.mu.Unlock()
		w.watching = false
		for id, timer := range w.timers {
			timer.Stop()
			delete(w.timers, id)
		}
	}()

	docs, err := s.docQuery(marshaler.CollectionName(constants.CollectionOperators), activeRemoteOperatorFilters(), "", 0)
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

// RearmOperatorStaleness moves operatorID's deadline timer to its next stale
// deadline. The heartbeat path calls it after recording a heartbeat. A failure
// only costs the timely push: the next read still reconciles the Operator.
func (s *DocumentStoreService) RearmOperatorStaleness(operatorID string) {
	s.staleness.mu.Lock()
	watching := s.staleness.watching
	s.staleness.mu.Unlock()
	if !watching {
		return
	}
	doc, err := s.docGet(marshaler.CollectionName(constants.CollectionOperators), operatorID)
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
func (s *DocumentStoreService) trackOperatorStaleness(t OperatorStatusTransition) {
	if t.Status != constants.OperatorStatusActive {
		s.disarmOperatorStaleness(t.OperatorID)
		return
	}
	s.RearmOperatorStaleness(t.OperatorID)
}

// armOperatorStaleness sets doc's timer to its stale deadline, or cancels it
// when doc is not an active remote Operator.
func (s *DocumentStoreService) armOperatorStaleness(doc *models.Document) error {
	deadline, _, err := operatorStaleDeadline(doc)
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
	if timer, ok := w.timers[doc.ID]; ok {
		timer.Reset(delay)
		return nil
	}
	operatorID := doc.ID
	w.timers[operatorID] = time.AfterFunc(delay, func() { s.operatorWentSilent(operatorID) })
	return nil
}

// disarmOperatorStaleness cancels operatorID's deadline timer.
func (s *DocumentStoreService) disarmOperatorStaleness(operatorID string) {
	w := &s.staleness
	w.mu.Lock()
	defer w.mu.Unlock()
	if timer, ok := w.timers[operatorID]; ok {
		timer.Stop()
		delete(w.timers, operatorID)
	}
}

// operatorWentSilent runs when operatorID's deadline timer fires. Reconciling
// re-checks the persisted deadline, so a heartbeat that landed in the meantime
// keeps the Operator active; a real transition is persisted and reported to the
// status observer, which also cancels the timer. An Operator that is still
// active afterwards (the timer fired a hair before the persisted deadline) has
// its timer re-armed, so it can never be left without one.
func (s *DocumentStoreService) operatorWentSilent(operatorID string) {
	if err := s.reconcileOperatorStaleness(marshaler.CollectionName(constants.CollectionOperators), operatorID); err != nil {
		s.logger.Warn("Operator staleness reconcile failed", "operator_id", operatorID, "error", err)
		return
	}
	s.RearmOperatorStaleness(operatorID)
}
