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
	MCPEndpoint      string `json:"mcp_endpoint"`
	MCPToolsList     string `json:"mcp_tools_list"`
	MCPToolsCall     string `json:"mcp_tools_call"`
	MCPToolsCallSSE  string `json:"mcp_tools_call_sse"`
	MCPResourcesList string `json:"mcp_resources_list"`
	MCPResourcesRead string `json:"mcp_resources_read"`
	MCPPromptsList   string `json:"mcp_prompts_list"`
	MCPPromptsGet    string `json:"mcp_prompts_get"`
	// A2A routes
	A2ACall string `json:"a2a_call"`
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
	// Data routes
	DataSettings    string `json:"data_settings"`
	DataDB          string `json:"data_db"`
	DataBlobs       string `json:"data_blobs"`
	DataPrefix      string `json:"data_prefix"`
	DataItems       string `json:"data_items"`
	DataBlobsPrefix string `json:"data_blobs_prefix"`
	// KV routes
	KV       string `json:"kv"`
	KVPrefix string `json:"kv_prefix"`
	// PubSub routes
	PubSubPublish string `json:"pubsub_publish"`
	PubSubStream  string `json:"pubsub_stream"`
	// SSE routes
	SSEPush   string `json:"sse_push"`
	SSEEvents string `json:"sse_events"`
	SSEStream string `json:"sse_stream"`
	// PKI routes
	PKICSRSign            string `json:"pki_csr_sign"`
	PKIDevicesEnroll      string `json:"pki_devices_enroll"`
	PKIAppsEnroll         string `json:"pki_apps_enroll"`
	PKICertificatesRevoke string `json:"pki_certificates_revoke"`
	PKIRevocationBundle   string `json:"pki_revocation_bundle"`
	PKICRL                string `json:"pki_crl"`
	PKICABundle           string `json:"pki_ca_bundle"`
	PKIFingerprint        string `json:"pki_fingerprint"`
	// Audit routes
	AuditReceipts       string `json:"audit_receipts"`
	AuditReceiptsExport string `json:"audit_receipts_export"`
	// User routes
	Users   string `json:"users"`
	UsersMe string `json:"users_me"`
	// Auth routes
	AuthLoginVerify                      string `json:"auth_login_verify"`
	AuthLogout                           string `json:"auth_logout"`
	AuthBootstrap                        string `json:"auth_bootstrap"`
	AuthBootstrapStatus                  string `json:"auth_bootstrap_status"`
	AuthCLIEnroll                        string `json:"auth_cli_enroll"`
	AuthDeviceEnroll                     string `json:"auth_device_enroll"`
	AuthPasskeysRegisterChallenge        string `json:"auth_passkeys_register_challenge"`
	AuthPasskeysRegisterVerify           string `json:"auth_passkeys_register_verify"`
	AuthPasskeysAuthenticateChallenge    string `json:"auth_passkeys_authenticate_challenge"`
	AuthPasskeysAuthenticateVerify       string `json:"auth_passkeys_authenticate_verify"`
	AuthPasskeys                         string `json:"auth_passkeys"`
	AuthPasskeysByID                     string `json:"auth_passkeys_by_id"`
	AuthPasskeysJITRegisterChallenge     string `json:"auth_passkeys_jit_register_challenge"`
	AuthPasskeysJITRegisterVerify        string `json:"auth_passkeys_jit_register_verify"`
	AuthPasskeysJITPrefix                string `json:"auth_passkeys_jit_prefix"`
	AuthPasskeysPrefix                   string `json:"auth_passkeys_prefix"`
	AuthPasskeysCLIRegisterChallenge     string `json:"auth_passkeys_cli_register_challenge"`
	AuthPasskeysCLIRegisterVerify        string `json:"auth_passkeys_cli_register_verify"`
	AuthPasskeysCLIAuthenticateChallenge string `json:"auth_passkeys_cli_authenticate_challenge"`
	AuthPasskeysCLIAuthenticateVerify    string `json:"auth_passkeys_cli_authenticate_verify"`
	AuthSessionsMe                       string `json:"auth_sessions_me"`
	// Approval routes
	Approvals         string `json:"approvals"`
	ApprovalsByID     string `json:"approvals_by_id"`
	ApprovalsPrefix   string `json:"approvals_prefix"`
	ApprovePage       string `json:"approve_page"`
	ApprovePagePrefix string `json:"approve_page_prefix"`
	// Admin routes
	AdminAppPoliciesBySigner string `json:"admin_app_policies_by_signer"`
	AdminAppsRevoke          string `json:"admin_apps_revoke"`
	AdminAppPoliciesPrefix   string `json:"admin_app_policies_prefix"`
	// Well-known routes
	WellKnownPKICABundle    string `json:"well_known_pki_ca_bundle"`
	WellKnownPKIFingerprint string `json:"well_known_pki_fingerprint"`
	WellKnownBinPrefix      string `json:"well_known_bin_prefix"`
	// Bootstrap scripts
	BootstrapCALinux   string `json:"bootstrap_ca_linux"`
	BootstrapCAWindows string `json:"bootstrap_ca_windows"`
	// Deploy scripts
	DeployScriptLinux   string `json:"deploy_script_linux"`
	DeployScriptWindows string `json:"deploy_script_windows"`
	// Health
	Health string `json:"health"`
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
	MCPEndpoint:      "/mcp",
	MCPToolsList:     "/api/v1/mcp/tools/list",
	MCPToolsCall:     "/api/v1/mcp/tools/call",
	MCPToolsCallSSE:  "/api/v1/mcp/tools/call/sse",
	MCPResourcesList: "/api/v1/mcp/resources/list",
	MCPResourcesRead: "/api/v1/mcp/resources/read",
	MCPPromptsList:   "/api/v1/mcp/prompts/list",
	MCPPromptsGet:    "/api/v1/mcp/prompts/get",
	// A2A routes
	A2ACall: "/api/v1/a2a/call",
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
	// Data routes
	DataSettings:    "/api/v1/data/settings",
	DataDB:          "/api/v1/data/",
	DataBlobs:       "/api/v1/blobs/",
	DataPrefix:      "/api/v1/data/",
	DataItems:       "/api/v1/data/items",
	DataBlobsPrefix: "/api/v1/blobs/",
	// KV routes
	KV:       "/api/v1/kv/",
	KVPrefix: "/api/v1/kv/",
	// PubSub routes
	PubSubPublish: "/api/v1/pubsub/publish",
	PubSubStream:  "/api/v1/pubsub/stream",
	// SSE routes
	SSEPush:   "/api/v1/sse/push",
	SSEEvents: "/api/v1/sse/events",
	SSEStream: "/api/v1/sse/stream",
	// PKI routes
	PKICSRSign:            "/api/v1/pki/csr/sign",
	PKIDevicesEnroll:      "/api/v1/pki/devices/enroll",
	PKIAppsEnroll:         "/api/v1/pki/apps/enroll",
	PKICertificatesRevoke: "/api/v1/pki/certificates/revoke",
	PKIRevocationBundle:   "/api/v1/pki/revocation-bundle",
	PKICRL:                "/.well-known/g8e/pki/crl",
	PKICABundle:           "/.well-known/g8e/pki/ca-bundle",
	PKIFingerprint:        "/.well-known/g8e/pki/fingerprint",
	// Audit routes
	AuditReceipts:       "/api/v1/audit/receipts",
	AuditReceiptsExport: "/api/v1/audit/receipts/export",
	// User routes
	Users:   "/api/v1/users",
	UsersMe: "/api/v1/users/me",
	// Auth routes
	AuthLoginVerify:                      "/api/v1/auth/login/verify",
	AuthLogout:                           "/api/v1/auth/logout",
	AuthBootstrap:                        "/api/v1/auth/bootstrap",
	AuthBootstrapStatus:                  "/api/v1/auth/bootstrap/status",
	AuthCLIEnroll:                        "/api/v1/auth/cli/enroll",
	AuthDeviceEnroll:                     "/api/v1/auth/device/enroll",
	AuthPasskeysRegisterChallenge:        "/api/v1/auth/passkeys/register/challenge",
	AuthPasskeysRegisterVerify:           "/api/v1/auth/passkeys/register/verify",
	AuthPasskeysAuthenticateChallenge:    "/api/v1/auth/passkeys/authenticate/challenge",
	AuthPasskeysAuthenticateVerify:       "/api/v1/auth/passkeys/authenticate/verify",
	AuthPasskeys:                         "/api/v1/auth/passkeys",
	AuthPasskeysByID:                     "/api/v1/auth/passkeys/",
	AuthPasskeysJITRegisterChallenge:     "/api/v1/auth/passkeys/jit-register/challenge",
	AuthPasskeysJITRegisterVerify:        "/api/v1/auth/passkeys/jit-register/verify",
	AuthPasskeysJITPrefix:                "/api/v1/auth/passkeys/jit-",
	AuthPasskeysPrefix:                   "/api/v1/auth/passkeys/",
	AuthPasskeysCLIRegisterChallenge:     "/api/v1/auth/passkeys/cli-register/challenge",
	AuthPasskeysCLIRegisterVerify:        "/api/v1/auth/passkeys/cli-register/verify",
	AuthPasskeysCLIAuthenticateChallenge: "/api/v1/auth/passkeys/cli/authenticate/challenge",
	AuthPasskeysCLIAuthenticateVerify:    "/api/v1/auth/passkeys/cli/authenticate/verify",
	AuthSessionsMe:                       "/api/v1/auth/sessions/me",
	// Approval routes
	Approvals:         "/api/v1/approvals",
	ApprovalsByID:     "/api/v1/approvals/",
	ApprovalsPrefix:   "/api/v1/approvals/",
	ApprovePage:       "/api/v1/approve/",
	ApprovePagePrefix: "/api/v1/approve/",
	// Admin routes
	AdminAppPoliciesBySigner: "/api/v1/admin/app-policies/",
	AdminAppsRevoke:          "/api/v1/admin/apps/revoke",
	AdminAppPoliciesPrefix:   "/api/v1/admin/app-policies/",
	// Well-known routes
	WellKnownPKICABundle:    "/.well-known/g8e/pki/ca-bundle",
	WellKnownPKIFingerprint: "/.well-known/g8e/pki/fingerprint",
	WellKnownBinPrefix:      "/.well-known/g8e/bin/",
	// Bootstrap scripts
	BootstrapCALinux:   "/bootstrap-ca",
	BootstrapCAWindows: "/bootstrap-ca.ps1",
	// Deploy scripts
	DeployScriptLinux:   "/g8e-operator.sh",
	DeployScriptWindows: "/g8e-operator.ps1",
	// Health
	Health: "/api/v1/health",
	// Landing
	Landing: "/",
}
