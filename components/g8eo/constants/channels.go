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

// Channel naming convention (shared across g8ed, g8ee, g8eo):
// Canonical values defined in shared/constants/channels.json (the source of truth).
// g8ee and g8ed are also consumers of that same JSON file.
//
//	cmd:{operator_id}:{operator_session_id}       g8ee -> Operator
//	results:{operator_id}:{operator_session_id}    Operator -> g8ee
//	heartbeat:{operator_id}:{operator_session_id}  Operator -> g8ee

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
