// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// CollectionName defines canonical collection names for operator.
type CollectionName string

const (
	CollectionUsers                 CollectionName = "users"
	CollectionWebSessions           CollectionName = "web_sessions"
	CollectionOperatorSessions      CollectionName = "operator_sessions"
	CollectionCLISessions           CollectionName = "cli_sessions"
	CollectionLoginAudit            CollectionName = "login_audit"
	CollectionAuthAdminAudit        CollectionName = "auth_admin_audit"
	CollectionAccountLocks          CollectionName = "account_locks"
	CollectionOrganizations         CollectionName = "organizations"
	CollectionOperators             CollectionName = "operators"
	CollectionOperatorUsage         CollectionName = "operator_usage"
	CollectionCases                 CollectionName = "cases"
	CollectionInvestigations        CollectionName = "investigations"
	CollectionTasks                 CollectionName = "tasks"
	CollectionMemories              CollectionName = "memories"
	CollectionSettings              CollectionName = "settings"
	CollectionConsoleAudit          CollectionName = "console_audit"
	CollectionBoundSessions         CollectionName = "bound_sessions"
	CollectionPasskeyChallenges     CollectionName = "passkey_challenges"
	CollectionPersonas              CollectionName = "personas"
	CollectionAgentActivityMetadata CollectionName = "agent_activity_metadata"
	CollectionReputationState       CollectionName = "reputation_state"
	CollectionReputationCommitments CollectionName = "reputation_commitments"
	CollectionStakeResolutions      CollectionName = "stake_resolutions"
	CollectionRevokedCertificates   CollectionName = "revoked_certificates"
	CollectionTrustedSigners        CollectionName = "trusted_signers"
	CollectionAppPolicies           CollectionName = "app_policies"
	CollectionConsensus             CollectionName = "consensus"
	CollectionEnrollmentTokens      CollectionName = "enrollment_tokens"
	CollectionCLIRecoveryRequests   CollectionName = "cli_recovery_requests"
	CollectionPlatformEnrollments   CollectionName = "platform_enrollments"
)
