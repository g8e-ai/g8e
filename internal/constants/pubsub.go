// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// PubSub field constants (actions and events are in channels.go)
const (
	PubSubFieldAction  = "action"
	PubSubFieldChannel = "channel"
	PubSubFieldData    = "data"
	PubSubFieldMessage = "message"
	PubSubFieldPattern = "pattern"
	PubSubFieldType    = "type"
	PubSubFieldSender  = "sender"

	// ReceiptSummaryMaxBytes is the maximum size for receipt summary text (4 KiB)
	ReceiptSummaryMaxBytes = 4096
)
