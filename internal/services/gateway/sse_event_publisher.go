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

	"github.com/g8e-ai/g8e/v2/internal/models"
)

// SSEEventPublisher is the Gateway's in-process producer path for SSE events:
// it appends a durable row to the SSE event store and then publishes the live
// event to the pub/sub channel of the route. The stored payload matches the
// SSEPushPayload wire shape so consumers parse it identically whether it
// arrived through the HTTP push endpoint or an in-process producer.
type SSEEventPublisher struct {
	sseStore *SSEEventService
	pubsub   *GatewayWebSocketHandler
}

// NewSSEEventPublisher creates a publisher over the given event store and
// pub/sub handler. A nil pubsub persists the row without live delivery.
func NewSSEEventPublisher(sseStore *SSEEventService, pubsub *GatewayWebSocketHandler) *SSEEventPublisher {
	return &SSEEventPublisher{sseStore: sseStore, pubsub: pubsub}
}

// Publish constructs the nested SSE event envelope for eventType, appends it to
// the SSE event store attributed to producerID, and publishes the live event to
// the channel the SSE stream handler subscribes for route. The row is appended
// before the live publish so a replaying consumer never misses an event it was
// shown live.
func (p *SSEEventPublisher) Publish(route SSERoute, eventType string, payload any, producerID string) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}
	envelope := sseEventEnvelope{
		Type: eventType,
		Data: dataBytes,
	}
	eventBytes, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal event envelope: %w", err)
	}
	pushPayload := models.SSEPushPayload{
		UserID: route.UserID,
		Event:  eventBytes,
	}
	if route.WebSessionID != "" {
		pushPayload.WebSessionID = route.WebSessionID
	} else {
		pushPayload.CliSessionID = route.CLISessionID
	}
	payloadBytes, err := json.Marshal(pushPayload)
	if err != nil {
		return fmt.Errorf("marshal push payload: %w", err)
	}

	rowID, err := p.sseStore.SSEEventsAppend(route, eventType, string(payloadBytes), producerID)
	if err != nil {
		return fmt.Errorf("append sse event: %w", err)
	}

	// The channel matches the SSE stream handler's subscription channel.
	var channel string
	switch {
	case route.CLISessionID != "":
		channel = "sse:cli:" + route.CLISessionID
	case route.WebSessionID != "":
		channel = "sse:web:" + route.WebSessionID
	}
	if channel != "" && p.pubsub != nil {
		pubEvent := models.SSEPublishedEvent{ID: rowID, Payload: json.RawMessage(payloadBytes)}
		envelopeJSON, err := json.Marshal(pubEvent)
		if err != nil {
			return fmt.Errorf("marshal published event: %w", err)
		}
		p.pubsub.Publish(channel, envelopeJSON)
	}
	return nil
}
