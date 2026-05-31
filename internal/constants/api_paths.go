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

// APIPaths defines the canonical G8E API paths.
var APIPaths = struct {
	InternalPrefix string            `json:"internal_prefix"`
	OperatorPrefix string            `json:"operator_prefix"`
	Client         map[string]string `json:"client"`
	ClientFull     map[string]string `json:"client_full"`
	Gateway        map[string]string `json:"gateway"`
}{
	InternalPrefix: "/api/v1",
	OperatorPrefix: "/api",
	Client: map[string]string{
		"chat":       "/chat",
		"health":     "/health",
		"sse_events": "/internal/sse/events",
		"sse_stream": "/internal/sse/stream",
	},
	ClientFull: map[string]string{
		"chat":       "/api/chat",
		"health":     "/api/health",
		"sse_events": "/api/internal/sse/events",
		"sse_stream": "/api/internal/sse/stream",
	},
	Gateway: map[string]string{
		"governance_envelopes": "/api/v1/governance/envelopes",
	},
}

// APIPathsGenerated is a mirror of APIPaths for compatibility.
// This is kept as a separate variable to avoid circular dependencies during build.
var APIPathsGenerated = APIPaths
