// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import (
	"errors"
	"fmt"
)

var (
	ErrTxUnknownEventType      = errors.New("TX_UNKNOWN_EVENT: event type not registered")
	ErrTxEventNotRequest       = errors.New("TX_EVENT_NOT_REQUEST: event is not a governed request")
	ErrTxEventNotGoverned      = errors.New("TX_EVENT_NOT_GOVERNED: event is not on the governed transport")
	ErrTxEventActionMismatch   = errors.New("TX_EVENT_ACTION_MISMATCH: event_type and action_type disagree")
	ErrTxOutcomeNotRegistered  = errors.New("TX_OUTCOME_NOT_REGISTERED: outcome event type not registered")
	ErrTxOutcomeNotAllowed     = errors.New("TX_OUTCOME_NOT_ALLOWED: outcome event type not listed for request")
	ErrTxPayloadDecoderMissing = errors.New("TX_PAYLOAD_DECODER_MISSING: no typed payload decoder for governed action")
)

func eventHasTransport(entry EventRegistryEntry, transport string) bool {
	for _, t := range entry.Transport {
		if t == transport {
			return true
		}
	}
	return false
}

// RequestEventForAction returns a canonical governed request event for an action class.
// When multiple request events share an action, the lexicographically smallest wire value wins.
func RequestEventForAction(action ActionType) (EventType, error) {
	var match EventType
	for event, entry := range Registry.byType {
		if entry.Kind != EventKindRequest || entry.GovernanceAction != action {
			continue
		}
		if !eventHasTransport(entry, "governed") {
			continue
		}
		if match == "" || string(event) < string(match) {
			match = event
		}
	}
	if match == "" {
		return "", fmt.Errorf("%w: %q", ErrTxUnknownActionType, action)
	}
	return match, nil
}

// ValidateGovernedRequest resolves the governed action class for a request event.
func ValidateGovernedRequest(event EventType) (ActionType, error) {
	entry, ok := Registry.Lookup(event)
	if !ok {
		return "", ErrTxUnknownEventType
	}
	if entry.Kind != EventKindRequest {
		return "", fmt.Errorf("%w: %q", ErrTxEventNotRequest, event)
	}
	if !eventHasTransport(entry, "governed") {
		return "", fmt.Errorf("%w: %q", ErrTxEventNotGoverned, event)
	}
	if entry.GovernanceAction == "" {
		return "", fmt.Errorf("%w: %q has no governance action", ErrTxEventNotGoverned, event)
	}
	return entry.GovernanceAction, nil
}

// ValidateGovernedEnvelopeFields ensures a governed envelope's event and action agree.
func ValidateGovernedEnvelopeFields(event EventType, action ActionType) error {
	expected, err := ValidateGovernedRequest(event)
	if err != nil {
		return err
	}
	if action != expected {
		return fmt.Errorf("%w: event %q expects %q, got %q", ErrTxEventActionMismatch, event, expected, action)
	}
	return nil
}

// ValidateGovernedResultEnvelope ensures a correlated result envelope carries the
// originating request's action class and a registered outcome for that request.
func ValidateGovernedResultEnvelope(requestEvent, outcomeEvent EventType, action ActionType) error {
	expectedAction, err := ValidateGovernedRequest(requestEvent)
	if err != nil {
		return err
	}
	if action != expectedAction {
		return fmt.Errorf("%w: request %q expects %q, got %q", ErrTxEventActionMismatch, requestEvent, expectedAction, action)
	}

	outcomeEntry, ok := Registry.Lookup(outcomeEvent)
	if !ok {
		return fmt.Errorf("%w: %q", ErrTxOutcomeNotRegistered, outcomeEvent)
	}
	switch outcomeEntry.Kind {
	case EventKindOutcome, EventKindFact, EventKindStream:
	default:
		return fmt.Errorf("%w: %q is kind %q", ErrTxOutcomeNotAllowed, outcomeEvent, outcomeEntry.Kind)
	}

	requestEntry, ok := Registry.Lookup(requestEvent)
	if !ok {
		return fmt.Errorf("%w: %q", ErrTxUnknownEventType, requestEvent)
	}
	if len(requestEntry.Outcomes) == 0 {
		return nil
	}
	for _, allowed := range requestEntry.Outcomes {
		if allowed == outcomeEntry.Key {
			return nil
		}
	}
	return fmt.Errorf("%w: outcome %q not listed for request %q", ErrTxOutcomeNotAllowed, outcomeEvent, requestEvent)
}
