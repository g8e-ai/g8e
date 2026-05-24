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

package sentinel

import (
	"log/slog"
	"testing"
)

// FuzzAnalyzeMCPArguments tests AnalyzeMCPArguments with random JSON inputs
// to catch edge-case panics, stack exhausts, and JSON parsing errors.
func FuzzAnalyzeMCPArguments(f *testing.F) {
	// Add seed corpus with valid and edge-case inputs
	f.Add(`{"command":"ls","path":"/tmp"}`)
	f.Add(`{"nested":{"deep":{"value":"test"}}}`)
	f.Add(`{"array":["item1","item2","item3"]}`)
	f.Add(`{"empty":{}}`)
	f.Add(`{"null":null}`)
	f.Add(`{"number":123,"bool":true}`)
	f.Add(`{"unicode":"测试🚀"}`)
	f.Add(`{"escape":"\"quoted\""}`)
	f.Add(`{"large":"` + string(make([]byte, 10000)) + `"}`)
	f.Add(`invalid json`)
	f.Add(``)
	f.Add(`{"recursive":{}}`)

	sentinel := NewSentinel(&SentinelConfig{SentinelEnabled: true}, slog.Default())

	f.Fuzz(func(t *testing.T, argumentsJSON string) {
		// This should never panic - AnalyzeMCPArguments must handle all inputs gracefully
		result := sentinel.AnalyzeMCPArguments(argumentsJSON)

		// Validate result structure
		if result == nil {
			t.Errorf("AnalyzeMCPArguments returned nil for input: %q", argumentsJSON)
		}

		// If result exists, validate fields
		if result != nil {
			// Safe should be false for invalid JSON, true otherwise
			// ThreatLevel should be one of the valid enum values
			// RiskScore should be 0-100
			if result.RiskScore < 0 || result.RiskScore > 100 {
				t.Errorf("RiskScore out of range [0,100]: %d", result.RiskScore)
			}
		}
	})
}
