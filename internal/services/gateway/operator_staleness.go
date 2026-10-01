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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// operatorHeartbeatLiveness is the subset of an Operator document the
// staleness check reads.
type operatorHeartbeatLiveness struct {
	Status          constants.OperatorStatus `json:"status"`
	OperatorType    constants.OperatorType   `json:"operator_type"`
	ClaimedAt       *time.Time               `json:"claimed_at"`
	LastHeartbeatAt *time.Time               `json:"last_heartbeat_at"`
}

// reconcileOperatorStaleness moves every remote Operator document that has been
// silent for longer than constants.OperatorHeartbeatStaleAfter from active to
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
		stale, err := operatorHeartbeatStale(doc, now)
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
				"stale_after", constants.OperatorHeartbeatStaleAfter)
		}
	}
	return nil
}

// operatorHeartbeatStale reports whether doc is an active remote Operator whose
// last sign of life is older than constants.OperatorHeartbeatStaleAfter. The
// last sign of life is the Gateway-stamped last_heartbeat_at, falling back to
// claimed_at and then to the document's created_at for an Operator that has not
// heartbeated yet.
func operatorHeartbeatStale(doc *models.Document, now time.Time) (bool, error) {
	wire, err := json.Marshal(doc.ForWire())
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
	}
	var op operatorHeartbeatLiveness
	if err := json.Unmarshal(wire, &op); err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
	}
	if op.Status != constants.OperatorStatusActive || op.OperatorType != constants.OperatorTypeRemote {
		return false, nil
	}

	lastSeen := doc.CreatedAt
	if op.ClaimedAt != nil {
		lastSeen = *op.ClaimedAt
	}
	if op.LastHeartbeatAt != nil {
		lastSeen = *op.LastHeartbeatAt
	}
	return now.Sub(lastSeen) > constants.OperatorHeartbeatStaleAfter, nil
}
