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
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// reconcileOperatorStaleness moves every remote Operator document that has been
// silent for longer than its constants.OperatorStaleAfter window (derived from
// the heartbeat interval it declared at session start) from active to
// stale, persisting the transition before the caller reads. It is a no-op for
// every collection except operators. An empty id reconciles the whole
// collection; a non-empty id reconciles that one document.
//
// The registry otherwise holds an Operator `active` until it is explicitly
// terminated, so a killed process, crashed host, or stopped container would
// stay selectable forever. Running the check inside the document store means no
// reader (registration, auth, SSE, the data API, enrollment, selection) can
// observe a stale-but-active document.
//
// Every transition the reconciler applies is reported to the bound
// OperatorStatusObserver after it is persisted, so the dashboard hears about it
// whichever reader or sweep happened to apply it.
//
// The embedded Operator is exempt: it is the Gateway's own in-process
// substrate and is live exactly when the Gateway is.
func (s *DocumentStoreService) reconcileOperatorStaleness(collection, id string) error {
	if collection != marshaler.CollectionName(constants.CollectionOperators) {
		return nil
	}

	var candidates []*models.Document
	if id != "" {
		doc, err := s.docGet(collection, id)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrOperatorStalenessReconcile, err)
		}
		if doc != nil {
			candidates = append(candidates, doc)
		}
	} else {
		docs, err := s.docQuery(collection, []models.DocFilter{
			{Field: "status", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorStatusActive))},
			{Field: "operator_type", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorTypeRemote))},
		}, "", 0)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrOperatorStalenessReconcile, err)
		}
		candidates = docs
	}

	now := time.Now().UTC()
	for _, doc := range candidates {
		stale, op, err := operatorHeartbeatStale(doc, now)
		if err != nil {
			return fmt.Errorf("%w: operator %s: %w", constants.ErrOperatorStalenessReconcile, doc.ID, err)
		}
		if !stale {
			continue
		}
		// Conditional on the status still being active so a concurrent
		// heartbeat recovery, stop, or termination is never overwritten.
		applied, err := s.DocConditionalUpdate(
			collection, doc.ID,
			json.RawMessage(fmt.Sprintf(`{"status":%q}`, constants.OperatorStatusStale)),
			"status", string(constants.OperatorStatusActive),
		)
		if err != nil {
			return fmt.Errorf("%w: operator %s: %w", constants.ErrOperatorStalenessReconcile, doc.ID, err)
		}
		if applied {
			s.logger.Warn("Operator heartbeat stale; marked stale",
				"operator_id", doc.ID,
				"stale_after", constants.OperatorStaleAfter(time.Duration(op.GetRuntimeConfig().GetHeartbeatIntervalMs())*time.Millisecond))
			s.NotifyOperatorStatusChanged(OperatorStatusTransition{
				OperatorID: doc.ID,
				UserID:     op.GetUserId(),
				Name:       op.GetName(),
				Status:     constants.OperatorStatusStale,
			})
		}
	}
	return nil
}

// ReconcileOperatorStaleness reconciles the whole Operator registry, moving
// every silent remote Operator to stale and reporting each transition to the
// bound observer. The sweeper calls it so a transition is pushed without
// waiting for a reader.
func (s *DocumentStoreService) ReconcileOperatorStaleness() error {
	return s.reconcileOperatorStaleness(marshaler.CollectionName(constants.CollectionOperators), "")
}

// operatorHeartbeatStale reports whether doc is an active remote Operator whose
// last sign of life is older than constants.OperatorStaleAfter for the
// heartbeat interval the Operator declared at session start. The
// last sign of life is the Gateway-stamped last_heartbeat_at, falling back to
// claimed_at and then to the document's created_at for an Operator that has not
// heartbeated yet. It also returns the parsed operator document so the caller can
// report the transition without re-reading the document.
func operatorHeartbeatStale(doc *models.Document, now time.Time) (bool, *operatorv1.OperatorDocument, error) {
	op, err := models.OperatorDocumentFromStore(doc)
	if err != nil {
		return false, nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
	}
	if constants.OperatorStatus(op.GetStatus()) != constants.OperatorStatusActive || constants.OperatorType(op.GetOperatorType()) != constants.OperatorTypeRemote {
		return false, op, nil
	}

	lastSeen := doc.CreatedAt
	if claimedAt := op.GetClaimedAt(); claimedAt != nil {
		lastSeen = claimedAt.AsTime()
	}
	if lastHeartbeatAt := op.GetLastHeartbeatAt(); lastHeartbeatAt != nil {
		lastSeen = lastHeartbeatAt.AsTime()
	}
	declared := time.Duration(op.GetRuntimeConfig().GetHeartbeatIntervalMs()) * time.Millisecond
	return now.Sub(lastSeen) > constants.OperatorStaleAfter(declared), op, nil
}
