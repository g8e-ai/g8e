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

package gateway

import (
	"github.com/g8e-ai/g8e/internal/services/governance"
)

// SetEnvelopeProcessor wires the synchronous envelope-processing pipeline
// into the governance controller. It must be called after the gateway service
// has been constructed and before BYO clients submit transactions to
// /api/v1/governance/envelopes. Calling with nil disables the endpoint.
func (ls *GatewayModeService) SetEnvelopeProcessor(p governance.EnvelopeProcessor) {
	ls.handler.governanceController.SetEnvelopeProcessor(p)
}
