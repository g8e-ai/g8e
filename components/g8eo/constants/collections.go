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

// CollectionName defines canonical collection names for g8es.
type CollectionName string

const (
	CollectionUsers             CollectionName = "users"
	CollectionWebSessions       CollectionName = "web_sessions"
	CollectionOperatorSessions  CollectionName = "operator_sessions"
	CollectionLoginAudit        CollectionName = "login_audit"
	CollectionAuthAdminAudit    CollectionName = "auth_admin_audit"
	CollectionAccountLocks      CollectionName = "account_locks"
	CollectionAPIKeys           CollectionName = "api_keys"
	CollectionOrganizations     CollectionName = "organizations"
	CollectionOperators         CollectionName = "operators"
	CollectionOperatorUsage     CollectionName = "operator_usage"
	CollectionCases             CollectionName = "cases"
	CollectionInvestigations    CollectionName = "investigations"
	CollectionTasks             CollectionName = "tasks"
	CollectionMemories          CollectionName = "memories"
	CollectionSettings          CollectionName = "settings"
	CollectionConsoleAudit      CollectionName = "console_audit"
	CollectionBoundSessions     CollectionName = "bound_sessions"
	CollectionPasskeyChallenges CollectionName = "passkey_challenges"
)

// DocumentID defines canonical document IDs for g8es.
type DocumentID string

const (
	DocIDPlatformSettings DocumentID = "platform_settings"
)
