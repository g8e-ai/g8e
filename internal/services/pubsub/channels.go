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

package pubsub

import (
	"fmt"

	"github.com/g8e-ai/g8e/internal/constants"
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
