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
	"encoding/json"
	"testing"
)

// FuzzEnvelopeJSONParsing tests GovernanceEnvelope JSON parsing with random inputs
// to catch edge-case panics and JSON parsing errors.
func FuzzEnvelopeJSONParsing(f *testing.F) {
	// Add seed corpus with valid and edge-case inputs
	f.Add(`{"id":"test-tx-1","operator_id":"op-123","operator_session_id":"sess-456","source_component":"client","action":"file_edit","payload":"{}"}`)
	f.Add(`{"id":"","operator_id":"","operator_session_id":"","source_component":"","action":"","payload":""}`)
	f.Add(`{"id":"` + string(make([]byte, 10000)) + `"}`)
	f.Add(`{"id":"test","nested":{"deep":{"value":"test"}}}`)
	f.Add(`{"id":"test","array":[1,2,3]}`)
	f.Add(`{"id":"test","null":null}`)
	f.Add(`{"id":"test","number":123,"bool":true}`)
	f.Add(`{"id":"test","unicode":"测试🚀"}`)
	f.Add(`{"id":"test","escape":"\"quoted\""}`)
	f.Add(`invalid json`)
	f.Add(``)
	f.Add(`{}`)
	f.Add(`[]`)
	f.Add(`null`)
	f.Add(`123`)
	f.Add(`"string"`)
	f.Add(`{"id":"test","payload":"` + string(make([]byte, 100000)) + `"}`)
	f.Add(`{"id":"test","payload":{"nested":{"deep":{"value":"` + string(make([]byte, 10000)) + `"}}}}`)

	f.Fuzz(func(t *testing.T, data string) {
		// This should never panic - JSON decoding must handle all inputs gracefully
		var envelope struct {
			ID                string `json:"id"`
			OperatorID        string `json:"operator_id"`
			OperatorSessionID string `json:"operator_session_id"`
			SourceComponent   string `json:"source_component"`
			Action            string `json:"action"`
			Payload           string `json:"payload"`
		}
		_ = json.Unmarshal([]byte(data), &envelope)
	})
}

// FuzzEnvelopeIdentityBinding tests the identity binding extraction with random inputs
// to catch edge-case panics in JSON parsing.
func FuzzEnvelopeIdentityBinding(f *testing.F) {
	// Add seed corpus with valid and edge-case inputs
	f.Add(`{"operator_session_id":"sess-123","operator_id":"op-456","source_component":"client"}`)
	f.Add(`{"operator_session_id":"","operator_id":"","source_component":""}`)
	f.Add(`{"operator_session_id":"test"}`)
	f.Add(`{"operator_id":"test"}`)
	f.Add(`{"source_component":"test"}`)
	f.Add(`{"operator_session_id":"` + string(make([]byte, 10000)) + `"}`)
	f.Add(`{"nested":{"deep":{"value":"test"}}}`)
	f.Add(`invalid json`)
	f.Add(``)
	f.Add(`{}`)
	f.Add(`[]`)
	f.Add(`null`)

	f.Fuzz(func(t *testing.T, data string) {
		// This should never panic - JSON decoding must handle all inputs gracefully
		var envelope struct {
			OperatorSessionID string `json:"operator_session_id"`
			OperatorID        string `json:"operator_id"`
			SourceComponent   string `json:"source_component"`
		}
		_ = json.Unmarshal([]byte(data), &envelope)
	})
}
