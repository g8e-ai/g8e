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

	"github.com/g8e-ai/g8e/v2/protocol"
)

var (
	ErrSSEEventNotRegistered  = errors.New("SSE_EVENT_NOT_REGISTERED: event type not in registry")
	ErrSSEEventNotOnTransport = errors.New("SSE_EVENT_NOT_ON_TRANSPORT: event is not on the sse transport")
	ErrSSEProducerNotAllowed  = errors.New("SSE_PRODUCER_NOT_ALLOWED: producer is not registered for this event")
)

// SSEPushValidation is the result of validating an SSE push request.
type SSEPushValidation struct {
	Persist bool
}

func producerListed(entry EventRegistryEntry, producer string) bool {
	for _, p := range entry.Producers {
		if p == producer {
			return true
		}
	}
	return false
}

// SSEProducerFromAppID maps an mTLS app SPIFFE ID to a registry producer name.
func SSEProducerFromAppID(appID string) (string, bool) {
	wid := protocol.NewWorkloadIdentity()
	if wid.IsEnsembleApp(appID) {
		return "ensemble", true
	}
	return "", false
}

// ValidateSSEPush ensures an event is registered for SSE, lists the producer,
// and reports whether the Gateway should persist it to gateway.sse_store.
func ValidateSSEPush(event EventType, producer string) (SSEPushValidation, error) {
	entry, ok := Registry.Lookup(event)
	if !ok {
		return SSEPushValidation{}, fmt.Errorf("%w: %q", ErrSSEEventNotRegistered, event)
	}
	if !eventHasTransport(entry, "sse") {
		return SSEPushValidation{}, fmt.Errorf("%w: %q", ErrSSEEventNotOnTransport, event)
	}
	if producer == "" {
		return SSEPushValidation{}, fmt.Errorf("%w: empty producer for %q", ErrSSEProducerNotAllowed, event)
	}
	if !producerListed(entry, producer) {
		return SSEPushValidation{}, fmt.Errorf("%w: %q cannot produce %q", ErrSSEProducerNotAllowed, producer, event)
	}
	return SSEPushValidation{Persist: entry.Persistence != "ephemeral"}, nil
}
