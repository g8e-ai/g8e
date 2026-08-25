// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// Channel naming convention:
//
//	cmd:{operator_id}:{operator_session_id}        Agent -> Operator
//	results:{operator_id}:{operator_session_id}    Operator -> Agent
//	heartbeat:{operator_id}:{operator_session_id}  Operator -> Agent
//	receipts:{operator_id}:{operator_session_id}   Operator -> Gateway (intercepted)
//
// Constructors live in internal/services/pubsub.
// Reference values in protocol/constants/channels.json for external consumers.

// PubSub wire protocol action strings (used in PubSubMessage.Action).
const (
	PubSubActionSubscribe   = "subscribe"
	PubSubActionPSubscribe  = "psubscribe"
	PubSubActionUnsubscribe = "unsubscribe"
	PubSubActionPublish     = "publish"
)

// PubSub wire protocol event type strings (used in PubSubEvent.Type).
const (
	PubSubEventMessage      = "message"
	PubSubEventPMessage     = "pmessage"
	PubSubEventSubscribed   = "subscribed"
	PubSubEventUnsubscribed = "unsubscribed"
	PubSubEventError        = "error"
)

// Storage and governance channel prefixes (for pubsub-based data operations).
const (
	ChannelStorageDocument = "storage_document"
	ChannelStorageKv       = "storage_kv"
	ChannelStorageBlob     = "storage_blob"
	ChannelGovernance      = "governance"
	ChannelOperatorIntent  = "operator_intent"
	ChannelOperatorDevice  = "operator_device"
	ChannelSseEvent        = "sse_event"
)

// Session channel prefixes (for operator session communication).
const (
	ChannelPrefixCmd       = "cmd"
	ChannelPrefixResults   = "results"
	ChannelPrefixHeartbeat = "heartbeat"
	ChannelPrefixReceipts  = "receipts"
)

// SSE event type strings (used in SSE event framing and consumer dispatch).
const (
	SSEEventTypeApprovalCompleted = "approval.completed"
)
