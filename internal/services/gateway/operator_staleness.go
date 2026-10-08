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
	"fmt"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// reconcileOperatorStaleness moves each of docs that is a remote Operator
// silent for longer than its constants.OperatorStaleAfter window (derived from
// the heartbeat interval it declared at session start) from active to stale,
// persisting the transition. It reports whether any of docs was stale: the
// caller's read then no longer reflects the store and must be repeated before
// it is returned. It is a no-op for every collection except operators.
//
// The registry otherwise holds an Operator `active` until it is explicitly
// terminated, so a killed process, crashed host, or stopped container would
// stay selectable forever. Running the check on every document a read returns
// means no reader (registration, auth, SSE, the data API, enrollment,
// selection) can observe a stale-but-active document. Operators no reader
// touches move when their staleness deadline timer fires
// (operator_staleness_watch.go).
//
// Every transition the reconciler applies is reported to the bound
// OperatorStatusObserver after it is persisted, so the dashboard hears about it
// whichever reader or staleness deadline timer happened to apply it.
//
// The embedded Operator is exempt: it is the Gateway's own in-process
// substrate and is live exactly when the Gateway is.
func (s *DocumentStoreService) reconcileOperatorStaleness(ctx context.Context, docs ...*models.Document) (bool, error) {
	if len(docs) == 0 || docs[0].Collection != marshaler.CollectionName(constants.CollectionOperators) {
		return false, nil
	}

	now := time.Now().UTC()
	found := false
	for _, doc := range docs {
		stale, op, err := operatorHeartbeatStale(doc, now)
		if err != nil {
			return false, fmt.Errorf("%w: operator %s: %w", constants.ErrOperatorStalenessReconcile, doc.ID, err)
		}
		if !stale {
			continue
		}
		found = true
		// Conditional on the status still being active so a concurrent
		// heartbeat recovery, stop, or termination is never overwritten.
		applied, err := s.DocConditionalUpdate(
			ctx, doc.Collection, doc.ID,
			json.RawMessage(fmt.Sprintf(`{"status":%q}`, constants.OperatorStatusStale)),
			"status", string(constants.OperatorStatusActive),
		)
		if err != nil {
			return false, fmt.Errorf("%w: operator %s: %w", constants.ErrOperatorStalenessReconcile, doc.ID, err)
		}
		if applied {
			s.logger.Warn("Operator heartbeat stale; marked stale",
				"operator_id", doc.ID,
				"stale_after", operatorStaleAfter(op))
			s.NotifyOperatorStatusChanged(ctx, OperatorStatusTransition{
				OperatorID: doc.ID,
				UserID:     op.GetUserId(),
				Name:       op.GetName(),
				Status:     constants.OperatorStatusStale,
			})
		}
	}
	return found, nil
}

// activeRemoteOperatorFilters selects the Operators that can go stale: remote
// Operators currently marked active.
func activeRemoteOperatorFilters() []models.DocFilter {
	return []models.DocFilter{
		{Field: "status", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorStatusActive))},
		{Field: "operator_type", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorTypeRemote))},
	}
}

// operatorHeartbeatStale reports whether doc is an active remote Operator whose
// last sign of life is older than constants.OperatorStaleAfter for the
// heartbeat interval the Operator declared at session start. It also returns
// the parsed operator document so the caller can report the transition without
// re-reading the document.
func operatorHeartbeatStale(doc *models.Document, now time.Time) (bool, *operatorv1.OperatorDocument, error) {
	deadline, op, err := operatorStaleDeadline(doc)
	if err != nil {
		return false, nil, err
	}
	return !deadline.IsZero() && now.After(deadline), op, nil
}

// storedActiveRemoteOperator reports from doc's stored status and type fields
// whether it can be an active remote Operator, so only those documents pay for
// a full decode. A field that does not decode as a string is left to the full
// decode to reject.
func storedActiveRemoteOperator(doc *models.Document) bool {
	var status, operatorType string
	if raw, ok := doc.Data["status"]; ok && json.Unmarshal(raw, &status) != nil {
		return true
	}
	if raw, ok := doc.Data["operator_type"]; ok && json.Unmarshal(raw, &operatorType) != nil {
		return true
	}
	return constants.OperatorStatus(status) == constants.OperatorStatusActive && constants.OperatorType(operatorType) == constants.OperatorTypeRemote
}

// operatorStaleDeadline returns the instant after which doc, an active remote
// Operator, is stale if it stays silent: its last sign of life plus
// constants.OperatorStaleAfter for the heartbeat interval it declared at
// session start. The last sign of life is the Gateway-stamped
// last_heartbeat_at, falling back to claimed_at and then to the document's
// created_at for an Operator that has not heartbeated yet. The deadline is zero
// for any other Operator, which can never go stale, and the operator document
// is then nil when the stored status or type alone rules doc out.
func operatorStaleDeadline(doc *models.Document) (time.Time, *operatorv1.OperatorDocument, error) {
	if !storedActiveRemoteOperator(doc) {
		return time.Time{}, nil, nil
	}
	op, err := models.OperatorDocumentFromStore(doc)
	if err != nil {
		return time.Time{}, nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
	}
	if constants.OperatorStatus(op.GetStatus()) != constants.OperatorStatusActive || constants.OperatorType(op.GetOperatorType()) != constants.OperatorTypeRemote {
		return time.Time{}, op, nil
	}

	lastSeen := doc.CreatedAt
	if claimedAt := op.GetClaimedAt(); claimedAt != nil {
		lastSeen = claimedAt.AsTime()
	}
	if lastHeartbeatAt := op.GetLastHeartbeatAt(); lastHeartbeatAt != nil {
		lastSeen = lastHeartbeatAt.AsTime()
	}
	return lastSeen.Add(operatorStaleAfter(op)), op, nil
}

// operatorStaleAfter is op's constants.OperatorStaleAfter window for the
// heartbeat interval it declared at session start.
func operatorStaleAfter(op *operatorv1.OperatorDocument) time.Duration {
	declared := time.Duration(op.GetRuntimeConfig().GetHeartbeatIntervalMs()) * time.Millisecond
	return constants.OperatorStaleAfter(declared)
}
