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

import "fmt"

// Channel naming convention (shared across client, agent, g8eo):
// Canonical values defined in protocol/constants/channels.json (the source of truth).
// Agent and client are also consumers of that same JSON file.
//
//	cmd:{operator_id}:{operator_session_id}       Agent -> Operator
//	results:{operator_id}:{operator_session_id}    Operator -> Agent
//	heartbeat:{operator_id}:{operator_session_id}  Operator -> Agent

// CmdChannel returns the command channel for an g8e.
func CmdChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("cmd:%s:%s", operatorID, operatorSessionID)
}

// ResultsChannel returns the results channel for an g8e.
func ResultsChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("results:%s:%s", operatorID, operatorSessionID)
}

// HeartbeatChannel returns the heartbeat channel for an g8e.
func HeartbeatChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("heartbeat:%s:%s", operatorID, operatorSessionID)
}

// PubSub wire protocol action strings (used in PubSubMessage.Action).
const (
	PubSubActionSubscribe   = "subscribe"
	PubSubActionPSubscribe  = "psubscribe"
	PubSubActionUnsubscribe = "unsubscribe"
	PubSubActionPublish     = "publish"
)

// PubSub wire protocol event type strings (used in PubSubEvent.Type).
const (
	PubSubEventMessage    = "message"
	PubSubEventPMessage   = "pmessage"
	PubSubEventSubscribed = "subscribed"
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

// StorageDocumentChannel returns the document storage channel for an operator.
func StorageDocumentChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", ChannelStorageDocument, operatorID, operatorSessionID)
}

// StorageKvChannel returns the KV storage channel for an operator.
func StorageKvChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", ChannelStorageKv, operatorID, operatorSessionID)
}

// StorageBlobChannel returns the blob storage channel for an operator.
func StorageBlobChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", ChannelStorageBlob, operatorID, operatorSessionID)
}

// GovernanceChannel returns the governance channel for envelope submission.
func GovernanceChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", ChannelGovernance, operatorID, operatorSessionID)
}

// OperatorIntentChannel returns the intent management channel for an operator.
func OperatorIntentChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", ChannelOperatorIntent, operatorID, operatorSessionID)
}

// OperatorDeviceChannel returns the device management channel for an operator.
func OperatorDeviceChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", ChannelOperatorDevice, operatorID, operatorSessionID)
}

// SseEventChannel returns the SSE event push channel.
func SseEventChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", ChannelSseEvent, operatorID, operatorSessionID)
}
