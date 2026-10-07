// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// operatorStatusProducerID is the producer_id stamped into SSE event rows the
// Gateway emits for Operator status transitions.
const operatorStatusProducerID = "g8e-gateway-operator-status"

// operatorStatusEvents maps each Operator status the Gateway pushes to the
// status.updated event that announces the transition into it. Bound and
// unbound transitions are published by g8ee after the session bind and are not
// Gateway-originated, so they are absent. So is offline: it is only ever the
// status a slot is created with (RegistrationService.createSlot), never a
// status anything transitions into, so there is no transition to announce.
var operatorStatusEvents = map[constants.OperatorStatus]constants.EventType{
	constants.OperatorStatusActive:     constants.EventOperatorStatusUpdatedActive,
	constants.OperatorStatusStale:      constants.EventOperatorStatusUpdatedStale,
	constants.OperatorStatusStopped:    constants.EventOperatorStatusUpdatedStopped,
	constants.OperatorStatusTerminated: constants.EventOperatorStatusUpdatedTerminated,
}

// OperatorStatusTransition is an Operator status change the Gateway has
// already persisted. Status is the new status; UserID is the owner whose
// dashboard sessions receive the event.
type OperatorStatusTransition struct {
	OperatorID string
	UserID     string
	Name       string
	Status     constants.OperatorStatus
}

// OperatorStatusTransition builds the transition of operatorID into status from
// the persisted Operator document, for a caller that knows the Operator only by
// id. It returns constants.ErrNotFound when no such Operator exists.
func (s *DocumentStoreService) OperatorStatusTransition(operatorID string, status constants.OperatorStatus) (OperatorStatusTransition, error) {
	doc, err := s.DocGet(marshaler.CollectionName(constants.CollectionOperators), operatorID)
	if err != nil {
		return OperatorStatusTransition{}, err
	}
	if doc == nil {
		return OperatorStatusTransition{}, fmt.Errorf("%w: operator %s", constants.ErrNotFound, operatorID)
	}
	op, err := models.OperatorDocumentFromStore(doc)
	if err != nil {
		return OperatorStatusTransition{}, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
	}
	return OperatorStatusTransition{OperatorID: operatorID, UserID: op.GetUserId(), Name: op.GetName(), Status: status}, nil
}

// OperatorStatusObserver is told about each persisted Operator status
// transition. The operator document stays the source of truth: an observer
// reports the change and must not fail the transition that produced it.
type OperatorStatusObserver interface {
	OperatorStatusChanged(OperatorStatusTransition)
}

// OperatorStatusPublisher pushes Operator status transitions to the owning
// user's dashboard as g8e.v1.operator.status.updated.<state> SSE events.
//
// The events are telemetry, not records: a failed push is logged and never
// fails or rolls back the transition, and a dashboard that missed one
// reconciles against the operator list.
type OperatorStatusPublisher struct {
	docStore  *DocumentStoreService
	publisher *SSEEventPublisher
	logger    *slog.Logger
}

// NewOperatorStatusPublisher creates a publisher that resolves the owner's web
// sessions from docStore and emits through publisher.
func NewOperatorStatusPublisher(docStore *DocumentStoreService, publisher *SSEEventPublisher, logger *slog.Logger) *OperatorStatusPublisher {
	return &OperatorStatusPublisher{docStore: docStore, publisher: publisher, logger: logger}
}

// OperatorStatusChanged implements OperatorStatusObserver. Delivery failures
// are logged because the transition is already persisted.
func (p *OperatorStatusPublisher) OperatorStatusChanged(t OperatorStatusTransition) {
	if err := p.Publish(t); err != nil {
		p.logger.Warn("operator status event not delivered",
			"operator_id", t.OperatorID,
			"status", t.Status,
			"error", err)
	}
}

// Publish emits the status.updated event for t to every unexpired web session
// of the Operator's owner. It returns constants.ErrOperatorStatusEventUnsupported
// when t.Status has no Gateway-published event. Sessions are delivered
// independently, so one failing session does not hide the event from the rest;
// the returned error joins every per-session failure.
func (p *OperatorStatusPublisher) Publish(t OperatorStatusTransition) error {
	eventType, ok := operatorStatusEvents[t.Status]
	if !ok {
		return fmt.Errorf("%w: %q", constants.ErrOperatorStatusEventUnsupported, t.Status)
	}

	sessionIDs, err := ownerWebSessionIDs(p.docStore, t.UserID)
	if err != nil {
		return fmt.Errorf("operator status event: resolve web sessions of %s: %w", t.UserID, err)
	}

	payload := models.OperatorStatusUpdatedPayload{
		OperatorID: t.OperatorID,
		Status:     t.Status,
		Name:       t.Name,
		Timestamp:  time.Now().UTC(),
	}
	var errs []error
	for _, sessionID := range sessionIDs {
		route := SSERoute{UserID: t.UserID, WebSessionID: sessionID}
		if err := p.publisher.Publish(route, string(eventType), payload, operatorStatusProducerID); err != nil {
			errs = append(errs, fmt.Errorf("web session %s: %w", sessionID, err))
		}
	}
	return errors.Join(errs...)
}

// ownerWebSessionIDs returns the ids of userID's web sessions that have not
// expired.
func ownerWebSessionIDs(docStore *DocumentStoreService, userID string) ([]string, error) {
	docs, err := docStore.DocQuery(marshaler.CollectionName(constants.CollectionWebSessions), []models.DocFilter{
		{Field: "user_id", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", userID))},
	}, "", 0)
	if err != nil {
		return nil, err
	}

	nowMs := time.Now().UnixMilli()
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		wire, err := json.Marshal(doc.ForWire())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreMarshalDocument, err)
		}
		var session models.WebSession
		if err := json.Unmarshal(wire, &session); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrDocumentStoreUnmarshalDocument, err)
		}
		if nowMs > session.ExpiresAtUnixMs {
			continue
		}
		ids = append(ids, doc.ID)
	}
	return ids, nil
}
