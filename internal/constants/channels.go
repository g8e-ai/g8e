// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package constants

// Channel naming convention:
//
//	cmd:{operator_id}:{operator_session_id}        Agent -> Operator
//	results:{operator_id}:{operator_session_id}    Operator -> Agent
//	heartbeat:{operator_id}:{operator_session_id}  Operator -> Agent
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
)

// SSE event type strings (used in SSE event framing and consumer dispatch).
const (
	SSEEventTypeApprovalCompleted = "approval.completed"
)
