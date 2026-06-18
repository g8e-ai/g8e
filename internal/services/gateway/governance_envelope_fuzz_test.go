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

//go:build integration

package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// FuzzEnvelopeIdentityBinding fuzzes the real verifyEnvelopeIdentityBinding
// against random request bodies. It exercises the canonical protojson decode
// path (the format real BYO clients send) and asserts the function never
// panics — malformed input must pass through gracefully to the processor.
func FuzzEnvelopeIdentityBinding(f *testing.F) {
	// Seed corpus in canonical protojson wire form (camelCase, enum-as-string)
	// plus malformed/edge-case inputs.
	f.Add(`{"operatorSessionId":"sess-123","operatorId":"op-456","sourceComponent":"COMPONENT_CLIENT"}`)
	f.Add(`{"operatorSessionId":"","operatorId":""}`)
	f.Add(`{"operatorSessionId":"test"}`)
	f.Add(`{"operatorId":"test"}`)
	f.Add(`{"sourceComponent":"COMPONENT_AGENT"}`)
	f.Add(`{"operatorSessionId":"` + string(make([]byte, 10000)) + `"}`)
	f.Add(`{"nested":{"deep":{"value":"test"}}}`)
	f.Add(`invalid json`)
	f.Add(``)
	f.Add(`{}`)
	f.Add(`[]`)
	f.Add(`null`)

	spiffeURL, err := url.Parse("spiffe://g8e.local/operator/org-1/op-1/sess-1")
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, data string) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{spiffeURL}}},
		}
		// Must never panic regardless of input.
		_ = verifyEnvelopeIdentityBinding(req, []byte(data))
	})
}
