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
	// MCP routes
	MCPEndpoint string `json:"mcp_endpoint"`
	// A2A routes
	A2ACall   string `json:"a2a_call"`
	A2APrefix string `json:"a2a_prefix"`
	// Governance routes
	GovernanceEnvelopes     string `json:"governance_envelopes"`
	GovernanceSigners       string `json:"governance_signers"`
	GovernanceSignersByID   string `json:"governance_signers_by_id"`
	GovernanceSignersPrefix string `json:"governance_signers_prefix"`
	// Operator routes
	Operators       string `json:"operators"`
	OperatorsByID   string `json:"operators_by_id"`
	OperatorsBind   string `json:"operators_bind"`
	OperatorsUnbind string `json:"operators_unbind"`
	OperatorsTarget string `json:"operators_target"`
	OperatorsReauth string `json:"operators_reauth"`
	// Intent routes
	GrantIntent  string `json:"grant_intent"`
	RevokeIntent string `json:"revoke_intent"`
	// Data routes
	DataSettings    string `json:"data_settings"`
	DataDB          string `json:"data_db"`
	DataBlobs       string `json:"data_blobs"`
	DataPrefix      string `json:"data_prefix"`
	DataItems       string `json:"data_items"`
	DataBlobsPrefix string `json:"data_blobs_prefix"`
	QueryPrefix     string `json:"query_prefix"`
	// KV routes
	KV       string `json:"kv"`
	KVPrefix string `json:"kv_prefix"`
	// PubSub routes
	PubSubPublish   string `json:"pubsub_publish"`
	PubSubStream    string `json:"pubsub_stream"`
	PubSubWebSocket string `json:"pubsub_websocket"`
	// SSE routes
	SSEPush   string `json:"sse_push"`
	SSEEvents string `json:"sse_events"`
	SSEStream string `json:"sse_stream"`
	// PKI routes
	PKICSRSign            string `json:"pki_csr_sign"`
	PKIDevicesEnroll      string `json:"pki_devices_enroll"`
	PKIAppsEnroll         string `json:"pki_apps_enroll"`
	PKIAppsDelegated      string `json:"pki_apps_delegated"`
	PKICertificatesRevoke string `json:"pki_certificates_revoke"`
	PKIRevocationBundle   string `json:"pki_revocation_bundle"`
	PKICRL                string `json:"pki_crl"`
	PKICABundle           string `json:"pki_ca_bundle"`
	PKIFingerprint        string `json:"pki_fingerprint"`
	// Audit routes
	AuditReceipts       string `json:"audit_receipts"`
	AuditReceiptsExport string `json:"audit_receipts_export"`
	AuditEvents         string `json:"audit_events"`
	AuditSummary        string `json:"audit_summary"`
	AuditReport         string `json:"audit_report"`
	AuditStream         string `json:"audit_stream"`
	// User routes
	Users   string `json:"users"`
	UsersMe string `json:"users_me"`
	// Auth routes
	AuthLogout                               string `json:"auth_logout"`
	AuthBootstrap                            string `json:"auth_bootstrap"`
	AuthBootstrapStatus                      string `json:"auth_bootstrap_status"`
	AuthCLIEnroll                            string `json:"auth_cli_enroll"`
	AuthDeviceEnroll                         string `json:"auth_device_enroll"`
	AuthCLIRecoveryRequest                   string `json:"auth_cli_recovery_request"`
	AuthCLIRecoveryStatus                    string `json:"auth_cli_recovery_status"`
	AuthCLIRecoveryApprove                   string `json:"auth_cli_recovery_approve"`
	AuthCLIRecoveryComplete                  string `json:"auth_cli_recovery_complete"`
	AuthCLIRotate                            string `json:"auth_cli_rotate"`
	AuthPasskeys                             string `json:"auth_passkeys"`
	AuthPasskeysByID                         string `json:"auth_passkeys_by_id"`
	AuthPasskeysJITRegisterChallenge         string `json:"auth_passkeys_jit_register_challenge"`
	AuthPasskeysJITRegisterVerify            string `json:"auth_passkeys_jit_register_verify"`
	AuthPasskeysJITPrefix                    string `json:"auth_passkeys_jit_prefix"`
	AuthPasskeysPrefix                       string `json:"auth_passkeys_prefix"`
	AuthPasskeysCLIStatus                    string `json:"auth_passkeys_cli_status"`
	AuthPasskeysConsoleRegisterChallenge     string `json:"auth_passkeys_console_register_challenge"`
	AuthPasskeysConsoleRegisterVerify        string `json:"auth_passkeys_console_register_verify"`
	AuthPasskeysConsoleAuthenticateChallenge string `json:"auth_passkeys_console_authenticate_challenge"`
	AuthPasskeysConsoleAuthenticateVerify    string `json:"auth_passkeys_console_authenticate_verify"`
	AuthPasskeysConsolePrefix                string `json:"auth_passkeys_console_prefix"`
	AuthSessionsMe                           string `json:"auth_sessions_me"`
	AuthEnrollmentTokenGenerate              string `json:"auth_enrollment_token_generate"`
	AuthEnrollmentTokenValidate              string `json:"auth_enrollment_token_validate"`
	// Approval routes
	Approvals             string `json:"approvals"`
	ApprovalsByID         string `json:"approvals_by_id"`
	ApprovalsPrefix       string `json:"approvals_prefix"`
	ApprovePage           string `json:"approve_page"`
	ApprovePagePrefix     string `json:"approve_page_prefix"`
	ApprovalsVerifyAction string `json:"approvals_verify_action"`
	ApprovalsCLIStatus    string `json:"approvals_cli_status"`
	ApprovalsCLIList      string `json:"approvals_cli_list"`
	// Admin routes
	AdminAppPoliciesBySigner string `json:"admin_app_policies_by_signer"`
	AdminAppsRevoke          string `json:"admin_apps_revoke"`
	AdminAppPoliciesPrefix   string `json:"admin_app_policies_prefix"`
	AdminConsensus           string `json:"admin_consensus"`
	AdminConsensusByID       string `json:"admin_consensus_by_id"`
	AdminConsensusPrefix     string `json:"admin_consensus_prefix"`
	// Consensus routes
	ConsensusDeliberate string `json:"consensus_deliberate"`
	// Well-known routes
	WellKnownPKICABundle    string `json:"well_known_pki_ca_bundle"`
	WellKnownPKIFingerprint string `json:"well_known_pki_fingerprint"`
	WellKnownBinPrefix      string `json:"well_known_bin_prefix"`
	WellKnownPKIPrefix      string `json:"well_known_pki_prefix"`
	// Deploy scripts
	DeployScriptLinux   string `json:"deploy_script_linux"`
	DeployScriptWindows string `json:"deploy_script_windows"`
	// Console SPA
	ConsolePrefix string `json:"console_prefix"`
	// WebSocket prefix
	WSPrefix string `json:"ws_prefix"`
	// User routes
	UsersPrefix string `json:"users_prefix"`
	// Auth session routes
	AuthSessionsPrefix string `json:"auth_sessions_prefix"`
	// Health
	Health string `json:"health"`
	// State
	State string `json:"state"`
	// Landing
	Landing string `json:"landing"`
}{
	InternalPrefix: "/api/v1",
	OperatorPrefix: "/api",
	Client: map[string]string{
		"chat":       "/api/v1/chat",
		"health":     "/api/v1/health",
		"sse_events": "/api/v1/internal/sse/events",
		"sse_stream": "/api/v1/internal/sse/stream",
	},
	// MCP routes
	MCPEndpoint: "/mcp",
	// A2A routes
	A2ACall:   "/api/v1/a2a/call",
	A2APrefix: "/api/v1/a2a/",
	// Governance routes
	GovernanceEnvelopes:     "/api/v1/governance/envelopes",
	GovernanceSigners:       "/api/v1/governance/signers",
	GovernanceSignersByID:   "/api/v1/governance/signers/",
	GovernanceSignersPrefix: "/api/v1/governance/signers/",
	// Operator routes
	Operators:       "/api/v1/operators",
	OperatorsByID:   "/api/v1/operators/",
	OperatorsBind:   "/api/v1/operators/bind",
	OperatorsUnbind: "/api/v1/operators/unbind",
	OperatorsTarget: "/api/v1/operators/target",
	OperatorsReauth: "/api/v1/operators/reauth",
	// Intent routes
	GrantIntent:  "/api/v1/operators/{operator_id}/intents/grant",
	RevokeIntent: "/api/v1/operators/{operator_id}/intents/revoke",
	// Data routes
	DataSettings:    "/api/v1/data/settings",
	DataDB:          "/api/v1/data/",
	DataBlobs:       "/api/v1/blobs/",
	DataPrefix:      "/api/v1/data/",
	DataItems:       "/api/v1/data/items",
	DataBlobsPrefix: "/api/v1/blobs/",
	QueryPrefix:     "/_query",
	// KV routes
	KV:       "/api/v1/kv/",
	KVPrefix: "/api/v1/kv/",
	// PubSub routes
	PubSubPublish:   "/api/v1/pubsub/publish",
	PubSubStream:    "/api/v1/pubsub/stream",
	PubSubWebSocket: "/ws/pubsub",
	// SSE routes
	SSEPush:   "/api/v1/sse/push",
	SSEEvents: "/api/v1/sse/events",
	SSEStream: "/api/v1/sse/stream",
	// PKI routes
	PKICSRSign:            "/api/v1/pki/csr/sign",
	PKIDevicesEnroll:      "/api/v1/pki/devices/enroll",
	PKIAppsEnroll:         "/api/v1/pki/apps/enroll",
	PKIAppsDelegated:      "/api/v1/pki/apps/delegated",
	PKICertificatesRevoke: "/api/v1/pki/certificates/revoke",
	PKIRevocationBundle:   "/api/v1/pki/revocation-bundle",
	PKICRL:                "/.well-known/g8e/pki/crl",
	PKICABundle:           "/.well-known/g8e/pki/ca-bundle",
	PKIFingerprint:        "/.well-known/g8e/pki/fingerprint",
	// Audit routes
	AuditReceipts:       "/api/v1/audit/receipts",
	AuditReceiptsExport: "/api/v1/audit/receipts/export",
	AuditEvents:         "/api/v1/audit/events",
	AuditSummary:        "/api/v1/audit/summary",
	AuditReport:         "/api/v1/audit/report",
	AuditStream:         "/api/v1/audit/stream",
	// User routes
	Users:   "/api/v1/users",
	UsersMe: "/api/v1/users/me",
	// Auth routes
	AuthLogout:                               "/api/v1/auth/logout",
	AuthBootstrap:                            "/api/v1/auth/bootstrap",
	AuthBootstrapStatus:                      "/api/v1/auth/bootstrap/status",
	AuthCLIEnroll:                            "/api/v1/auth/cli/enroll",
	AuthDeviceEnroll:                         "/api/v1/auth/device/enroll",
	AuthCLIRecoveryRequest:                   "/api/v1/auth/cli/recovery/request",
	AuthCLIRecoveryStatus:                    "/api/v1/auth/cli/recovery/status",
	AuthCLIRecoveryApprove:                   "/api/v1/auth/cli/recovery/approve",
	AuthCLIRecoveryComplete:                  "/api/v1/auth/cli/recovery/complete",
	AuthCLIRotate:                            "/api/v1/auth/cli/rotate",
	AuthPasskeys:                             "/api/v1/auth/passkeys",
	AuthPasskeysByID:                         "/api/v1/auth/passkeys/",
	AuthPasskeysJITRegisterChallenge:         "/api/v1/auth/passkeys/jit-register/challenge",
	AuthPasskeysJITRegisterVerify:            "/api/v1/auth/passkeys/jit-register/verify",
	AuthPasskeysJITPrefix:                    "/api/v1/auth/passkeys/jit-",
	AuthPasskeysPrefix:                       "/api/v1/auth/passkeys/",
	AuthPasskeysCLIStatus:                    "/api/v1/auth/passkeys/cli/status",
	AuthPasskeysConsoleRegisterChallenge:     "/api/v1/auth/passkeys/console/register/challenge",
	AuthPasskeysConsoleRegisterVerify:        "/api/v1/auth/passkeys/console/register/verify",
	AuthPasskeysConsoleAuthenticateChallenge: "/api/v1/auth/passkeys/console/authenticate/challenge",
	AuthPasskeysConsoleAuthenticateVerify:    "/api/v1/auth/passkeys/console/authenticate/verify",
	AuthPasskeysConsolePrefix:                "/api/v1/auth/passkeys/console/",
	AuthSessionsMe:                           "/api/v1/auth/sessions/me",
	AuthEnrollmentTokenGenerate:              "/api/v1/auth/enrollment-token/generate",
	AuthEnrollmentTokenValidate:              "/api/v1/auth/enrollment-token/validate",
	// Approval routes
	Approvals:             "/api/v1/approvals",
	ApprovalsByID:         "/api/v1/approvals/",
	ApprovalsPrefix:       "/api/v1/approvals/",
	ApprovePage:           "/api/v1/approve/",
	ApprovePagePrefix:     "/api/v1/approve/",
	ApprovalsVerifyAction: "/verify",
	ApprovalsCLIStatus:    "/api/v1/approvals/status/",
	ApprovalsCLIList:      "/api/v1/approvals/pending",
	// Admin routes
	AdminAppPoliciesBySigner: "/api/v1/admin/app-policies/",
	AdminAppsRevoke:          "/api/v1/admin/apps/revoke",
	AdminAppPoliciesPrefix:   "/api/v1/admin/app-policies/",
	AdminConsensus:           "/api/v1/admin/consensus",
	AdminConsensusByID:       "/api/v1/admin/consensus/",
	AdminConsensusPrefix:     "/api/v1/admin/consensus/",
	// Consensus routes
	ConsensusDeliberate: "/consensus/v1/deliberate",
	// Well-known routes
	WellKnownPKICABundle:    "/.well-known/g8e/pki/ca-bundle",
	WellKnownPKIFingerprint: "/.well-known/g8e/pki/fingerprint",
	WellKnownBinPrefix:      "/.well-known/g8e/bin/",
	WellKnownPKIPrefix:      "/.well-known/g8e/pki/",
	// Deploy scripts
	DeployScriptLinux:   "/" + DeployScriptFilenameLinux,
	DeployScriptWindows: "/" + DeployScriptFilenameWindows,
	// Console SPA
	ConsolePrefix: "/console/",
	// WebSocket prefix
	WSPrefix: "/ws/",
	// User routes
	UsersPrefix: "/api/v1/users/",
	// Auth session routes
	AuthSessionsPrefix: "/api/v1/auth/sessions/",
	// Health
	Health: "/api/v1/health",
	// State
	State: "/api/v1/state",
	// Landing
	Landing: "/",
}
