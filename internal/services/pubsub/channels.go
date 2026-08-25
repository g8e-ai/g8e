// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// CmdChannel returns the command channel for an operator session.
func CmdChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", constants.ChannelPrefixCmd, operatorID, operatorSessionID)
}

// ResultsChannel returns the results channel for an operator session.
func ResultsChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", constants.ChannelPrefixResults, operatorID, operatorSessionID)
}

// HeartbeatChannel returns the heartbeat channel for an operator session.
func HeartbeatChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", constants.ChannelPrefixHeartbeat, operatorID, operatorSessionID)
}

// ReceiptsChannel returns the receipts channel for an operator session.
// The operator publishes signed ActionReceipt envelopes to this channel
// after execution; the gateway intercepts, verifies, records, and fans out.
func ReceiptsChannel(operatorID, operatorSessionID string) string {
	return fmt.Sprintf("%s:%s:%s", constants.ChannelPrefixReceipts, operatorID, operatorSessionID)
}
