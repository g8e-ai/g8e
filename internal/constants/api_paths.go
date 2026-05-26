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

// APIPaths defines the canonical G8E API paths.
var APIPaths = struct {
	InternalPrefix string            `json:"internal_prefix"`
	OperatorPrefix string            `json:"operator_prefix"`
	G8ee           map[string]string `json:"g8ee"`
	G8eeFull       map[string]string `json:"g8ee_full"`
	Client         map[string]string `json:"client"`
	ClientFull     map[string]string `json:"client_full"`
}{
	InternalPrefix: "/api/v1",
	OperatorPrefix: "/api",
	G8ee: map[string]string{
		"auth_generate_key":              "/auth/api-key/generate",
		"auth_revoke_cert":               "/auth/certificate/revoke",
		"case":                           "/case",
		"cases":                          "/cases",
		"chat":                           "/chat",
		"chat_stop":                      "/chat/stop",
		"chat_triage_answer":             "/chat/triage/answer",
		"chat_triage_skip":               "/chat/triage/skip",
		"chat_triage_timeout":            "/chat/triage/timeout",
		"health":                         "/health",
		"investigation":                  "/investigation",
		"investigations":                 "/investigations",
		"operator_approval_pending":      "/operator/approval/pending",
		"operator_approval_respond":      "/operator/approval/respond",
		"operator_direct_command":        "/operator/direct-command",
		"operators_authenticate":         "/operators/authenticate",
		"operators_bind":                 "/operators/bind",
		"operators_claim_slot":           "/operators/claim-slot",
		"operators_create_slot":          "/operators/create-slot",
		"operators_deregister_session":   "/operators/session/deregister",
		"operators_device_link_register": "/operators/device-link/register",
		"operators_gateway_session_auth": "/operators/gateway-session-auth",
		"operators_refresh_session":      "/operators/session/refresh",
		"operators_register_session":     "/operators/session/register",
		"operators_stop":                 "/operators/stop",
		"operators_terminate":            "/operators/terminate",
		"operators_unbind":               "/operators/unbind",
		"operators_update_api_key":       "/operators/update-api-key",
		"operators_validate_session":     "/operators/session/validate",
		"settings":                       "/settings",
		"settings_sync":                  "/settings/sync",
		"settings_user":                  "/settings/user",
	},
	G8eeFull: map[string]string{
		"auth_generate_key":              "/api/v1/auth/api-key/generate",
		"auth_revoke_cert":               "/api/v1/auth/certificate/revoke",
		"case":                           "/api/v1/case",
		"cases":                          "/api/v1/cases",
		"chat":                           "/api/v1/chat",
		"chat_stop":                      "/api/v1/chat/stop",
		"chat_triage_answer":             "/api/v1/chat/triage/answer",
		"chat_triage_skip":               "/api/v1/chat/triage/skip",
		"chat_triage_timeout":            "/api/v1/chat/triage/timeout",
		"health":                         "/api/v1/health",
		"investigation":                  "/api/v1/investigation",
		"investigations":                 "/api/v1/investigations",
		"operator_approval_pending":      "/api/v1/operator/approval/pending",
		"operator_approval_respond":      "/api/v1/operator/approval/respond",
		"operator_direct_command":        "/api/v1/operator/direct-command",
		"operators_authenticate":         "/api/v1/operators/authenticate",
		"operators_bind":                 "/api/v1/operators/bind",
		"operators_claim_slot":           "/api/v1/operators/claim-slot",
		"operators_create_slot":          "/api/v1/operators/create-slot",
		"operators_deregister_session":   "/api/v1/operators/session/deregister",
		"operators_device_link_register": "/api/v1/operators/device-link/register",
		"operators_gateway_session_auth": "/api/v1/operators/gateway-session-auth",
		"operators_refresh_session":      "/api/v1/operators/session/refresh",
		"operators_register_session":     "/api/v1/operators/session/register",
		"operators_stop":                 "/api/v1/operators/stop",
		"operators_terminate":            "/api/v1/operators/terminate",
		"operators_unbind":               "/api/v1/operators/unbind",
		"operators_update_api_key":       "/api/v1/operators/update-api-key",
		"operators_validate_session":     "/api/v1/operators/session/validate",
		"settings":                       "/api/v1/settings",
		"settings_sync":                  "/api/v1/settings/sync",
		"settings_user":                  "/api/v1/settings/user",
	},
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
}

// APIPathsGenerated is a mirror of APIPaths for compatibility.
// This is kept as a separate variable to avoid circular dependencies during build.
var APIPathsGenerated = APIPaths

// GetG8eePath returns the full internal path for a G8ee API route key.
func GetG8eePath(key string) string {
	path, ok := APIPaths.G8ee[key]
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s%s", APIPaths.InternalPrefix, path)
}
